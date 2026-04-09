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
	_ resource.Resource                = &MetadataConfigurationResource{}
	_ resource.ResourceWithImportState = &MetadataConfigurationResource{}
)

// NewMetadataConfigurationResource creates a new metadata configuration resource.
func NewMetadataConfigurationResource() resource.Resource {
	return &MetadataConfigurationResource{}
}

// MetadataConfigurationResource defines the resource implementation.
type MetadataConfigurationResource struct {
	client *client.Client
}

// MetadataConfigurationResourceModel describes the resource data model.
type MetadataConfigurationResourceModel struct {
	ConfigurationJSON jsontypes.Normalized `tfsdk:"configuration_json"`
}

func (r *MetadataConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_metadata_configuration"
}

func (r *MetadataConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jellyfin metadata configuration. " +
			"Controls metadata settings such as how file creation time is used for date added.",
		Attributes: map[string]schema.Attribute{
			"configuration_json": schema.StringAttribute{
				MarkdownDescription: "The metadata configuration as a JSON string. " +
					"Supports settings like UseFileCreationTimeForDateAdded.",
				Required:   true,
				CustomType: jsontypes.NormalizedType{},
			},
		},
	}
}

func (r *MetadataConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MetadataConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data MetadataConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &client.MetadataConfiguration{RawJSON: data.ConfigurationJSON.ValueString()}
	if err := r.client.UpdateMetadataConfiguration(config); err != nil {
		resp.Diagnostics.AddError("Failed to update metadata configuration", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MetadataConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data MetadataConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, err := r.client.GetMetadataConfiguration()
	if err != nil {
		resp.Diagnostics.AddError("Failed to read metadata configuration", err.Error())
		return
	}

	normalized, err := normalizeJSON(config.RawJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize metadata configuration", err.Error())
		return
	}

	data.ConfigurationJSON = jsontypes.NewNormalizedValue(normalized)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MetadataConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data MetadataConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &client.MetadataConfiguration{RawJSON: data.ConfigurationJSON.ValueString()}
	if err := r.client.UpdateMetadataConfiguration(config); err != nil {
		resp.Diagnostics.AddError("Failed to update metadata configuration", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MetadataConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Metadata configuration cannot be deleted. We just remove from state.
}

func (r *MetadataConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Singleton resource — the import ID is not used. Read will populate all fields.
	data := MetadataConfigurationResourceModel{
		ConfigurationJSON: jsontypes.NewNormalizedValue("{}"),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
