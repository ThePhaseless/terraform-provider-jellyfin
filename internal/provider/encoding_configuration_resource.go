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
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &EncodingConfigurationResource{}
	_ resource.ResourceWithImportState = &EncodingConfigurationResource{}
)

// NewEncodingConfigurationResource creates a new encoding configuration resource.
func NewEncodingConfigurationResource() resource.Resource {
	return &EncodingConfigurationResource{}
}

// EncodingConfigurationResource defines the resource implementation.
type EncodingConfigurationResource struct {
	client *client.Client
}

// EncodingConfigurationResourceModel describes the resource data model.
type EncodingConfigurationResourceModel struct {
	ID                types.String         `tfsdk:"id"`
	ConfigurationJSON jsontypes.Normalized `tfsdk:"configuration_json"`
}

func (r *EncodingConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_encoding_configuration"
}

func (r *EncodingConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jellyfin encoding/transcoding configuration. " +
			"The configuration is passed as a JSON string for full flexibility over all encoding options.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Resource identifier. Always set to `encoding` for this singleton resource.",
				Computed:            true,
			},
			"configuration_json": schema.StringAttribute{
				MarkdownDescription: "The encoding configuration as a JSON string.",
				Required:            true,
				CustomType:          jsontypes.NormalizedType{},
			},
		},
	}
}

func (r *EncodingConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *EncodingConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EncodingConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &client.EncodingOptions{RawJSON: data.ConfigurationJSON.ValueString()}
	if err := r.client.UpdateEncodingOptions(config); err != nil {
		resp.Diagnostics.AddError("Failed to update encoding configuration", err.Error())
		return
	}

	data.ID = types.StringValue("encoding")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EncodingConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EncodingConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, err := r.client.GetEncodingOptions()
	if err != nil {
		resp.Diagnostics.AddError("Failed to read encoding configuration", err.Error())
		return
	}

	normalized, err := normalizeJSON(config.RawJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize encoding configuration", err.Error())
		return
	}

	data.ID = types.StringValue("encoding")
	data.ConfigurationJSON = jsontypes.NewNormalizedValue(normalized)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EncodingConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data EncodingConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &client.EncodingOptions{RawJSON: data.ConfigurationJSON.ValueString()}
	if err := r.client.UpdateEncodingOptions(config); err != nil {
		resp.Diagnostics.AddError("Failed to update encoding configuration", err.Error())
		return
	}

	data.ID = types.StringValue("encoding")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EncodingConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Encoding configuration cannot be deleted. We just remove from state.
}

func (r *EncodingConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Singleton resource — the import ID is not used. Read will populate all fields.
	data := EncodingConfigurationResourceModel{
		ID:                types.StringValue("encoding"),
		ConfigurationJSON: jsontypes.NewNormalizedValue("{}"),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
