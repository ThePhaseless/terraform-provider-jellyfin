// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

const ssoPluginID = "505ce9d1-d916-42fa-86ca-673ef241d7df"

var (
	_ resource.Resource                = &SSOPluginConfigurationResource{}
	_ resource.ResourceWithImportState = &SSOPluginConfigurationResource{}
)

// NewSSOPluginConfigurationResource creates a new SSO plugin configuration resource.
func NewSSOPluginConfigurationResource() resource.Resource {
	return &SSOPluginConfigurationResource{}
}

// SSOPluginConfigurationResource defines the resource implementation.
type SSOPluginConfigurationResource struct {
	client *client.Client
}

// SSOPluginConfigurationResourceModel describes the resource data model.
type SSOPluginConfigurationResourceModel struct {
	ID           types.String `tfsdk:"id"`
	PluginID     types.String `tfsdk:"plugin_id"`
	OidConfigs   types.Map    `tfsdk:"oid_configs"`
	SamlConfigs  types.Map    `tfsdk:"saml_configs"`
}

// SSOOidConfigModel describes an OID provider configuration.
type SSOOidConfigModel struct {
	OidEndpoint                   types.String `tfsdk:"oid_endpoint"`
	OidClientID                   types.String `tfsdk:"oid_client_id"`
	OidSecret                     types.String `tfsdk:"oid_secret"`
	Enabled                       types.Bool   `tfsdk:"enabled"`
	EnableAuthorization           types.Bool   `tfsdk:"enable_authorization"`
	EnableAllFolders              types.Bool   `tfsdk:"enable_all_folders"`
	EnabledFolders                types.List   `tfsdk:"enabled_folders"`
	AdminRoles                    types.List   `tfsdk:"admin_roles"`
	Roles                         types.List   `tfsdk:"roles"`
	EnableFolderRoles             types.Bool   `tfsdk:"enable_folder_roles"`
	EnableLiveTvRoles             types.Bool   `tfsdk:"enable_live_tv_roles"`
	EnableLiveTv                  types.Bool   `tfsdk:"enable_live_tv"`
	EnableLiveTvManagement        types.Bool   `tfsdk:"enable_live_tv_management"`
	LiveTvRoles                   types.List   `tfsdk:"live_tv_roles"`
	LiveTvManagementRoles         types.List   `tfsdk:"live_tv_management_roles"`
	FolderRoleMapping             types.List   `tfsdk:"folder_role_mapping"`
	RoleClaim                     types.String `tfsdk:"role_claim"`
	OidScopes                     types.List   `tfsdk:"oid_scopes"`
	DefaultProvider               types.String `tfsdk:"default_provider"`
	SchemeOverride                types.String `tfsdk:"scheme_override"`
	PortOverride                  types.Int64  `tfsdk:"port_override"`
	NewPath                       types.Bool   `tfsdk:"new_path"`
	CanonicalLinks                types.Map    `tfsdk:"canonical_links"`
	DefaultUsernameClaim          types.String `tfsdk:"default_username_claim"`
	AvatarUrlFormat               types.String `tfsdk:"avatar_url_format"`
	DisableHttps                  types.Bool   `tfsdk:"disable_https"`
	DisablePushedAuthorization    types.Bool   `tfsdk:"disable_pushed_authorization"`
	DoNotValidateEndpoints        types.Bool   `tfsdk:"do_not_validate_endpoints"`
	DoNotValidateIssuerName       types.Bool   `tfsdk:"do_not_validate_issuer_name"`
	DoNotLoadProfile              types.Bool   `tfsdk:"do_not_load_profile"`
}

