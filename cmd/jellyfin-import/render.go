// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/provider"
)

// omittedAttributes lists, by resource type, the configurable attributes the
// importer leaves out on purpose, as dotted paths through nested attributes.
// Each is optional and computed, so the imported state keeps its value
// without the configuration planning a change.
var omittedAttributes = map[string][]string{
	"jellyfin_user":    {"policy"},
	"jellyfin_library": {"library_options"},
	// Jellyfin maintains it itself.
	"jellyfin_livetv_configuration": {"media_locations_created"},
}

// rendered reports whether the importer writes the attribute at attrPath of
// resourceType into the configuration. Sensitive values stay out of
// resources.tf, and a deprecated attribute leaves the imported value to the
// attribute that replaces it.
func rendered(resourceType, attrPath string, a schema.Attribute) bool {
	return (a.IsOptional() || a.IsRequired()) && !a.IsSensitive() && a.GetDeprecationMessage() == "" && !slices.Contains(omittedAttributes[resourceType], attrPath)
}

// importedAttributes renders the attributes of resourceType as the Read that
// follows its import with importID reads them from raw, the document
// Jellyfin serves, and from what else c serves, so the configuration sets
// exactly what the imported state holds.
func importedAttributes(ctx context.Context, c *client.Client, resourceType, importID, raw string) (map[string]string, error) {
	s, state, err := provider.ReadForImport(ctx, c, resourceType, importID, raw)
	if err != nil {
		return nil, err
	}
	return renderAttributes(resourceType, "", s.Attributes, state, 1)
}

// renderAttributes renders the non-null values of obj's attributes that
// rendered accepts. depth is the block nesting level the attributes are
// written at, used to indent multi-line values.
func renderAttributes(resourceType, parent string, attrs map[string]schema.Attribute, obj basetypes.ObjectValue, depth int) (map[string]string, error) {
	out := map[string]string{}
	values := obj.Attributes()
	for name, a := range attrs {
		p := name
		if parent != "" {
			p = parent + "." + name
		}
		v := values[name]
		if !rendered(resourceType, p, a) || v == nil || v.IsNull() || v.IsUnknown() {
			continue
		}
		s, err := renderValue(resourceType, p, a, v, depth)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out[name] = s
	}
	return out, nil
}

func renderValue(resourceType, attrPath string, a schema.Attribute, v attr.Value, depth int) (string, error) {
	switch n := a.(type) {
	case schema.SingleNestedAttribute:
		o, ok := v.(basetypes.ObjectValue)
		if !ok {
			return "", fmt.Errorf("got %T for an object", v)
		}
		attrs, err := renderAttributes(resourceType, attrPath, n.Attributes, o, depth+1)
		if err != nil {
			return "", err
		}
		return hclObject(attrs, depth), nil
	case schema.ListNestedAttribute:
		l, ok := v.(basetypes.ListValue)
		if !ok {
			return "", fmt.Errorf("got %T for a list", v)
		}
		if len(l.Elements()) == 0 {
			return "[]", nil
		}
		indent := strings.Repeat("  ", depth)
		var b strings.Builder
		b.WriteString("[\n")
		for _, e := range l.Elements() {
			o, ok := e.(basetypes.ObjectValue)
			if !ok {
				return "", fmt.Errorf("got %T for a list element", e)
			}
			attrs, err := renderAttributes(resourceType, attrPath, n.NestedObject.Attributes, o, depth+2)
			if err != nil {
				return "", err
			}
			b.WriteString(indent + "  " + hclObject(attrs, depth+1) + ",\n")
		}
		b.WriteString(indent + "]")
		return b.String(), nil
	}
	return renderScalar(v)
}

func renderScalar(v attr.Value) (string, error) {
	if v.IsNull() {
		return "null", nil
	}
	switch x := v.(type) {
	case basetypes.StringValue:
		return hclString(x.ValueString()), nil
	case basetypes.BoolValue:
		return strconv.FormatBool(x.ValueBool()), nil
	case basetypes.Int64Value:
		return strconv.FormatInt(x.ValueInt64(), 10), nil
	case basetypes.Float64Value:
		return strconv.FormatFloat(x.ValueFloat64(), 'f', -1, 64), nil
	case basetypes.ListValue:
		elems := make([]string, len(x.Elements()))
		for i, e := range x.Elements() {
			s, err := renderScalar(e)
			if err != nil {
				return "", err
			}
			elems[i] = s
		}
		return "[" + strings.Join(elems, ", ") + "]", nil
	}
	return "", fmt.Errorf("cannot render a %T", v)
}
