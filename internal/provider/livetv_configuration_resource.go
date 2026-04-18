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
	_ resource.Resource                = &LiveTVConfigurationResource{}
	_ resource.ResourceWithImportState = &LiveTVConfigurationResource{}
)

// NewLiveTVConfigurationResource creates a new Live TV configuration resource.
func NewLiveTVConfigurationResource() resource.Resource {
	return &LiveTVConfigurationResource{}
}

// LiveTVConfigurationResource defines the resource implementation.
type LiveTVConfigurationResource struct {
	client *client.Client
}

// LiveTVConfigurationResourceModel describes the resource data model.
type LiveTVConfigurationResourceModel struct {
	ID                types.String         `tfsdk:"id"`
	ConfigurationJSON jsontypes.Normalized `tfsdk:"configuration_json"`
}

func (r *LiveTVConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_livetv_configuration"
}

func (r *LiveTVConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jellyfin Live TV configuration. " +
			"Controls Live TV settings such as recording options, tuner hosts, and listing providers.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Resource identifier. Always set to `livetv` for this singleton resource.",
				Computed:            true,
			},
			"configuration_json": schema.StringAttribute{
				MarkdownDescription: "The Live TV configuration as a JSON string. " +
					"Supports settings like EnableRecordingSubfolders, PrePaddingSeconds, PostPaddingSeconds, " +
					"TunerHosts, ListingProviders, SaveRecordingNFO, and SaveRecordingImages.",
				Required:   true,
				CustomType: jsontypes.NormalizedType{},
			},
		},
	}
}

func (r *LiveTVConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *LiveTVConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data LiveTVConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &client.LiveTVConfiguration{RawJSON: data.ConfigurationJSON.ValueString()}
	if err := r.client.UpdateLiveTVConfiguration(config); err != nil {
		resp.Diagnostics.AddError("Failed to update Live TV configuration", err.Error())
		return
	}

	data.ID = types.StringValue("livetv")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LiveTVConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data LiveTVConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, err := r.client.GetLiveTVConfiguration()
	if err != nil {
		resp.Diagnostics.AddError("Failed to read Live TV configuration", err.Error())
		return
	}

	normalized, err := normalizeJSON(config.RawJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize Live TV configuration", err.Error())
		return
	}

	data.ID = types.StringValue("livetv")
	data.ConfigurationJSON = jsontypes.NewNormalizedValue(normalized)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LiveTVConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data LiveTVConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &client.LiveTVConfiguration{RawJSON: data.ConfigurationJSON.ValueString()}
	if err := r.client.UpdateLiveTVConfiguration(config); err != nil {
		resp.Diagnostics.AddError("Failed to update Live TV configuration", err.Error())
		return
	}

	data.ID = types.StringValue("livetv")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LiveTVConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Live TV configuration cannot be deleted. We just remove from state.
}

func (r *LiveTVConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Singleton resource — the import ID is not used. Read will populate all fields.
	data := LiveTVConfigurationResourceModel{
		ID:                types.StringValue("livetv"),
		ConfigurationJSON: jsontypes.NewNormalizedValue("{}"),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
