// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

var (
	_ resource.Resource                = &MetadataConfigurationResource{}
	_ resource.ResourceWithImportState = &MetadataConfigurationResource{}
	_ resource.ResourceWithModifyPlan  = &MetadataConfigurationResource{}
	_ wireBound                        = &MetadataConfigurationResource{}
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

var metadataWire = sync.OnceValues(func() (*wire.Binding, error) {
	return wire.Bind(schemaOf(&MetadataConfigurationResource{}), "MetadataConfiguration",
		wire.Identity("id"))
})

func (r *MetadataConfigurationResource) Wire() (*wire.Binding, error) { return metadataWire() }

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
			"use_file_creation_time_for_date_added": optionalBool("Whether to use file creation time for date added."),
		},
	}
}

func (r *MetadataConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
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
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue("metadata"))...)
}

// ModifyPlan gates each configured field on the Jellyfin version it needs, so
// a field a later pin adds is checked without a change here.
func (r *MetadataConfigurationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	if b := wireBinding(&resp.Diagnostics, metadataWire); b != nil {
		resp.Diagnostics.Append(checkServerHasFields(ctx, r.client, b, req.Config)...)
	}
}

func (r *MetadataConfigurationResource) apply(ctx context.Context, data *MetadataConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	b := wireBinding(diags, metadataWire)
	if b == nil || !r.document().write(ctx, b, data, diags) {
		return
	}
	data.ID = types.StringValue("metadata")
	diags.Append(state.Set(ctx, data)...)
}

func (r *MetadataConfigurationResource) read(ctx context.Context, data *MetadataConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	b := wireBinding(diags, metadataWire)
	if b == nil || !r.document().read(ctx, b, data, diags) {
		return
	}
	data.ID = types.StringValue("metadata")
	diags.Append(state.Set(ctx, data)...)
}

func (r *MetadataConfigurationResource) document() document {
	return document{what: "metadata configuration", get: r.client.GetMetadataConfiguration, put: r.client.UpdateMetadataConfiguration}
}