// SSOSamlConfigModel describes a SAML provider configuration.
type SSOSamlConfigModel struct {
	SamlEndpoint                  types.String `tfsdk:"saml_endpoint"`
	SamlClientID                  types.String `tfsdk:"saml_client_id"`
	SamlCertificate               types.String `tfsdk:"saml_certificate"`
	Enabled                       types.Bool   `tfsdk:"enabled"`
	EnableAuthorization           types.Bool   `tfsdk:"enable_authorization"`
	EnableAllFolders              types.Bool   `tfsdk:"enable_all_folders"`
	EnabledFolders                types.List   `tfsdk:"enabled_folders"`
	AdminRoles                    types.List   `tfsdk:"admin_roles"`
	Roles                         types.List   `tfsdk:"roles"`
	EnableFolderRoles             types.Bool   `tfsdk:"enable_folder_roles"`
	EnableLiveTvRoles             types.Bool   `tfsdk:"enable_live_tv_roles"`
	EnableLiveTv                  types.Bool   `tfsdk:"enable_live_tv"`
	EnableLiveTvManagement        types.Bool   `tfsdk:"enable_live_tv_management"`
	LiveTvRoles                   types.List   `tfsdk:"live_tv_roles"`
	LiveTvManagementRoles         types.List   `tfsdk:"live_tv_management_roles"`
	FolderRoleMapping             types.List   `tfsdk:"folder_role_mapping"`
	RoleClaim                     types.String `tfsdk:"role_claim"`
	DefaultProvider               types.String `tfsdk:"default_provider"`
	SchemeOverride                types.String `tfsdk:"scheme_override"`
	PortOverride                  types.Int64  `tfsdk:"port_override"`
	NewPath                       types.Bool   `tfsdk:"new_path"`
	CanonicalLinks                types.Map    `tfsdk:"canonical_links"`
	DefaultUsernameClaim          types.String `tfsdk:"default_username_claim"`
	AvatarUrlFormat               types.String `tfsdk:"avatar_url_format"`
	DisableHttps                  types.Bool   `tfsdk:"disable_https"`
	DisablePushedAuthorization    types.Bool   `tfsdk:"disable_pushed_authorization"`
	DoNotValidateEndpoints        types.Bool   `tfsdk:"do_not_validate_endpoints"`
	DoNotValidateIssuerName       types.Bool   `tfsdk:"do_not_validate_issuer_name"`
	DoNotLoadProfile              types.Bool   `tfsdk:"do_not_load_profile"`
}

// SSOFolderRoleMappingModel describes a folder role mapping entry.
type SSOFolderRoleMappingModel struct {
	Role    types.String `tfsdk:"role"`
	Folders types.List   `tfsdk:"folders"`
}

func ssoCommonConfigAttributes() map[string]schema.Attribute {
	optionalBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.UseStateForUnknown(),
			},
		}
	}
	optionalString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		}
	}
	optionalInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		}
	}
	optionalStringList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{
			ElementType:         types.StringType,
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		}
	}

	return map[string]schema.Attribute{
		"enabled":                optionalBool("Whether this provider is enabled."),
		"enable_authorization":   optionalBool("Whether authorization is enabled."),
		"enable_all_folders":     optionalBool("Whether all folders are enabled."),
		"enabled_folders":        optionalStringList("Folders enabled for this provider."),
		"admin_roles":            optionalStringList("Roles that grant administrator access."),
		"roles":                  optionalStringList("Roles that grant standard access."),
		"enable_folder_roles":    optionalBool("Whether folder roles are enabled."),
		"enable_live_tv_roles":   optionalBool("Whether live TV roles are enabled."),
		"enable_live_tv":         optionalBool("Whether live TV is enabled."),
		"enable_live_tv_management": optionalBool("Whether live TV management is enabled."),
		"live_tv_roles":          optionalStringList("Roles that grant live TV access."),
		"live_tv_management_roles": optionalStringList("Roles that grant live TV management access."),
		"folder_role_mapping": schema.ListNestedAttribute{
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"role": schema.StringAttribute{
						Description:         "Role name.",
						MarkdownDescription: "Role name.",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"folders": optionalStringList("Folders mapped to this role."),
				},
			},
			Description:         "Folder role mappings.",
			MarkdownDescription: "Folder role mappings.",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		},
		"role_claim":               optionalString("Claim used for roles."),
		"default_provider":         optionalString("Default authentication provider."),
		"scheme_override":          optionalString("Scheme override."),
		"port_override":            optionalInt("Port override."),
		"new_path":                 optionalBool("Whether to use a new path."),
		"canonical_links": schema.MapAttribute{
			ElementType:         types.StringType,
			Description:         "Server-managed canonical links. Omit to preserve server-side mappings.",
			MarkdownDescription: "Server-managed canonical links. Omit to preserve server-side mappings.",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.Map{
				mapplanmodifier.UseStateForUnknown(),
			},
		},
		"default_username_claim":   optionalString("Claim used for the default username."),
		"avatar_url_format":        optionalString("Avatar URL format."),
		"disable_https":            optionalBool("Whether HTTPS is disabled."),
		"disable_pushed_authorization": optionalBool("Whether pushed authorization is disabled."),
		"do_not_validate_endpoints":    optionalBool("Whether endpoint validation is skipped."),
		"do_not_validate_issuer_name":  optionalBool("Whether issuer name validation is skipped."),
		"do_not_load_profile":          optionalBool("Whether profile loading is skipped."),
	}
}

