// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

var (
	_ resource.Resource                = &PluginConfigurationResource{}
	_ resource.ResourceWithImportState = &PluginConfigurationResource{}
)

// NewPluginConfigurationResource creates a new plugin configuration resource.
func NewPluginConfigurationResource() resource.Resource {
	return &PluginConfigurationResource{}
}

// PluginConfigurationResource defines the resource implementation.
type PluginConfigurationResource struct {
	client *client.Client
}

// PluginConfigurationResourceModel describes the resource data model.
type PluginConfigurationResourceModel struct {
	ID            types.String         `tfsdk:"id"`
	PluginID      types.String         `tfsdk:"plugin_id"`
	Configuration jsontypes.Normalized `tfsdk:"configuration_json"`
}

func (r *PluginConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin_configuration"
}

func (r *PluginConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages plugin configuration in Jellyfin. Configuration is passed as a JSON string, " +
			"allowing universal support for any plugin settings including SSO-Auth.",
		MarkdownDescription: "Manages plugin configuration in Jellyfin. Configuration is passed as a JSON string, " +
			"allowing universal support for any plugin settings including SSO-Auth.",
		Attributes: map[string]schema.Attribute{
			"plugin_id": pluginIDAttribute(),
			"id": schema.StringAttribute{
				Description:         "The plugin configuration resource identifier.",
				MarkdownDescription: "The plugin configuration resource identifier.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"configuration_json": schema.StringAttribute{
				Description: "The plugin configuration as a JSON string. " +
					"For SSO-Auth, this would include SAML/OIDC configuration. " +
					"This allows universal configuration of any plugin. " +
					"Jellyfin replaces the plugin's whole configuration with it, so a key it leaves out takes the plugin's default, " +
					"and only the keys it names are compared with what the server holds: a key added outside Terraform, " +
					"such as an SSO provider added on the plugin's page, plans no change, and the next update removes it. " +
					"An import reads every key.",
				MarkdownDescription: "The plugin configuration as a JSON string. " +
					"For SSO-Auth, this would include SAML/OIDC configuration. " +
					"This allows universal configuration of any plugin. " +
					"Jellyfin replaces the plugin's whole configuration with it, so a key it leaves out takes the plugin's default, " +
					"and only the keys it names are compared with what the server holds: a key added outside Terraform, " +
					"such as an SSO provider added on the plugin's page, plans no change, and the next update removes it. " +
					"An import reads every key.",
				Required:   true,
				CustomType: jsontypes.NormalizedType{},
			},
		},
	}
}

func (r *PluginConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *PluginConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.write(ctx, req.Plan, &resp.Diagnostics, &resp.State)
}

func (r *PluginConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PluginConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	configJSON, err := r.client.GetPluginConfiguration(ctx, data.PluginID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read plugin configuration", err.Error())
		return
	}

	// An import has no configuration yet, and reads the whole of it.
	served, err := servedConfiguration(configJSON, data.Configuration.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read plugin configuration", err.Error())
		return
	}

	data.Configuration = jsontypes.NewNormalizedValue(served)
	data.ID = data.PluginID

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.write(ctx, req.Plan, &resp.Diagnostics, &resp.State)
}

// write posts the planned configuration and reads back what the server keeps
// of it.
func (r *PluginConfigurationResource) write(ctx context.Context, plan tfsdk.Plan, diags *diag.Diagnostics, state *tfsdk.State) {
	var data PluginConfigurationResourceModel
	diags.Append(plan.Get(ctx, &data)...)
	if diags.HasError() {
		return
	}

	if err := r.client.UpdatePluginConfiguration(ctx, data.PluginID.ValueString(), data.Configuration.ValueString()); err != nil {
		diags.AddError("Failed to update plugin configuration", err.Error())
		return
	}

	configJSON, err := r.client.GetPluginConfiguration(ctx, data.PluginID.ValueString())
	if err != nil {
		diags.AddError("Failed to read plugin configuration after update", err.Error())
		return
	}
	served, err := servedConfiguration(configJSON, data.Configuration.ValueString())
	if err != nil {
		diags.AddError("Failed to read plugin configuration after update", err.Error())
		return
	}
	data.Configuration = jsontypes.NewNormalizedValue(served)
	data.ID = data.PluginID

	diags.Append(state.Set(ctx, &data)...)
}

