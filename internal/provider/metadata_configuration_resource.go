// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
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
	ID                              types.String `tfsdk:"id"`
	UseFileCreationTimeForDateAdded types.Bool   `tfsdk:"use_file_creation_time_for_date_added"`
}

func (r *MetadataConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_metadata_configuration"
}

func (r *MetadataConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages the Jellyfin metadata configuration.",
		MarkdownDescription: "Manages the Jellyfin metadata configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "Resource identifier. Always set to `metadata` for this singleton resource.",
				MarkdownDescription: "Resource identifier. Always set to `metadata` for this singleton resource.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"use_file_creation_time_for_date_added": schema.BoolAttribute{Description: "Whether to use file creation time for date added.", MarkdownDescription: "Whether to use file creation time for date added.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
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

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *MetadataConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data MetadataConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *MetadataConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data MetadataConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *MetadataConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Metadata configuration cannot be deleted. We just remove from state.
}

func (r *MetadataConfigurationResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Singleton resource — the import ID is not used. Read will populate all fields.
	data := MetadataConfigurationResourceModel{ID: types.StringValue("metadata")}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MetadataConfigurationResource) apply(ctx context.Context, data *MetadataConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetMetadataConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read current metadata configuration", err.Error())
		return
	}

	base, err := parseJSONObject(current.RawJSON)
	if err != nil {
		diags.AddError("Failed to parse current metadata configuration", err.Error())
		return
	}

	overlayMetadataConfiguration(ctx, base, data)

	payload, err := json.Marshal(base)
	if err != nil {
		diags.AddError("Failed to serialize metadata configuration", err.Error())
		return
	}

	if err := r.client.UpdateMetadataConfiguration(ctx, &client.MetadataConfiguration{RawJSON: string(payload)}); err != nil {
		diags.AddError("Failed to update metadata configuration", err.Error())
		return
	}

	updated, err := r.client.GetMetadataConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read metadata configuration after update", err.Error())
		return
	}

	flattenMetadataConfiguration(ctx, updated.RawJSON, data, diags)
	data.ID = types.StringValue("metadata")
	diags.Append(state.Set(ctx, data)...)
}

func (r *MetadataConfigurationResource) read(ctx context.Context, data *MetadataConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetMetadataConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read metadata configuration", err.Error())
		return
	}

	flattenMetadataConfiguration(ctx, current.RawJSON, data, diags)
	data.ID = types.StringValue("metadata")
	diags.Append(state.Set(ctx, data)...)
}

func overlayMetadataConfiguration(_ context.Context, m map[string]json.RawMessage, data *MetadataConfigurationResourceModel) {
	putJSONBool(m, "UseFileCreationTimeForDateAdded", data.UseFileCreationTimeForDateAdded)
}

func flattenMetadataConfiguration(_ context.Context, raw string, data *MetadataConfigurationResourceModel, diags *diag.Diagnostics) {
	m, err := parseJSONObject(raw)
	if err != nil {
		diags.AddError("Failed to parse metadata configuration", err.Error())
		return
	}
	data.UseFileCreationTimeForDateAdded = getJSONBool(m, "UseFileCreationTimeForDateAdded")
}