func oidConfigAttributes() map[string]schema.Attribute {
	attrs := ssoCommonConfigAttributes()
	optionalString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		}
	}
	optionalStringList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{
			ElementType:         types.StringType,
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		}
	}
	attrs["oid_endpoint"] = optionalString("OIDC endpoint URL.")
	attrs["oid_client_id"] = optionalString("OIDC client ID.")
	attrs["oid_secret"] = schema.StringAttribute{
		Description:         "OIDC client secret.",
		MarkdownDescription: "OIDC client secret.",
		Optional:            true,
		Computed:            true,
		Sensitive:           true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
	attrs["oid_scopes"] = optionalStringList("OIDC scopes.")
	return attrs
}

func samlConfigAttributes() map[string]schema.Attribute {
	attrs := ssoCommonConfigAttributes()
	optionalString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		}
	}
	attrs["saml_endpoint"] = optionalString("SAML endpoint URL.")
	attrs["saml_client_id"] = optionalString("SAML client ID.")
	attrs["saml_certificate"] = schema.StringAttribute{
		Description:         "SAML certificate (public key).",
		MarkdownDescription: "SAML certificate (public key).",
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
	return attrs
}

func (r *SSOPluginConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sso_plugin_configuration"
}

func (r *SSOPluginConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages the SSO-Auth plugin configuration with typed attributes.",
		MarkdownDescription: "Manages the SSO-Auth plugin configuration with typed attributes.",
		Attributes: map[string]schema.Attribute{
			"plugin_id": schema.StringAttribute{
				Description:         "The plugin ID (GUID).",
				MarkdownDescription: "The plugin ID (GUID).",
				Required:            true,
				Validators:          requiredIdentifierValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": schema.StringAttribute{
				Description:         "The plugin configuration resource identifier.",
				MarkdownDescription: "The plugin configuration resource identifier.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"oid_configs": schema.MapNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: oidConfigAttributes(),
				},
				Description:         "Map of OID provider configurations keyed by provider name.",
				MarkdownDescription: "Map of OID provider configurations keyed by provider name.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"saml_configs": schema.MapNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: samlConfigAttributes(),
				},
				Description:         "Map of SAML provider configurations keyed by provider name.",
				MarkdownDescription: "Map of SAML provider configurations keyed by provider name.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *SSOPluginConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SSOPluginConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SSOPluginConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.requireSSOPluginInstalled(ctx, &resp.Diagnostics); err != nil {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
	if resp.Diagnostics.HasError() {
		return
	}

	r.checkSSOVersionWarning(ctx, &resp.Diagnostics)
}

func (r *SSOPluginConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SSOPluginConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &data, &resp.Diagnostics, &resp.State)
	if resp.Diagnostics.HasError() {
		return
	}

	r.checkSSOVersionWarning(ctx, &resp.Diagnostics)
}

func (r *SSOPluginConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SSOPluginConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.requireSSOPluginInstalled(ctx, &resp.Diagnostics); err != nil {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
	if resp.Diagnostics.HasError() {
		return
	}

	r.checkSSOVersionWarning(ctx, &resp.Diagnostics)
}

func (r *SSOPluginConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Plugin configuration cannot truly be deleted.
}

func (r *SSOPluginConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("plugin_id"), req, resp)
}

