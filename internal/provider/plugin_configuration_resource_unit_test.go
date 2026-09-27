// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestUnitPluginIDPlan runs each plugin configuration resource's plugin_id
// plan modifiers in order, handing each the previous one's plan value as the
// framework does.
func TestUnitPluginIDPlan(t *testing.T) {
	ctx := context.Background()
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
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "jellyfin"}, &meta)
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
		attr, ok := schemaResp.Schema.Attributes["plugin_id"].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s: plugin_id is %T, want schema.StringAttribute", meta.TypeName, schemaResp.Schema.Attributes["plugin_id"])
		}

		// RequiresReplace skips a create or a destroy, which it tells apart by
		// a null state or plan object.
		typ, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
		if !ok {
			t.Fatalf("%s: schema type is not an object", meta.TypeName)
		}
		attrs := make(map[string]tftypes.Value, len(typ.AttributeTypes))
		for name, attrType := range typ.AttributeTypes {
			attrs[name] = tftypes.NewValue(attrType, nil)
		}
		object := tftypes.NewValue(typ, attrs)

		for _, c := range cases {
			t.Run(meta.TypeName+"/"+c.name, func(t *testing.T) {
				req := planmodifier.StringRequest{
					ConfigValue: c.config,
					StateValue:  c.state,
					PlanValue:   c.config,
					State:       tfsdk.State{Schema: schemaResp.Schema, Raw: object},
					Plan:        tfsdk.Plan{Schema: schemaResp.Schema, Raw: object},
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
