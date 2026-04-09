// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

var (
	_ resource.Resource                = &NetworkingConfigurationResource{}
	_ resource.ResourceWithImportState = &NetworkingConfigurationResource{}
)

// NewNetworkingConfigurationResource creates a new networking configuration resource.
func NewNetworkingConfigurationResource() resource.Resource {
	return &NetworkingConfigurationResource{}
}

// NetworkingConfigurationResource defines the resource implementation.
type NetworkingConfigurationResource struct {
	client *client.Client
}

// NetworkingConfigurationResourceModel describes the resource data model.
type NetworkingConfigurationResourceModel struct {
	ConfigurationJSON jsontypes.Normalized `tfsdk:"configuration_json"`
}

func (r *NetworkingConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_networking_configuration"
}

func (r *NetworkingConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jellyfin networking configuration. " +
			"Controls network settings including HTTPS, ports, remote access, proxy settings, and IP filtering. " +
			"The configuration is passed as a JSON string for full flexibility.",
		Attributes: map[string]schema.Attribute{
			"configuration_json": schema.StringAttribute{
				MarkdownDescription: "The networking configuration as a JSON string. Supports all Jellyfin network settings " +
					"including BaseUrl, EnableHttps, RequireHttps, CertificatePath, InternalHttpPort, PublicHttpPort, " +
					"EnableRemoteAccess, KnownProxies, RemoteIPFilter, and more.",
				Required:   true,
				CustomType: jsontypes.NormalizedType{},
			},
		},
	}
}

func (r *NetworkingConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NetworkingConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NetworkingConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &client.NetworkConfiguration{RawJSON: data.ConfigurationJSON.ValueString()}
	if err := r.client.UpdateNetworkConfiguration(config); err != nil {
		resp.Diagnostics.AddError("Failed to update networking configuration", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkingConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NetworkingConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, err := r.client.GetNetworkConfiguration()
	if err != nil {
		resp.Diagnostics.AddError("Failed to read networking configuration", err.Error())
		return
	}

	normalized, err := normalizeJSON(config.RawJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize networking configuration", err.Error())
		return
	}

	data.ConfigurationJSON = jsontypes.NewNormalizedValue(normalized)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkingConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NetworkingConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &client.NetworkConfiguration{RawJSON: data.ConfigurationJSON.ValueString()}
	if err := r.client.UpdateNetworkConfiguration(config); err != nil {
		resp.Diagnostics.AddError("Failed to update networking configuration", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkingConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Networking configuration cannot be deleted. We just remove from state.
}

func (r *NetworkingConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Singleton resource — the import ID is not used. Read will populate all fields.
	data := NetworkingConfigurationResourceModel{
		ConfigurationJSON: jsontypes.NewNormalizedValue("{}"),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