func (r *SSOPluginConfigurationResource) requireSSOPluginInstalled(ctx context.Context, diags *diag.Diagnostics) error {
	installed, err := r.client.GetInstalledPlugins(ctx)
	if err != nil {
		diags.AddError("Failed to check installed plugins", err.Error())
		return err
	}

	canonical := normalizeGUID(ssoPluginID)
	for _, p := range installed {
		if normalizeGUID(p.ID) == canonical {
			return nil
		}
	}

	diags.AddError(
		"SSO-Auth plugin not installed",
		fmt.Sprintf("SSO-Auth plugin %s is not installed on the server. Register the plugin repository and install the plugin before managing its configuration, for example with the jellyfin_plugin_repository and jellyfin_plugin resources.", ssoPluginID),
	)
	return fmt.Errorf("SSO-Auth plugin not installed")
}

func (r *SSOPluginConfigurationResource) checkSSOVersionWarning(ctx context.Context, diags *diag.Diagnostics) {
	installed, err := r.client.GetInstalledPlugins(ctx)
	if err != nil {
		return
	}
	canonical := normalizeGUID(ssoPluginID)
	for _, p := range installed {
		if normalizeGUID(p.ID) == canonical {
			if detail, ok := versionNewerWarning("SSO-Auth plugin", p.Version, supportedSSOPluginVersion()); ok {
				diags.AddWarning("SSO-Auth plugin version newer than supported", detail)
			}
			return
		}
	}
}

// normalizeGUID returns a lowercase, dash-free GUID so that the provider can
// compare IDs regardless of whether Jellyfin returns them as "D" or "N" format.
func normalizeGUID(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "-", ""))
}

func (r *SSOPluginConfigurationResource) apply(ctx context.Context, data *SSOPluginConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetPluginConfiguration(ctx, data.PluginID.ValueString())
	if err != nil {
		diags.AddError("Failed to read SSO plugin configuration", err.Error())
		return
	}

	base, err := parseJSONObject(current)
	if err != nil {
		diags.AddError("Failed to parse SSO plugin configuration", err.Error())
		return
	}

	d := overlaySSOPluginConfiguration(ctx, base, data)
	if d.HasError() {
		diags.Append(d...)
		return
	}

	payload, err := json.Marshal(base)
	if err != nil {
		diags.AddError("Failed to serialize SSO plugin configuration", err.Error())
		return
	}

	if err := r.client.UpdatePluginConfiguration(ctx, data.PluginID.ValueString(), string(payload)); err != nil {
		diags.AddError("Failed to update SSO plugin configuration", err.Error())
		return
	}

	updated, err := r.client.GetPluginConfiguration(ctx, data.PluginID.ValueString())
	if err != nil {
		diags.AddError("Failed to read SSO plugin configuration after update", err.Error())
		return
	}

	flattenSSOPluginConfiguration(ctx, updated, data, diags)
	data.ID = data.PluginID
}

func (r *SSOPluginConfigurationResource) read(ctx context.Context, data *SSOPluginConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetPluginConfiguration(ctx, data.PluginID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			state.RemoveResource(ctx)
			return
		}
		diags.AddError("Failed to read SSO plugin configuration", err.Error())
		return
	}

	flattenSSOPluginConfiguration(ctx, current, data, diags)
	data.ID = data.PluginID
}

func overlaySSOPluginConfiguration(ctx context.Context, m map[string]json.RawMessage, data *SSOPluginConfigurationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	if !data.OidConfigs.IsNull() && !data.OidConfigs.IsUnknown() {
		var configs map[string]SSOOidConfigModel
		if d := data.OidConfigs.ElementsAs(ctx, &configs, false); d.HasError() {
			return append(diags, d...)
		}
		obj := map[string]json.RawMessage{}
		for name, cfg := range configs {
			entry := map[string]json.RawMessage{}
			overlayOidConfig(ctx, entry, &cfg)
			b, err := json.Marshal(entry)
			if err != nil {
				return append(diags, diag.NewErrorDiagnostic("Failed to marshal OID config", err.Error()))
			}
			obj[name] = b
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return append(diags, diag.NewErrorDiagnostic("Failed to marshal OID configs", err.Error()))
		}
		m["OidConfigs"] = b
	}

	if !data.SamlConfigs.IsNull() && !data.SamlConfigs.IsUnknown() {
		var configs map[string]SSOSamlConfigModel
		if d := data.SamlConfigs.ElementsAs(ctx, &configs, false); d.HasError() {
			return append(diags, d...)
		}
		obj := map[string]json.RawMessage{}
		for name, cfg := range configs {
			entry := map[string]json.RawMessage{}
			overlaySamlConfig(ctx, entry, &cfg)
			b, err := json.Marshal(entry)
			if err != nil {
				return append(diags, diag.NewErrorDiagnostic("Failed to marshal SAML config", err.Error()))
			}
			obj[name] = b
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return append(diags, diag.NewErrorDiagnostic("Failed to marshal SAML configs", err.Error()))
		}
		m["SamlConfigs"] = b
	}

	return diags
}

