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
	_ resource.Resource                = &BrandingConfigurationResource{}
	_ resource.ResourceWithImportState = &BrandingConfigurationResource{}
)

// NewBrandingConfigurationResource creates a new branding configuration resource.
func NewBrandingConfigurationResource() resource.Resource {
	return &BrandingConfigurationResource{}
}

// BrandingConfigurationResource defines the resource implementation.
type BrandingConfigurationResource struct {
	client *client.Client
}

// BrandingConfigurationResourceModel describes the resource data model.
type BrandingConfigurationResourceModel struct {
	ConfigurationJSON jsontypes.Normalized `tfsdk:"configuration_json"`
}

func (r *BrandingConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_branding_configuration"
}

func (r *BrandingConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jellyfin branding configuration. " +
			"Controls branding settings such as the splashscreen.",
		Attributes: map[string]schema.Attribute{
			"configuration_json": schema.StringAttribute{
				MarkdownDescription: "The branding configuration as a JSON string. " +
					"Supports settings like SplashscreenEnabled.",
				Required:   true,
				CustomType: jsontypes.NormalizedType{},
			},
		},
	}
}

func (r *BrandingConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BrandingConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data BrandingConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &client.BrandingConfiguration{RawJSON: data.ConfigurationJSON.ValueString()}
	if err := r.client.UpdateBrandingConfiguration(config); err != nil {
		resp.Diagnostics.AddError("Failed to update branding configuration", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BrandingConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data BrandingConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, err := r.client.GetBrandingConfiguration()
	if err != nil {
		resp.Diagnostics.AddError("Failed to read branding configuration", err.Error())
		return
	}

	normalized, err := normalizeJSON(config.RawJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize branding configuration", err.Error())
		return
	}

	data.ConfigurationJSON = jsontypes.NewNormalizedValue(normalized)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BrandingConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data BrandingConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &client.BrandingConfiguration{RawJSON: data.ConfigurationJSON.ValueString()}
	if err := r.client.UpdateBrandingConfiguration(config); err != nil {
		resp.Diagnostics.AddError("Failed to update branding configuration", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BrandingConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Branding configuration cannot be deleted. We just remove from state.
}

func (r *BrandingConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Singleton resource — the import ID is not used. Read will populate all fields.
	data := BrandingConfigurationResourceModel{
		ConfigurationJSON: jsontypes.NewNormalizedValue("{}"),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
