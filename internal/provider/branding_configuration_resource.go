// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

var (
	_ resource.Resource                = &BrandingConfigurationResource{}
	_ resource.ResourceWithImportState = &BrandingConfigurationResource{}
	_ resource.ResourceWithModifyPlan  = &BrandingConfigurationResource{}
	_ wireBound                        = &BrandingConfigurationResource{}
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
	ID                   types.String `tfsdk:"id"`
	LoginDisclaimer      types.String `tfsdk:"login_disclaimer"`
	CustomCSS            types.String `tfsdk:"custom_css"`
	SplashscreenEnabled  types.Bool   `tfsdk:"splashscreen_enabled"`
	SplashscreenLocation types.String `tfsdk:"splashscreen_location"`
}

var brandingWire = sync.OnceValues(func() (*wire.Binding, error) {
	return wire.Bind(schemaOf(&BrandingConfigurationResource{}), "BrandingOptionsDto",
		wire.Identity("id"),
		wire.NeverSent("splashscreen_location", splashscreenLocationUnsupportedMessage))
})

func (r *BrandingConfigurationResource) Wire() (*wire.Binding, error) { return brandingWire() }

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
			"login_disclaimer":      schema.StringAttribute{Description: "The login disclaimer text.", MarkdownDescription: "The login disclaimer text.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"custom_css":            schema.StringAttribute{Description: "Custom CSS content.", MarkdownDescription: "Custom CSS content.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"splashscreen_enabled":  schema.BoolAttribute{Description: "Whether the splash screen is enabled.", MarkdownDescription: "Whether the splash screen is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"splashscreen_location": schema.StringAttribute{Description: "The splash screen location. " + splashscreenLocationUnsupportedMessage, MarkdownDescription: "The splash screen location. " + splashscreenLocationUnsupportedMessage, Optional: true, DeprecationMessage: splashscreenLocationUnsupportedMessage, Validators: []validator.String{splashscreenLocationValidator{}}},
		},
	}
}

const splashscreenLocationUnsupportedMessage = "Jellyfin ignores a splash screen location in the branding configuration, so setting it is an error. The attribute will be removed in a future release."

// splashscreenLocationValidator rejects a configured splashscreen_location at
// plan time. Jellyfin 10.11 and 12 drop the key and read it back as null,
// which Terraform reports only as an inconsistent result after apply.
type splashscreenLocationValidator struct{}

func (splashscreenLocationValidator) Description(context.Context) string {
	return "must not be set, because Jellyfin ignores the splash screen location"
}

func (v splashscreenLocationValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (splashscreenLocationValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Unsupported branding option",
		"Jellyfin ignores a splash screen location in the branding configuration, so the server would drop this value. Remove it from the configuration.")
}

func (r *BrandingConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
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
	// Singleton resource, so the import ID is not used. Set only the id: the
	// framework types every other attribute from the schema, and the Read that
	// follows an import fills them.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue("branding"))...)
}

// ModifyPlan gates each configured field on the Jellyfin version it needs, so
// a field a later pin adds is checked without a change here.
func (r *BrandingConfigurationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	if b := wireBinding(&resp.Diagnostics, brandingWire); b != nil {
		resp.Diagnostics.Append(checkServerHasFields(ctx, r.client, b, req.Config)...)
	}
}

func (r *BrandingConfigurationResource) apply(ctx context.Context, data *BrandingConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	b := wireBinding(diags, brandingWire)
	if b == nil {
		return
	}

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

	if d := b.OverlayModel(ctx, base, data); d.HasError() {
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

	diags.Append(b.FlattenAfterApply(ctx, updated.RawJSON, data)...)
	data.ID = types.StringValue("branding")
	diags.Append(state.Set(ctx, data)...)
}

func (r *BrandingConfigurationResource) read(ctx context.Context, data *BrandingConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	b := wireBinding(diags, brandingWire)
	if b == nil {
		return
	}

	current, err := r.client.GetBrandingConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read branding configuration", err.Error())
		return
	}

	diags.Append(b.FlattenInto(ctx, current.RawJSON, data)...)
	data.ID = types.StringValue("branding")
	diags.Append(state.Set(ctx, data)...)
}