func overlayOidConfig(ctx context.Context, m map[string]json.RawMessage, cfg *SSOOidConfigModel) {
	overlayCommonConfig(ctx, m, &cfg.Enabled, &cfg.EnableAuthorization, &cfg.EnableAllFolders, cfg.EnabledFolders, cfg.AdminRoles, cfg.Roles,
		&cfg.EnableFolderRoles, &cfg.EnableLiveTvRoles, &cfg.EnableLiveTv, &cfg.EnableLiveTvManagement, cfg.LiveTvRoles, cfg.LiveTvManagementRoles,
		cfg.FolderRoleMapping, cfg.RoleClaim, cfg.DefaultProvider, cfg.SchemeOverride, cfg.PortOverride, &cfg.NewPath, cfg.CanonicalLinks,
		cfg.DefaultUsernameClaim, cfg.AvatarUrlFormat, &cfg.DisableHttps, &cfg.DisablePushedAuthorization, &cfg.DoNotValidateEndpoints,
		&cfg.DoNotValidateIssuerName, &cfg.DoNotLoadProfile)
	putJSONString(m, "OidEndpoint", cfg.OidEndpoint)
	putJSONString(m, "OidClientId", cfg.OidClientID)
	putJSONString(m, "OidSecret", cfg.OidSecret)
	if d := putJSONStringList(ctx, m, "OidScopes", cfg.OidScopes); d.HasError() {
		// errors are not propagated because overlayOidConfig signature doesn't return diagnostics
	}
}

func overlaySamlConfig(ctx context.Context, m map[string]json.RawMessage, cfg *SSOSamlConfigModel) {
	overlayCommonConfig(ctx, m, &cfg.Enabled, &cfg.EnableAuthorization, &cfg.EnableAllFolders, cfg.EnabledFolders, cfg.AdminRoles, cfg.Roles,
		&cfg.EnableFolderRoles, &cfg.EnableLiveTvRoles, &cfg.EnableLiveTv, &cfg.EnableLiveTvManagement, cfg.LiveTvRoles, cfg.LiveTvManagementRoles,
		cfg.FolderRoleMapping, cfg.RoleClaim, cfg.DefaultProvider, cfg.SchemeOverride, cfg.PortOverride, &cfg.NewPath, cfg.CanonicalLinks,
		cfg.DefaultUsernameClaim, cfg.AvatarUrlFormat, &cfg.DisableHttps, &cfg.DisablePushedAuthorization, &cfg.DoNotValidateEndpoints,
		&cfg.DoNotValidateIssuerName, &cfg.DoNotLoadProfile)
	putJSONString(m, "SamlEndpoint", cfg.SamlEndpoint)
	putJSONString(m, "SamlClientId", cfg.SamlClientID)
	putJSONString(m, "SamlCertificate", cfg.SamlCertificate)
}

