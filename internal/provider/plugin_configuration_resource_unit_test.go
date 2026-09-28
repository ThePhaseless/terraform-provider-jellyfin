// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

// TestUnitPluginIDPlan runs each plugin configuration resource's plugin_id
// plan modifiers in order, handing each the previous one's plan value as the
// framework does.
func TestUnitPluginIDPlan(t *testing.T) {
	ctx := t.Context()
	const dashed = "94879a0c-da24-4eb1-aa06-f28b4b9333b1"

	cases := []struct {
		name    string
		state   types.String
		config  types.String
		plan    types.String
		replace bool
	}{
		{"dash-free spelling of the GUID in state", types.StringValue(dashed), types.StringValue("94879a0cda244eb1aa06f28b4b9333b1"), types.StringValue(dashed), false},
		{"dashed spelling of the GUID in state", types.StringValue("94879a0cda244eb1aa06f28b4b9333b1"), types.StringValue(dashed), types.StringValue("94879a0cda244eb1aa06f28b4b9333b1"), false},
		{"upper-case spelling of the GUID in state", types.StringValue(dashed), types.StringValue("94879A0C-DA24-4EB1-AA06-F28B4B9333B1"), types.StringValue(dashed), false},
		{"different GUID", types.StringValue(dashed), types.StringValue("505ce9d1d91642fa86ca673ef241d7df"), types.StringValue("505ce9d1d91642fa86ca673ef241d7df"), true},
		{"unknown until apply", types.StringValue(dashed), types.StringUnknown(), types.StringUnknown(), true},
		{"create", types.StringNull(), types.StringValue(dashed), types.StringValue(dashed), false},
	}

	for _, r := range []resource.Resource{NewPluginConfigurationResource(), &JellyfinSecurityPluginConfigurationResource{}} {
		typeName := resourceTypeName(ctx, r)
		resourceSchema := schemaOf(r)
		attr, ok := resourceSchema.Attributes["plugin_id"].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s: plugin_id is %T, want schema.StringAttribute", typeName, resourceSchema.Attributes["plugin_id"])
		}

		// RequiresReplace skips a create or a destroy, which it tells apart by
		// a null state or plan object.
		typ, ok := resourceSchema.Type().TerraformType(ctx).(tftypes.Object)
		if !ok {
			t.Fatalf("%s: schema type is not an object", typeName)
		}
		attrs := make(map[string]tftypes.Value, len(typ.AttributeTypes))
		for name, attrType := range typ.AttributeTypes {
			attrs[name] = tftypes.NewValue(attrType, nil)
		}
		object := tftypes.NewValue(typ, attrs)

		for _, c := range cases {
			t.Run(typeName+"/"+c.name, func(t *testing.T) {
				req := planmodifier.StringRequest{
					ConfigValue: c.config,
					StateValue:  c.state,
					PlanValue:   c.config,
					State:       tfsdk.State{Schema: resourceSchema, Raw: object},
					Plan:        tfsdk.Plan{Schema: resourceSchema, Raw: object},
				}
				if c.state.IsNull() {
					req.State.Raw = tftypes.NewValue(typ, nil)
				}
				replace := false
				for _, m := range attr.PlanModifiers {
					resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
					m.PlanModifyString(ctx, req, resp)
					if resp.Diagnostics.HasError() {
						t.Fatalf("plan modifier: %v", resp.Diagnostics.Errors())
					}
					req.PlanValue = resp.PlanValue
					replace = replace || resp.RequiresReplace
				}
				if !req.PlanValue.Equal(c.plan) || replace != c.replace {
					t.Errorf("plan = %v, replace %t; want %v, replace %t", req.PlanValue, replace, c.plan, c.replace)
				}
			})
		}
	}
}

// Jellyfin replaces a plugin's configuration with the one posted and serves
// every key back, those the configuration leaves out with their defaults; a
// configuration that names some keys reads back as itself.
func TestUnitPluginConfigurationReadsBackTheKeysItManages(t *testing.T) {
	ctx := t.Context()
	const defaults = `{"Server": "https://musicbrainz.org", "RateLimit": 1, "ReplaceArtistName": false}`
	stored := defaults
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, stored)
		case http.MethodPost:
			var posted map[string]any
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Error(err)
			}
			var merged map[string]any
			if err := json.Unmarshal([]byte(defaults), &merged); err != nil {
				t.Fatal(err)
			}
			maps.Copy(merged, posted)
			b, _ := json.Marshal(merged)
			stored = string(b)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	r := &PluginConfigurationResource{client: client.NewClient(srv.URL, "k")}
	s := schemaOf(r)
	null := tftypes.NewValue(s.Type().TerraformType(ctx), nil)
	configured := `{"Server": "https://mb.example"}`

	plan := tfsdk.Plan{Schema: s, Raw: null}
	if d := plan.Set(ctx, &PluginConfigurationResourceModel{ID: types.StringUnknown(), PluginID: types.StringValue("mb"), Configuration: jsontypes.NewNormalizedValue(configured)}); d.HasError() {
		t.Fatal(d)
	}
	created := resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: null}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	read := resource.ReadResponse{State: created.State}
	r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	var got PluginConfigurationResourceModel
	if d := read.State.Get(ctx, &got); d.HasError() {
		t.Fatal(d)
	}
	if equal, d := got.Configuration.StringSemanticEquals(ctx, jsontypes.NewNormalizedValue(configured)); d.HasError() || !equal {
		t.Errorf("refreshed configuration_json = %s, want the configured %s", got.Configuration.ValueString(), configured)
	}

	// An import has no configuration to follow and reads every key.
	imported := tfsdk.State{Schema: s, Raw: null}
	if d := imported.SetAttribute(ctx, path.Root("plugin_id"), types.StringValue("mb")); d.HasError() {
		t.Fatal(d)
	}
	read = resource.ReadResponse{State: imported}
	r.Read(ctx, resource.ReadRequest{State: imported}, &read)
	if d := read.State.Get(ctx, &got); d.HasError() || !strings.Contains(got.Configuration.ValueString(), "RateLimit") {
		t.Errorf("imported configuration_json = %s (%v), want every served key", got.Configuration.ValueString(), d)
	}
}
