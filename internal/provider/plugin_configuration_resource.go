// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
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
					"This allows universal configuration of any plugin.",
				MarkdownDescription: "The plugin configuration as a JSON string. " +
					"For SSO-Auth, this would include SAML/OIDC configuration. " +
					"This allows universal configuration of any plugin.",
				Required:   true,
				CustomType: jsontypes.NormalizedType{},
			},
		},
	}
}

func (r *PluginConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}

	r.client = c
}

func (r *PluginConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PluginConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdatePluginConfiguration(ctx, data.PluginID.ValueString(), data.Configuration.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to update plugin configuration", err.Error())
		return
	}

	data.ID = data.PluginID
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
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

	normalized, err := normalizeJSON(configJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize plugin configuration", err.Error())
		return
	}

	data.Configuration = jsontypes.NewNormalizedValue(normalized)
	data.ID = data.PluginID

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PluginConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdatePluginConfiguration(ctx, data.PluginID.ValueString(), data.Configuration.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to update plugin configuration", err.Error())
		return
	}

	configJSON, err := r.client.GetPluginConfiguration(ctx, data.PluginID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read plugin configuration after update", err.Error())
		return
	}
	normalized, err := normalizeJSON(configJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize plugin configuration", err.Error())
		return
	}
	data.Configuration = jsontypes.NewNormalizedValue(normalized)
	data.ID = data.PluginID

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
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

// samePluginGUIDPlanModifier plans the plugin_id in state when the
// configuration spells the same GUID another way. jellyfin_plugin's id and GET
// /Plugins use the dash-free spelling while a GUID copied from elsewhere, such
// as an import ID, often has dashes, and without this a change of spelling
// would replace the resource. Terraform accepts a prior value in place of a
// configured one the provider treats as equal.
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