func overlayCommonConfig(ctx context.Context, m map[string]json.RawMessage,
	enabled, enableAuthorization, enableAllFolders *types.Bool,
	enabledFolders, adminRoles, roles types.List,
	enableFolderRoles, enableLiveTvRoles, enableLiveTv, enableLiveTvManagement *types.Bool,
	liveTvRoles, liveTvManagementRoles types.List,
	folderRoleMapping types.List,
	roleClaim, defaultProvider, schemeOverride types.String,
	portOverride types.Int64,
	newPath *types.Bool,
	canonicalLinks types.Map,
	defaultUsernameClaim, avatarUrlFormat types.String,
	disableHttps, disablePushedAuthorization, doNotValidateEndpoints, doNotValidateIssuerName, doNotLoadProfile *types.Bool,
) {
	putJSONBool(m, "Enabled", *enabled)
	putJSONBool(m, "EnableAuthorization", *enableAuthorization)
	putJSONBool(m, "EnableAllFolders", *enableAllFolders)
	putJSONStringList(ctx, m, "EnabledFolders", enabledFolders)
	putJSONStringList(ctx, m, "AdminRoles", adminRoles)
	putJSONStringList(ctx, m, "Roles", roles)
	putJSONBool(m, "EnableFolderRoles", *enableFolderRoles)
	putJSONBool(m, "EnableLiveTvRoles", *enableLiveTvRoles)
	putJSONBool(m, "EnableLiveTv", *enableLiveTv)
	putJSONBool(m, "EnableLiveTvManagement", *enableLiveTvManagement)
	putJSONStringList(ctx, m, "LiveTvRoles", liveTvRoles)
	putJSONStringList(ctx, m, "LiveTvManagementRoles", liveTvManagementRoles)
	if d := putFolderRoleMappings(ctx, m, folderRoleMapping); d.HasError() {
		// ignored
	}
	putJSONString(m, "RoleClaim", roleClaim)
	putJSONString(m, "DefaultProvider", defaultProvider)
	putJSONString(m, "SchemeOverride", schemeOverride)
	putJSONInt64(m, "PortOverride", portOverride)
	putJSONBool(m, "NewPath", *newPath)
	putJSONStringMap(ctx, m, "CanonicalLinks", canonicalLinks)
	putJSONString(m, "DefaultUsernameClaim", defaultUsernameClaim)
	putJSONString(m, "AvatarUrlFormat", avatarUrlFormat)
	putJSONBool(m, "DisableHttps", *disableHttps)
	putJSONBool(m, "DisablePushedAuthorization", *disablePushedAuthorization)
	putJSONBool(m, "DoNotValidateEndpoints", *doNotValidateEndpoints)
	putJSONBool(m, "DoNotValidateIssuerName", *doNotValidateIssuerName)
	putJSONBool(m, "DoNotLoadProfile", *doNotLoadProfile)
}

func putFolderRoleMappings(ctx context.Context, m map[string]json.RawMessage, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}
	var mappings []SSOFolderRoleMappingModel
	if d := v.ElementsAs(ctx, &mappings, false); d.HasError() {
		return append(diags, d...)
	}
	entries := make([]map[string]json.RawMessage, len(mappings))
	for i, mapping := range mappings {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "Role", mapping.Role)
		putJSONStringList(ctx, entry, "Folders", mapping.Folders)
		entries[i] = entry
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal folder role mappings", err.Error()))
	}
	m["FolderRoleMapping"] = b
	return diags
}

func flattenSSOPluginConfiguration(ctx context.Context, raw string, data *SSOPluginConfigurationResourceModel, diags *diag.Diagnostics) {
	m, err := parseJSONObject(raw)
	if err != nil {
		diags.AddError("Failed to parse SSO plugin configuration", err.Error())
		return
	}

	data.OidConfigs = flattenOidConfigs(ctx, m, diags)
	data.SamlConfigs = flattenSamlConfigs(ctx, m, diags)
}

func flattenOidConfigs(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.Map {
	raw, ok := m["OidConfigs"]
	if !ok {
		return types.MapNull(types.ObjectType{AttrTypes: oidConfigObjectTypes()})
	}
	if isJSONNull(raw) {
		return types.MapNull(types.ObjectType{AttrTypes: oidConfigObjectTypes()})
	}

	var configs map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &configs); err != nil {
		diags.AddError("Failed to parse OID configs", err.Error())
		return types.MapNull(types.ObjectType{AttrTypes: oidConfigObjectTypes()})
	}

	objects := make(map[string]attr.Value, len(configs))
	objType := types.ObjectType{AttrTypes: oidConfigObjectTypes()}
	for name, cfg := range configs {
		attrs := oidConfigAttrs(ctx, cfg, diags)
		obj, d := types.ObjectValue(oidConfigObjectTypes(), attrs)
		if d.HasError() {
			diags.Append(d...)
			return types.MapNull(objType)
		}
		objects[name] = obj
	}

	result, d := types.MapValue(objType, objects)
	if d.HasError() {
		diags.Append(d...)
		return types.MapNull(objType)
	}
	return result
}

