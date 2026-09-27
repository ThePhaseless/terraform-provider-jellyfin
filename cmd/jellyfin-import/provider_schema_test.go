// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/provider"
)

// omittedAttributes lists, by resource type, the configurable attributes the
// importer leaves out on purpose, as dotted paths through nested attributes.
var omittedAttributes = map[string][]string{
	"jellyfin_user":                     {"password", "policy"},
	"jellyfin_library":                  {"library_options"},
	"jellyfin_livetv_configuration":     {"media_locations_created", "listing_providers.password"},
	"jellyfin_networking_configuration": {"certificate_password"},
}

func isOmitted(resourceType, attrPath string) bool {
	for _, p := range omittedAttributes[resourceType] {
		if p == attrPath {
			return true
		}
	}
	return false
}

func isConfigurable(a schema.Attribute) bool {
	return a.IsOptional() || a.IsRequired()
}

func joinPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

func providerSchemas(t *testing.T) map[string]schema.Schema {
	t.Helper()

	ctx := context.Background()
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

func TestFieldTablesMatchProviderSchemas(t *testing.T) {
	schemas := providerSchemas(t)
	tables := map[string][]hclField{
		"jellyfin_scheduled_task":           scheduledTaskFields,
		"jellyfin_system_configuration":     systemFields,
		"jellyfin_encoding_configuration":   encodingFields,
		"jellyfin_networking_configuration": networkingFields,
		"jellyfin_branding_configuration":   brandingFields,
		"jellyfin_livetv_configuration":     livetvFields,
		"jellyfin_metadata_configuration":   metadataFields,
	}

	for resourceType, fields := range tables {
		t.Run(resourceType, func(t *testing.T) {
			s, ok := schemas[resourceType]
			if !ok {
				t.Fatalf("the provider has no %s resource", resourceType)
			}
			for _, err := range compareFieldsToSchema(resourceType, "", fields, s.Attributes) {
				t.Error(err)
			}
		})
	}
}

// TestLibraryCollectionTypesMatchProviderValidator runs every collection type
// Jellyfin offers through jellyfin_library's validators, so the importer skips
// exactly the libraries the provider would reject.
func TestLibraryCollectionTypesMatchProviderValidator(t *testing.T) {
	ctx := context.Background()
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
		if accepted := !resp.Diagnostics.HasError(); accepted != libraryCollectionTypes[collectionType] {
			t.Errorf("jellyfin_library accepts %q: %t, but libraryCollectionTypes says %t", collectionType, accepted, libraryCollectionTypes[collectionType])
		}
	}
	for collectionType := range libraryCollectionTypes {
		if !slices.Contains(jellyfinTypes, collectionType) {
			t.Errorf("libraryCollectionTypes has %q, which Jellyfin does not offer", collectionType)
		}
	}
}

// compareFieldsToSchema reports fields that name no configurable attribute
// or disagree with it on nesting, and configurable attributes that are
// neither generated nor listed in omittedAttributes.
func compareFieldsToSchema(resourceType, parent string, fields []hclField, attrs map[string]schema.Attribute) []error {
	var errs []error
	generated := make(map[string]bool, len(fields))
	for _, f := range fields {
		p := joinPath(parent, f.attr)
		generated[f.attr] = true
		a, ok := attrs[f.attr]
		switch {
		case !ok:
			errs = append(errs, fmt.Errorf("%s has no attribute %s", resourceType, p))
			continue
		case !isConfigurable(a):
			errs = append(errs, fmt.Errorf("%s.%s is not configurable", resourceType, p))
			continue
		}
		switch a := a.(type) {
		case schema.ListNestedAttribute:
			if f.nested == nil || f.object {
				errs = append(errs, fmt.Errorf("%s.%s is a list of objects", resourceType, p))
				continue
			}
			errs = append(errs, compareFieldsToSchema(resourceType, p, f.nested, a.NestedObject.Attributes)...)
		case schema.SingleNestedAttribute:
			if !f.object {
				errs = append(errs, fmt.Errorf("%s.%s is a single object", resourceType, p))
				continue
			}
			errs = append(errs, compareFieldsToSchema(resourceType, p, f.nested, a.Attributes)...)
		default:
			if f.nested != nil {
				errs = append(errs, fmt.Errorf("%s.%s is not a nested attribute", resourceType, p))
			}
		}
	}

	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := joinPath(parent, name)
		if generated[name] || !isConfigurable(attrs[name]) || isOmitted(resourceType, p) {
			continue
		}
		errs = append(errs, fmt.Errorf("%s.%s is neither generated nor listed in omittedAttributes", resourceType, p))
	}
	return errs
}

// configMatchesImportedState checks that the configuration of every resource
// the plan imports sets each configurable attribute the imported state holds
// a value for, to that value. Terraform plans no change for an attribute that
// is optional and computed and left out of the configuration, so an empty
// plan alone would not show that the importer missed one.
type configMatchesImportedState struct {
	schemas map[string]schema.Schema
}

func (c configMatchesImportedState) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	config := make(map[string]map[string]interface{})
	for _, r := range req.Plan.Config.RootModule.Resources {
		values := make(map[string]interface{}, len(r.Expressions))
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
		state, _ := rc.Change.Before.(map[string]interface{})
		errs = append(errs, compareConfigToState(rc.Type, rc.Address, "", s.Attributes, state, config[rc.Address])...)
	}
	if imported == 0 {
		errs = append(errs, errors.New("the plan imports no resources"))
	}
	resp.Error = errors.Join(errs...)
}

func compareConfigToState(resourceType, address, parent string, attrs map[string]schema.Attribute, state, config map[string]interface{}) []error {
	var errs []error
	for name, a := range attrs {
		p := joinPath(parent, name)
		stateValue := state[name]
		if stateValue == nil || !isConfigurable(a) || isOmitted(resourceType, p) {
			continue
		}
		configValue, ok := config[name]
		if !ok || configValue == nil {
			errs = append(errs, fmt.Errorf("%s: %s is %s in the imported state but not set in the configuration", address, p, show(stateValue)))
			continue
		}

		switch a := a.(type) {
		case schema.SingleNestedAttribute:
			s, sok := stateValue.(map[string]interface{})
			c, cok := configValue.(map[string]interface{})
			if !sok || !cok {
				errs = append(errs, fmt.Errorf("%s: %s is %s in the configuration, want an object", address, p, show(configValue)))
				continue
			}
			errs = append(errs, compareConfigToState(resourceType, address, p, a.Attributes, s, c)...)
		case schema.ListNestedAttribute:
			s, sok := stateValue.([]interface{})
			c, cok := configValue.([]interface{})
			if !sok || !cok || len(s) != len(c) {
				errs = append(errs, fmt.Errorf("%s: %s is %s in the configuration, want %s", address, p, show(configValue), show(stateValue)))
				continue
			}
			for i := range s {
				se, _ := s[i].(map[string]interface{})
				ce, _ := c[i].(map[string]interface{})
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

func show(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// normalizeNumbers turns json.Number values into float64, so a number reads
// the same whether Terraform rendered it from the configuration or the state.
func normalizeNumbers(v interface{}) interface{} {
	switch v := v.(type) {
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return v.String()
		}
		return f
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, e := range v {
			out[i] = normalizeNumbers(e)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for k, e := range v {
			out[k] = normalizeNumbers(e)
		}
		return out
	default:
		return v
	}
}
