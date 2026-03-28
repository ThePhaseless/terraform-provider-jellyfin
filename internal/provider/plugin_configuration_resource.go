// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &PluginConfigurationResource{}

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
	PluginID      types.String `tfsdk:"plugin_id"`
	Configuration types.String `tfsdk:"configuration_json"`
}

func (r *PluginConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin_configuration"
}

func (r *PluginConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages plugin configuration in Jellyfin. Configuration is passed as a JSON string, " +
			"allowing universal support for any plugin settings including SSO-Auth.",
		Attributes: map[string]schema.Attribute{
			"plugin_id": schema.StringAttribute{
				MarkdownDescription: "The plugin ID (GUID).",
				Required:            true,
			},
			"configuration_json": schema.StringAttribute{
				MarkdownDescription: "The plugin configuration as a JSON string. " +
					"For SSO-Auth, this would include SAML/OIDC configuration. " +
					"This allows universal configuration of any plugin.",
				Required: true,
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

	if err := r.client.UpdatePluginConfiguration(data.PluginID.ValueString(), data.Configuration.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to update plugin configuration", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PluginConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	configJSON, err := r.client.GetPluginConfiguration(data.PluginID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read plugin configuration", err.Error())
		return
	}

	data.Configuration = types.StringValue(configJSON)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PluginConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdatePluginConfiguration(data.PluginID.ValueString(), data.Configuration.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to update plugin configuration", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Plugin configuration cannot truly be deleted — it resets when the plugin is uninstalled.
	// We simply remove it from state.
}