func flattenSamlConfigs(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.Map {
	raw, ok := m["SamlConfigs"]
	if !ok {
		return types.MapNull(types.ObjectType{AttrTypes: samlConfigObjectTypes()})
	}
	if isJSONNull(raw) {
		return types.MapNull(types.ObjectType{AttrTypes: samlConfigObjectTypes()})
	}

	var configs map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &configs); err != nil {
		diags.AddError("Failed to parse SAML configs", err.Error())
		return types.MapNull(types.ObjectType{AttrTypes: samlConfigObjectTypes()})
	}

	objects := make(map[string]attr.Value, len(configs))
	objType := types.ObjectType{AttrTypes: samlConfigObjectTypes()}
	for name, cfg := range configs {
		attrs := samlConfigAttrs(ctx, cfg, diags)
		obj, d := types.ObjectValue(samlConfigObjectTypes(), attrs)
		if d.HasError() {
			diags.Append(d...)
			return types.MapNull(objType)
		}
		objects[name] = obj
	}

	result, d := types.MapValue(objType, objects)
	if d.HasError() {
		diags.Append(d...)
		return types.MapNull(objType)
	}
	return result
}

func oidConfigObjectTypes() map[string]attr.Type {
	attrs := commonConfigObjectTypes()
	attrs["oid_endpoint"] = types.StringType
	attrs["oid_client_id"] = types.StringType
	attrs["oid_secret"] = types.StringType
	attrs["oid_scopes"] = types.ListType{ElemType: types.StringType}
	return attrs
}

func samlConfigObjectTypes() map[string]attr.Type {
	attrs := commonConfigObjectTypes()
	attrs["saml_endpoint"] = types.StringType
	attrs["saml_client_id"] = types.StringType
	attrs["saml_certificate"] = types.StringType
	return attrs
}

func commonConfigObjectTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"enabled":                         types.BoolType,
		"enable_authorization":            types.BoolType,
		"enable_all_folders":              types.BoolType,
		"enabled_folders":                 types.ListType{ElemType: types.StringType},
		"admin_roles":                     types.ListType{ElemType: types.StringType},
		"roles":                           types.ListType{ElemType: types.StringType},
		"enable_folder_roles":             types.BoolType,
		"enable_live_tv_roles":            types.BoolType,
		"enable_live_tv":                  types.BoolType,
		"enable_live_tv_management":       types.BoolType,
		"live_tv_roles":                   types.ListType{ElemType: types.StringType},
		"live_tv_management_roles":        types.ListType{ElemType: types.StringType},
		"folder_role_mapping":             types.ListType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"role": types.StringType, "folders": types.ListType{ElemType: types.StringType}}}},
		"role_claim":                      types.StringType,
		"default_provider":                types.StringType,
		"scheme_override":                 types.StringType,
		"port_override":                   types.Int64Type,
		"new_path":                        types.BoolType,
		"canonical_links":                 types.MapType{ElemType: types.StringType},
		"default_username_claim":          types.StringType,
		"avatar_url_format":               types.StringType,
		"disable_https":                   types.BoolType,
		"disable_pushed_authorization":    types.BoolType,
		"do_not_validate_endpoints":       types.BoolType,
		"do_not_validate_issuer_name":     types.BoolType,
		"do_not_load_profile":             types.BoolType,
	}
}

func oidConfigAttrs(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) map[string]attr.Value {
	attrs := commonConfigAttrs(ctx, m, diags)
	attrs["oid_endpoint"] = getJSONString(m, "OidEndpoint")
	attrs["oid_client_id"] = getJSONString(m, "OidClientId")
	attrs["oid_secret"] = getJSONString(m, "OidSecret")
	attrs["oid_scopes"], _ = getJSONStringList(ctx, m, "OidScopes")
	return attrs
}

