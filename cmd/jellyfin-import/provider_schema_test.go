// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/provider"
)

func providerSchemas(t *testing.T) map[string]schema.Schema {
	t.Helper()

	ctx := t.Context()
	schemas := make(map[string]schema.Schema)
	for _, newResource := range provider.New("test")().Resources(ctx) {
		r := newResource()
		var meta fwresource.MetadataResponse
		r.Metadata(ctx, fwresource.MetadataRequest{ProviderTypeName: "jellyfin"}, &meta)
		var s fwresource.SchemaResponse
		r.Schema(ctx, fwresource.SchemaRequest{}, &s)
		if s.Diagnostics.HasError() {
			t.Fatalf("schema of %s: %v", meta.TypeName, s.Diagnostics)
		}
		schemas[meta.TypeName] = s.Schema
	}
	return schemas
}

// TestOmittedAttributesAreConfigurable keeps omittedAttributes from naming
// an attribute the provider renamed or dropped, which would leave nothing out.
func TestOmittedAttributesAreConfigurable(t *testing.T) {
	schemas := providerSchemas(t)
	for resourceType, paths := range omittedAttributes {
		s, ok := schemas[resourceType]
		if !ok {
			t.Errorf("the provider has no %s resource", resourceType)
			continue
		}
		for _, p := range paths {
			a, ok := attributeAt(s.Attributes, p)
			switch {
			case !ok:
				t.Errorf("%s has no attribute %s", resourceType, p)
			case !a.IsOptional() && !a.IsRequired():
				t.Errorf("%s.%s is not configurable", resourceType, p)
			}
		}
	}
}

func attributeAt(attrs map[string]schema.Attribute, attrPath string) (schema.Attribute, bool) {
	name, rest, nested := strings.Cut(attrPath, ".")
	a, ok := attrs[name]
	if !ok || !nested {
		return a, ok
	}
	switch n := a.(type) {
	case schema.ListNestedAttribute:
		return attributeAt(n.NestedObject.Attributes, rest)
	case schema.SingleNestedAttribute:
		return attributeAt(n.Attributes, rest)
	}
	return nil, false
}

// TestLibraryCollectionTypesMatchProviderValidator runs every collection type
// Jellyfin offers through jellyfin_library's validators, so the importer skips
// exactly the libraries the provider would reject.
func TestLibraryCollectionTypesMatchProviderValidator(t *testing.T) {
	ctx := t.Context()
	s, ok := providerSchemas(t)["jellyfin_library"]
	if !ok {
		t.Fatal("the provider has no jellyfin_library resource")
	}
	attr, ok := s.Attributes["collection_type"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("jellyfin_library.collection_type is %T, want a string attribute", s.Attributes["collection_type"])
	}

	// CollectionTypeOptions in the Jellyfin 12.1 API.
	jellyfinTypes := []string{"movies", "tvshows", "music", "musicvideos", "homevideos", "boxsets", "books", "mixed"}
	for _, collectionType := range jellyfinTypes {
		var resp validator.StringResponse
		for _, v := range attr.Validators {
			v.ValidateString(ctx, validator.StringRequest{
				Path:        path.Root("collection_type"),
				ConfigValue: types.StringValue(collectionType),
			}, &resp)
		}
		_, imported := provider.LibraryCollectionType(collectionType)
		if accepted := !resp.Diagnostics.HasError(); accepted != imported {
			t.Errorf("jellyfin_library accepts %q: %t, but the importer imports it: %t", collectionType, accepted, imported)
		}
	}
}

// configMatchesImportedState checks that the configuration of every resource
// the plan imports sets each attribute rendered accepts and the imported
// state holds a value for, to that value. Terraform plans no change for an
// attribute that is optional and computed and left out of the configuration,
// so an empty plan alone would not show that the importer missed one.
type configMatchesImportedState struct {
	schemas map[string]schema.Schema
}

func (c configMatchesImportedState) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	config := make(map[string]map[string]any)
	for _, r := range req.Plan.Config.RootModule.Resources {
		values := make(map[string]any, len(r.Expressions))
		for name, expr := range r.Expressions {
			if expr != nil && expr.ExpressionData != nil {
				values[name] = expr.ConstantValue
			}
		}
		config[r.Address] = values
	}

	var errs []error
	imported := 0
	for _, rc := range req.Plan.ResourceChanges {
		if rc.Change == nil || rc.Change.Importing == nil {
			continue
		}
		imported++
		s, ok := c.schemas[rc.Type]
		if !ok {
			errs = append(errs, fmt.Errorf("%s: the provider has no %s resource", rc.Address, rc.Type))
			continue
		}
		state, _ := rc.Change.Before.(map[string]any)
		errs = append(errs, compareConfigToState(rc.Type, rc.Address, "", s.Attributes, state, config[rc.Address])...)
	}
	if imported == 0 {
		errs = append(errs, errors.New("the plan imports no resources"))
	}
	resp.Error = errors.Join(errs...)
}

func compareConfigToState(resourceType, address, parent string, attrs map[string]schema.Attribute, state, config map[string]any) []error {
	var errs []error
	for name, a := range attrs {
		p := joinPath(parent, name)
		stateValue := state[name]
		if stateValue == nil || !rendered(resourceType, p, a, func(name string) bool { return state[name] == nil }) {
			continue
		}
		configValue, ok := config[name]
		if !ok || configValue == nil {
			errs = append(errs, fmt.Errorf("%s: %s is %s in the imported state but not set in the configuration", address, p, show(stateValue)))
			continue
		}

		switch a := a.(type) {
		case schema.SingleNestedAttribute:
			s, sok := stateValue.(map[string]any)
			c, cok := configValue.(map[string]any)
			if !sok || !cok {
				errs = append(errs, fmt.Errorf("%s: %s is %s in the configuration, want an object", address, p, show(configValue)))
				continue
			}
			errs = append(errs, compareConfigToState(resourceType, address, p, a.Attributes, s, c)...)
		case schema.ListNestedAttribute:
			s, sok := stateValue.([]any)
			c, cok := configValue.([]any)
			if !sok || !cok || len(s) != len(c) {
				errs = append(errs, fmt.Errorf("%s: %s is %s in the configuration, want %s", address, p, show(configValue), show(stateValue)))
				continue
			}
			for i := range s {
				se, _ := s[i].(map[string]any)
				ce, _ := c[i].(map[string]any)
				errs = append(errs, compareConfigToState(resourceType, address, p, a.NestedObject.Attributes, se, ce)...)
			}
		default:
			if !reflect.DeepEqual(normalizeNumbers(configValue), normalizeNumbers(stateValue)) {
				errs = append(errs, fmt.Errorf("%s: %s is %s in the configuration, want %s", address, p, show(configValue), show(stateValue)))
			}
		}
	}
	return errs
}

func show(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// normalizeNumbers turns json.Number values into float64, so a number reads
// the same whether Terraform rendered it from the configuration or the state.
func normalizeNumbers(v any) any {
	switch v := v.(type) {
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return v.String()
		}
		return f
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = normalizeNumbers(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = normalizeNumbers(e)
		}
		return out
	default:
		return v
	}
}