// servedConfiguration returns the normalized configuration served holds,
// keeping only the keys managed has at each level of its objects. Jellyfin
// replaces a plugin's whole configuration with the one posted and serves
// every key back, a key the configuration leaves out with its default, so
// only the keys the configuration names say whether the server kept it. An
// empty managed, as on an import, keeps every key.
func servedConfiguration(served, managed string) (string, error) {
	if managed == "" {
		return normalizeJSON(served)
	}
	servedValue, err := decodeJSON(served)
	if err != nil {
		return "", fmt.Errorf("parsing the served configuration: %w", err)
	}
	managedValue, err := decodeJSON(managed)
	if err != nil {
		return "", fmt.Errorf("parsing the configuration: %w", err)
	}
	kept, err := json.Marshal(keysOf(servedValue, managedValue))
	if err != nil {
		return "", err
	}
	return normalizeJSON(string(kept))
}

func decodeJSON(raw string) (any, error) {
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// keysOf returns served with only the object keys shape has, at any depth,
// following lists of the same length element by element.
func keysOf(served, shape any) any {
	switch s := shape.(type) {
	case map[string]any:
		m, ok := served.(map[string]any)
		if !ok {
			return served
		}
		out := make(map[string]any, len(s))
		for k, v := range s {
			if sv, ok := m[k]; ok {
				out[k] = keysOf(sv, v)
			}
		}
		return out
	case []any:
		l, ok := served.([]any)
		if !ok || len(l) != len(s) {
			return served
		}
		out := make([]any, len(l))
		for i := range l {
			out[i] = keysOf(l[i], s[i])
		}
		return out
	}
	return served
}

func (r *PluginConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Plugin configuration cannot truly be deleted — it resets when the plugin is uninstalled.
	// We simply remove it from state.
}

func (r *PluginConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("plugin_id"), req, resp)
}

func pluginIDAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Description:         "The plugin ID (GUID), with or without dashes. Both spellings name the same plugin, so switching between them plans no change.",
		MarkdownDescription: "The plugin ID (GUID), with or without dashes. Both spellings name the same plugin, so switching between them plans no change.",
		Required:            true,
		Validators:          requiredIdentifierValidators(),
		PlanModifiers: []planmodifier.String{
			samePluginGUIDPlanModifier{},
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// normalizeGUID returns a lowercase, dash-free GUID so that the provider can
// compare IDs regardless of whether Jellyfin returns them as "D" or "N" format.
func normalizeGUID(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "-", ""))
}

// samePluginGUIDPlanModifier plans the state's plugin_id when the configuration
// spells the same GUID another way, so a dashed GUID, as in an import ID, does
// not replace the resource.
type samePluginGUIDPlanModifier struct{}

func (samePluginGUIDPlanModifier) Description(context.Context) string {
	return "Keeps the plugin_id in state when the configuration spells the same GUID another way."
}

func (m samePluginGUIDPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (samePluginGUIDPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	if normalizeGUID(req.PlanValue.ValueString()) == normalizeGUID(req.StateValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}

// normalizeJSON re-encodes JSON to remove insignificant formatting and sort object keys.
func normalizeJSON(raw string) (string, error) {
	normalized, err := normalizeJSONRecursive(json.RawMessage(raw), 0)
	if err != nil {
		return "", fmt.Errorf("parsing JSON for normalization: %w", err)
	}
	return string(normalized), nil
}

const maxJSONNormalizeDepth = 100

func normalizeJSONRecursive(raw json.RawMessage, depth int) (json.RawMessage, error) {
	if depth > maxJSONNormalizeDepth {
		return nil, fmt.Errorf("JSON nesting exceeds maximum depth of %d", maxJSONNormalizeDepth)
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, err
	}

	trimmed := bytes.TrimSpace(compact.Bytes())
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty JSON value")
	}

	switch trimmed[0] {
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &object); err != nil {
			return nil, err
		}
		for key, value := range object {
			normalized, err := normalizeJSONRecursive(value, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = normalized
		}
		return json.Marshal(object)
	case '[':
		var list []json.RawMessage
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return nil, err
		}
		for i, value := range list {
			normalized, err := normalizeJSONRecursive(value, depth+1)
			if err != nil {
				return nil, err
			}
			list[i] = normalized
		}
		return json.Marshal(list)
	}

	var rawValue json.RawMessage
	if err := json.Unmarshal(trimmed, &rawValue); err != nil {
		return nil, err
	}
	return json.Marshal(rawValue)
}