func samlConfigAttrs(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) map[string]attr.Value {
	attrs := commonConfigAttrs(ctx, m, diags)
	attrs["saml_endpoint"] = getJSONString(m, "SamlEndpoint")
	attrs["saml_client_id"] = getJSONString(m, "SamlClientId")
	attrs["saml_certificate"] = getJSONString(m, "SamlCertificate")
	return attrs
}

func commonConfigAttrs(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) map[string]attr.Value {
	attrs := map[string]attr.Value{}
	attrs["enabled"] = getJSONBool(m, "Enabled")
	attrs["enable_authorization"] = getJSONBool(m, "EnableAuthorization")
	attrs["enable_all_folders"] = getJSONBool(m, "EnableAllFolders")
	attrs["enabled_folders"], _ = getJSONStringList(ctx, m, "EnabledFolders")
	attrs["admin_roles"], _ = getJSONStringList(ctx, m, "AdminRoles")
	attrs["roles"], _ = getJSONStringList(ctx, m, "Roles")
	attrs["enable_folder_roles"] = getJSONBool(m, "EnableFolderRoles")
	attrs["enable_live_tv_roles"] = getJSONBool(m, "EnableLiveTvRoles")
	attrs["enable_live_tv"] = getJSONBool(m, "EnableLiveTv")
	attrs["enable_live_tv_management"] = getJSONBool(m, "EnableLiveTvManagement")
	attrs["live_tv_roles"], _ = getJSONStringList(ctx, m, "LiveTvRoles")
	attrs["live_tv_management_roles"], _ = getJSONStringList(ctx, m, "LiveTvManagementRoles")
	attrs["folder_role_mapping"] = flattenFolderRoleMappings(ctx, m, diags)
	attrs["role_claim"] = getJSONString(m, "RoleClaim")
	attrs["default_provider"] = getJSONString(m, "DefaultProvider")
	attrs["scheme_override"] = getJSONString(m, "SchemeOverride")
	attrs["port_override"] = getJSONInt64(m, "PortOverride")
	attrs["new_path"] = getJSONBool(m, "NewPath")
	attrs["canonical_links"], _ = getJSONStringMap(ctx, m, "CanonicalLinks")
	attrs["default_username_claim"] = getJSONString(m, "DefaultUsernameClaim")
	attrs["avatar_url_format"] = getJSONString(m, "AvatarUrlFormat")
	attrs["disable_https"] = getJSONBool(m, "DisableHttps")
	attrs["disable_pushed_authorization"] = getJSONBool(m, "DisablePushedAuthorization")
	attrs["do_not_validate_endpoints"] = getJSONBool(m, "DoNotValidateEndpoints")
	attrs["do_not_validate_issuer_name"] = getJSONBool(m, "DoNotValidateIssuerName")
	attrs["do_not_load_profile"] = getJSONBool(m, "DoNotLoadProfile")
	return attrs
}

func flattenFolderRoleMappings(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["FolderRoleMapping"]
	if !ok {
		return types.ListNull(types.ObjectType{AttrTypes: map[string]attr.Type{"role": types.StringType, "folders": types.ListType{ElemType: types.StringType}}})
	}
	if isJSONNull(raw) {
		return types.ListNull(types.ObjectType{AttrTypes: map[string]attr.Type{"role": types.StringType, "folders": types.ListType{ElemType: types.StringType}}})
	}

	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse folder role mappings", err.Error())
		return types.ListNull(types.ObjectType{AttrTypes: map[string]attr.Type{"role": types.StringType, "folders": types.ListType{ElemType: types.StringType}}})
	}

	objType := types.ObjectType{AttrTypes: map[string]attr.Type{"role": types.StringType, "folders": types.ListType{ElemType: types.StringType}}}
	objects := make([]attr.Value, len(entries))
	for i, entry := range entries {
		folders, _ := getJSONStringList(ctx, entry, "Folders")
		attrs := map[string]attr.Value{
			"role":    getJSONString(entry, "Role"),
			"folders": folders,
		}
		obj, d := types.ObjectValue(objType.AttrTypes, attrs)
		if d.HasError() {
			diags.Append(d...)
			return types.ListNull(objType)
		}
		objects[i] = obj
	}

	list, d := types.ListValue(objType, objects)
	if d.HasError() {
		diags.Append(d...)
		return types.ListNull(objType)
	}
	return list
}
