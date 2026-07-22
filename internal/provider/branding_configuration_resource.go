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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
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
	ID types.String `tfsdk:"id"`
	LoginDisclaimer types.String `tfsdk:"login_disclaimer"`
	CustomCss types.String `tfsdk:"custom_css"`
	SplashscreenEnabled types.Bool `tfsdk:"splashscreen_enabled"`
	SplashscreenLocation types.String `tfsdk:"splashscreen_location"`
}

func (r *BrandingConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_branding_configuration"
}

func (r *BrandingConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages the Jellyfin branding configuration.",
		MarkdownDescription: "Manages the Jellyfin branding configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "Resource identifier. Always set to `branding` for this singleton resource.",
				MarkdownDescription: "Resource identifier. Always set to `branding` for this singleton resource.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"login_disclaimer": schema.StringAttribute{Description: "The login disclaimer text.", MarkdownDescription: "The login disclaimer text.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"custom_css": schema.StringAttribute{Description: "Custom CSS content.", MarkdownDescription: "Custom CSS content.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"splashscreen_enabled": schema.BoolAttribute{Description: "Whether the splash screen is enabled.", MarkdownDescription: "Whether the splash screen is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"splashscreen_location": schema.StringAttribute{Description: "The splash screen location.", MarkdownDescription: "The splash screen location.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
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

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *BrandingConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data BrandingConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *BrandingConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data BrandingConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *BrandingConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Branding configuration cannot be deleted. We just remove from state.
}

func (r *BrandingConfigurationResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Singleton resource — the import ID is not used. Read will populate all fields.
	data := BrandingConfigurationResourceModel{ID: types.StringValue("branding")}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BrandingConfigurationResource) apply(ctx context.Context, data *BrandingConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetBrandingConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read current branding configuration", err.Error())
		return
	}

	base, err := parseJSONObject(current.RawJSON)
	if err != nil {
		diags.AddError("Failed to parse current branding configuration", err.Error())
		return
	}

	d := overlayBrandingConfiguration(ctx, base, data)
	if d.HasError() {
		diags.Append(d...)
		return
	}

	payload, err := json.Marshal(base)
	if err != nil {
		diags.AddError("Failed to serialize branding configuration", err.Error())
		return
	}

	if err := r.client.UpdateBrandingConfiguration(ctx, &client.BrandingConfiguration{RawJSON: string(payload)}); err != nil {
		diags.AddError("Failed to update branding configuration", err.Error())
		return
	}

	updated, err := r.client.GetBrandingConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read branding configuration after update", err.Error())
		return
	}

	flattenBrandingConfiguration(ctx, updated.RawJSON, data, diags)
	data.ID = types.StringValue("branding")
	diags.Append(state.Set(ctx, data)...)
}

func (r *BrandingConfigurationResource) read(ctx context.Context, data *BrandingConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetBrandingConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read branding configuration", err.Error())
		return
	}

	flattenBrandingConfiguration(ctx, current.RawJSON, data, diags)
	data.ID = types.StringValue("branding")
	diags.Append(state.Set(ctx, data)...)
}

func overlayBrandingConfiguration(ctx context.Context, m map[string]json.RawMessage, data *BrandingConfigurationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	putJSONString(m, "LoginDisclaimer", data.LoginDisclaimer)
	putJSONString(m, "CustomCss", data.CustomCss)
	putJSONBool(m, "SplashscreenEnabled", data.SplashscreenEnabled)
	putJSONString(m, "SplashscreenLocation", data.SplashscreenLocation)
	return diags
}

func flattenBrandingConfiguration(ctx context.Context, raw string, data *BrandingConfigurationResourceModel, diags *diag.Diagnostics) {
	m, err := parseJSONObject(raw)
	if err != nil {
		diags.AddError("Failed to parse branding configuration", err.Error())
		return
	}
	data.LoginDisclaimer = getJSONString(m, "LoginDisclaimer")
	data.CustomCss = getJSONString(m, "CustomCss")
	data.SplashscreenEnabled = getJSONBool(m, "SplashscreenEnabled")
	data.SplashscreenLocation = getJSONString(m, "SplashscreenLocation")
}
