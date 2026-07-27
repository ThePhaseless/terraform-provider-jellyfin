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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

const jellyfinSecurityPluginID = "94879a0c-da24-4eb1-aa06-f28b4b9333b1"

var (
	_ resource.Resource                = &JellyfinSecurityPluginConfigurationResource{}
	_ resource.ResourceWithImportState = &JellyfinSecurityPluginConfigurationResource{}
)

// JellyfinSecurityPluginConfigurationResource defines the resource implementation.
type JellyfinSecurityPluginConfigurationResource struct {
	client *client.Client
}

// JellyfinSecurityPluginConfigurationResourceModel describes the resource data model.
type JellyfinSecurityPluginConfigurationResourceModel struct {
	ID                                 types.String `tfsdk:"id"`
	PluginID                           types.String `tfsdk:"plugin_id"`
	Enabled                            types.Bool   `tfsdk:"enabled"`
	BlockEmptyPasswordLogin            types.Bool   `tfsdk:"block_empty_password_login"`
	RequireChallengeIPMatch            types.Bool   `tfsdk:"require_challenge_ip_match"`
	RegisteredDeviceMaxAgeDays         types.Int64  `tfsdk:"registered_device_max_age_days"`
	BareDeviceIDBypassEnabled          types.Bool   `tfsdk:"bare_device_id_bypass_enabled"`
	RequireTwoFactorToDisable          types.Bool   `tfsdk:"require_two_factor_to_disable"`
	SelfServiceStepUpMode              types.String `tfsdk:"self_service_step_up_mode"`
	StepUpLevel                        types.String `tfsdk:"step_up_level"`
	StepUpWindowSeconds                types.Int64  `tfsdk:"step_up_window_seconds"`
	AllowIndefiniteTrust               types.Bool   `tfsdk:"allow_indefinite_trust"`
	HideBuiltinTwoFactorButton         types.Bool   `tfsdk:"hide_builtin_two_factor_button"`
	HideBuiltinPasskeyButton           types.Bool   `tfsdk:"hide_builtin_passkey_button"`
	EnforcementScope                   types.String `tfsdk:"enforcement_scope"`
	LanBypassEnabled                   types.Bool   `tfsdk:"lan_bypass_enabled"`
	LanBypassCidrs                     types.List   `tfsdk:"lan_bypass_cidrs"`
	TrustForwardedFor                  types.Bool   `tfsdk:"trust_forwarded_for"`
	TrustedProxyCidrs                  types.List   `tfsdk:"trusted_proxy_cidrs"`
	EmailOtpEnabled                    types.Bool   `tfsdk:"email_otp_enabled"`
	HibpEnabled                        types.Bool   `tfsdk:"hibp_enabled"`
	EmailOTPTTLSeconds                 types.Int64  `tfsdk:"email_otp_ttl_seconds"`
	ChallengeTokenTTLSeconds           types.Int64  `tfsdk:"challenge_token_ttl_seconds"`
	PairingCodeTTLSeconds              types.Int64  `tfsdk:"pairing_code_ttl_seconds"`
	MaxFailedAttempts                  types.Int64  `tfsdk:"max_failed_attempts"`
	LockoutDurationMinutes             types.Int64  `tfsdk:"lockout_duration_minutes"`
	ExemptAdministratorsFromLockout    types.Bool   `tfsdk:"exempt_administrators_from_lockout"`
	DisablePasswordLogin               types.Bool   `tfsdk:"disable_password_login"`
	AllowAdminPasswordLogin            types.Bool   `tfsdk:"allow_admin_password_login"`
	AllowPasswordLoginOnLan            types.Bool   `tfsdk:"allow_password_login_on_lan"`
	PasswordLoginExemptCidrs           types.List   `tfsdk:"password_login_exempt_cidrs"`
	EnablePasswordRecovery             types.Bool   `tfsdk:"enable_password_recovery"`
	HideBuiltinForgotPassword          types.Bool   `tfsdk:"hide_builtin_forgot_password"`
	LoginLinksBelowQuickConnect        types.Bool   `tfsdk:"login_links_below_quick_connect"`
	AuditLogMaxEntries                 types.Int64  `tfsdk:"audit_log_max_entries"`
	NtfyURL                            types.String `tfsdk:"ntfy_url"`
	NtfyTopic                          types.String `tfsdk:"ntfy_topic"`
	GotifyURL                          types.String `tfsdk:"gotify_url"`
	GotifyAppToken                     types.String `tfsdk:"gotify_app_token"`
	AllowPrivateNotificationTargets    types.Bool   `tfsdk:"allow_private_notification_targets"`
	NotifyEmailAddresses               types.List   `tfsdk:"notify_email_addresses"`
	SMTPHost                           types.String `tfsdk:"smtp_host"`
	SMTPPort                           types.Int64  `tfsdk:"smtp_port"`
	SMTPUseSsl                         types.Bool   `tfsdk:"smtp_use_ssl"`
	SMTPUsername                       types.String `tfsdk:"smtp_username"`
	SMTPPassword                       types.String `tfsdk:"smtp_password"`
	SMTPFromAddress                    types.String `tfsdk:"smtp_from_address"`
	SMTPFromName                       types.String `tfsdk:"smtp_from_name"`
	UserEmails                         types.List   `tfsdk:"user_emails"`
	TotpIssuerName                     types.String `tfsdk:"totp_issuer_name"`
	DefaultLanguage                    types.String `tfsdk:"default_language"`
	PreVerifyWindowSeconds             types.Int64  `tfsdk:"pre_verify_window_seconds"`
	TrustCookieTTLDays                 types.Int64  `tfsdk:"trust_cookie_ttl_days"`
	NatHairpinSelfIPBypass             types.Bool   `tfsdk:"nat_hairpin_self_ip_bypass"`
	DefaultMaxConcurrentSessions       types.Int64  `tfsdk:"default_max_concurrent_sessions"`
	EnrollmentDeadline                 types.String `tfsdk:"enrollment_deadline"`
	WebhookURL                         types.String `tfsdk:"webhook_url"`
	WebhookSecret                      types.String `tfsdk:"webhook_secret"`
	GeoIPAsnDbPath                     types.String `tfsdk:"geo_ip_asn_db_path"`
	GeoIPCountryDbPath                 types.String `tfsdk:"geo_ip_country_db_path"`
	WebauthnRpID                       types.String `tfsdk:"webauthn_rp_id"`
	WebauthnOrigins                    types.List   `tfsdk:"webauthn_origins"`
	BypassForExternalAuthProviders     types.Bool   `tfsdk:"bypass_for_external_auth_providers"`
	OidcProviders                      types.List   `tfsdk:"oidc_providers"`
	GeoIPCityDbPath                    types.String `tfsdk:"geo_ip_city_db_path"`
	IPBanEnabled                       types.Bool   `tfsdk:"ip_ban_enabled"`
	IPBanFailureThreshold              types.Int64  `tfsdk:"ip_ban_failure_threshold"`
	IPBanFailureWindowMinutes          types.Int64  `tfsdk:"ip_ban_failure_window_minutes"`
	IPBanDurationHours                 types.Int64  `tfsdk:"ip_ban_duration_hours"`
	IPBanExemptCidrs                   types.List   `tfsdk:"ip_ban_exempt_cidrs"`
	ImpossibleTravelEnabled            types.Bool   `tfsdk:"impossible_travel_enabled"`
	ImpossibleTravelMaxKmh             types.Int64  `tfsdk:"impossible_travel_max_kmh"`
	WebhookEd25519PrivateKey           types.String `tfsdk:"webhook_ed25519_private_key"`
	OnboardingPasswordMinLength        types.Int64  `tfsdk:"onboarding_password_min_length"`
	OnboardingPasswordRequireUppercase types.Bool   `tfsdk:"onboarding_password_require_uppercase"`
	OnboardingPasswordRequireLowercase types.Bool   `tfsdk:"onboarding_password_require_lowercase"`
	OnboardingPasswordRequireDigit     types.Bool   `tfsdk:"onboarding_password_require_digit"`
	OnboardingPasswordRequireSymbol    types.Bool   `tfsdk:"onboarding_password_require_symbol"`
}

// OidcProviderModel describes an OIDC provider configuration.
type OidcProviderModel struct {
	Id                       types.String `tfsdk:"id"`
	DisplayName              types.String `tfsdk:"display_name"`
	Preset                   types.String `tfsdk:"preset"`
	DiscoveryURL             types.String `tfsdk:"discovery_url"`
	ClientID                 types.String `tfsdk:"client_id"`
	ClientSecret             types.String `tfsdk:"client_secret"`
	Scopes                   types.List   `tfsdk:"scopes"`
	AcrValues                types.List   `tfsdk:"acr_values"`
	UsernameClaim            types.String `tfsdk:"username_claim"`
	AllowedGroups            types.List   `tfsdk:"allowed_groups"`
	AdminGroups              types.List   `tfsdk:"admin_groups"`
	AllowAdminGroupElevation types.Bool   `tfsdk:"allow_admin_group_elevation"`
	TemplateUserID           types.String `tfsdk:"template_user_id"`
	AutoCreateUsers          types.Bool   `tfsdk:"auto_create_users"`
	RequireIdpMfa            types.Bool   `tfsdk:"require_idp_mfa"`
	BypassPluginTwoFa        types.Bool   `tfsdk:"bypass_plugin_two_fa"`
	Enabled                  types.Bool   `tfsdk:"enabled"`
	ShowLoginButton          types.Bool   `tfsdk:"show_login_button"`
	ForceHTTPS               types.Bool   `tfsdk:"force_https"`
	AllowPrivateNetworks     types.Bool   `tfsdk:"allow_private_networks"`
	AdditionalAllowedCidrs   types.List   `tfsdk:"additional_allowed_cidrs"`
	SyncProfilePicture       types.Bool   `tfsdk:"sync_profile_picture"`
	PictureClaim             types.String `tfsdk:"picture_claim"`
	PromptSelectAccount      types.Bool   `tfsdk:"prompt_select_account"`
	OmitPromptLogin          types.Bool   `tfsdk:"omit_prompt_login"`
	ApplyRoleLibraryAccess   types.Bool   `tfsdk:"apply_role_library_access"`
	RoleLibraryMappings      types.List   `tfsdk:"role_library_mappings"`
	EmailClaim               types.String `tfsdk:"email_claim"`
	SyncEmailFromClaim       types.Bool   `tfsdk:"sync_email_from_claim"`
	ButtonText               types.String `tfsdk:"button_text"`
	ButtonIconURL            types.String `tfsdk:"button_icon_url"`
	ForcePasswordSetup       types.Bool   `tfsdk:"force_password_setup"`
	CreatedAt                types.String `tfsdk:"created_at"`
}

// RoleLibraryMappingModel describes a role-to-library mapping entry.
type RoleLibraryMappingModel struct {
	Role       types.String `tfsdk:"role"`
	LibraryIDs types.List   `tfsdk:"library_ids"`
}

// UserEmailEntryModel describes a user-email mapping entry.
type UserEmailEntryModel struct {
	UserID types.String `tfsdk:"user_id"`
	Email  types.String `tfsdk:"email"`
}

// NewJellyfinSecurityPluginConfigurationResource creates a new JellyfinSecurity plugin configuration resource.
func NewJellyfinSecurityPluginConfigurationResource() resource.Resource {
	return &JellyfinSecurityPluginConfigurationResource{}
}
func (r *JellyfinSecurityPluginConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_plugin_configuration"
}
func (r *JellyfinSecurityPluginConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
	sensitiveString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			Sensitive:           true,
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

	resp.Schema = schema.Schema{
		Description:         "Manages the JellyfinSecurity plugin configuration with typed attributes.",
		MarkdownDescription: "Manages the JellyfinSecurity plugin configuration with typed attributes.",
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
			"enabled":                            optionalBool("Whether the plugin is enabled."),
			"block_empty_password_login":         optionalBool("Block login for users with empty passwords."),
			"require_challenge_ip_match":         optionalBool("Require challenge token IP to match request IP."),
			"registered_device_max_age_days":     optionalInt("Max age in days for registered devices."),
			"bare_device_id_bypass_enabled":      optionalBool("Enable bypass for bare device IDs."),
			"require_two_factor_to_disable":      optionalBool("Require 2FA to disable the plugin."),
			"self_service_step_up_mode":          optionalString("Self-service step-up mode: Off, UserChoice, or Forced."),
			"step_up_level":                      optionalString("Step-up level: Off, Destructive, AllConfigChanges, or Everything."),
			"step_up_window_seconds":             optionalInt("Step-up authentication window in seconds."),
			"allow_indefinite_trust":             optionalBool("Allow indefinite trust for devices."),
			"hide_builtin_two_factor_button":     optionalBool("Hide the built-in 2FA login button."),
			"hide_builtin_passkey_button":        optionalBool("Hide the built-in passkey login button."),
			"enforcement_scope":                  optionalString("2FA enforcement scope: Optional, Admins, or All."),
			"lan_bypass_enabled":                 optionalBool("Enable LAN bypass for 2FA."),
			"lan_bypass_cidrs":                   optionalStringList("CIDR ranges that bypass 2FA on LAN."),
			"trust_forwarded_for":                optionalBool("Trust X-Forwarded-For header."),
			"trusted_proxy_cidrs":                optionalStringList("Trusted proxy CIDR ranges."),
			"email_otp_enabled":                  optionalBool("Enable email OTP."),
			"hibp_enabled":                       optionalBool("Enable Have I Been Pwned password check."),
			"email_otp_ttl_seconds":              optionalInt("Email OTP time-to-live in seconds."),
			"challenge_token_ttl_seconds":        optionalInt("Challenge token time-to-live in seconds."),
			"pairing_code_ttl_seconds":           optionalInt("Pairing code time-to-live in seconds."),
			"max_failed_attempts":                optionalInt("Max failed auth attempts before lockout."),
			"lockout_duration_minutes":           optionalInt("Lockout duration in minutes."),
			"exempt_administrators_from_lockout": optionalBool("Exempt administrators from lockout."),
			"disable_password_login":             optionalBool("Disable password login entirely."),
			"allow_admin_password_login":         optionalBool("Allow password login for administrators."),
			"allow_password_login_on_lan":        optionalBool("Allow password login on LAN."),
			"password_login_exempt_cidrs":        optionalStringList("CIDR ranges exempt from password login disable."),
			"enable_password_recovery":           optionalBool("Enable password recovery."),
			"hide_builtin_forgot_password":       optionalBool("Hide the built-in forgot password link."),
			"login_links_below_quick_connect":    optionalBool("Show login links below Quick Connect."),
			"audit_log_max_entries":              optionalInt("Max audit log entries."),
			"ntfy_url":                           optionalString("ntfy notification URL."),
			"ntfy_topic":                         optionalString("ntfy notification topic."),
			"gotify_url":                         optionalString("Gotify notification URL."),
			"gotify_app_token":                   sensitiveString("Gotify app token."),
			"allow_private_notification_targets": optionalBool("Allow private network notification targets."),
			"notify_email_addresses":             optionalStringList("Email addresses to notify."),
			"smtp_host":                          optionalString("SMTP server host."),
			"smtp_port":                          optionalInt("SMTP server port."),
			"smtp_use_ssl":                       optionalBool("Use SSL for SMTP."),
			"smtp_username":                      optionalString("SMTP username."),
			"smtp_password":                      sensitiveString("SMTP password."),
			"smtp_from_address":                  optionalString("SMTP from address."),
			"smtp_from_name":                     optionalString("SMTP from name."),
			"user_emails": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"user_id": optionalString("Jellyfin user ID."),
						"email":   optionalString("User email address."),
					},
				},
				Description:         "User email mappings.",
				MarkdownDescription: "User email mappings.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"totp_issuer_name":                   optionalString("TOTP issuer name."),
			"default_language":                   optionalString("Default language code."),
			"pre_verify_window_seconds":          optionalInt("Pre-verify window in seconds."),
			"trust_cookie_ttl_days":              optionalInt("Trust cookie TTL in days."),
			"nat_hairpin_self_ip_bypass":         optionalBool("Enable NAT hairpin self-IP bypass."),
			"default_max_concurrent_sessions":    optionalInt("Default max concurrent sessions per user (0 = unlimited)."),
			"enrollment_deadline":                optionalString("2FA enrollment deadline (ISO-8601 or null)."),
			"webhook_url":                        optionalString("Webhook notification URL."),
			"webhook_secret":                     sensitiveString("Webhook signing secret."),
			"geo_ip_asn_db_path":                 optionalString("Path to GeoIP ASN database."),
			"geo_ip_country_db_path":             optionalString("Path to GeoIP country database."),
			"webauthn_rp_id":                     optionalString("WebAuthn relying party ID."),
			"webauthn_origins":                   optionalStringList("WebAuthn allowed origins."),
			"bypass_for_external_auth_providers": optionalBool("Bypass 2FA for external auth providers."),
			"oidc_providers": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: oidcProviderAttributes(optionalBool, optionalString, sensitiveString, optionalStringList, optionalInt),
				},
				Description:         "List of OIDC provider configurations.",
				MarkdownDescription: "List of OIDC provider configurations.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"geo_ip_city_db_path":                   optionalString("Path to GeoIP city database."),
			"ip_ban_enabled":                        optionalBool("Enable IP banning."),
			"ip_ban_failure_threshold":              optionalInt("IP ban failure threshold."),
			"ip_ban_failure_window_minutes":         optionalInt("IP ban failure window in minutes."),
			"ip_ban_duration_hours":                 optionalInt("IP ban duration in hours."),
			"ip_ban_exempt_cidrs":                   optionalStringList("CIDR ranges exempt from IP banning."),
			"impossible_travel_enabled":             optionalBool("Enable impossible travel detection."),
			"impossible_travel_max_kmh":             optionalInt("Impossible travel max speed in km/h."),
			"webhook_ed25519_private_key":           sensitiveString("Webhook Ed25519 private key."),
			"onboarding_password_min_length":        optionalInt("Onboarding password minimum length."),
			"onboarding_password_require_uppercase": optionalBool("Require uppercase in onboarding passwords."),
			"onboarding_password_require_lowercase": optionalBool("Require lowercase in onboarding passwords."),
			"onboarding_password_require_digit":     optionalBool("Require digit in onboarding passwords."),
			"onboarding_password_require_symbol":    optionalBool("Require symbol in onboarding passwords."),
		},
	}
}

func oidcProviderAttributes(
	optionalBool func(string) schema.BoolAttribute,
	optionalString func(string) schema.StringAttribute,
	sensitiveString func(string) schema.StringAttribute,
	optionalStringList func(string) schema.ListAttribute,
	_ func(string) schema.Int64Attribute,
) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":                          optionalString("Provider ID (callback URL slug)."),
		"display_name":                optionalString("Display name for the provider."),
		"preset":                      optionalString("Provider preset (e.g. generic, authentik)."),
		"discovery_url":               optionalString("OIDC discovery URL."),
		"client_id":                   optionalString("OIDC client ID."),
		"client_secret":               sensitiveString("OIDC client secret."),
		"scopes":                      optionalStringList("OIDC scopes (space-delimited on wire)."),
		"acr_values":                  optionalStringList("OIDC ACR values (space-delimited on wire)."),
		"username_claim":              optionalString("JWT claim for username."),
		"allowed_groups":              optionalStringList("Groups allowed to log in (comma-delimited on wire)."),
		"admin_groups":                optionalStringList("Groups granted admin (comma-delimited on wire)."),
		"allow_admin_group_elevation": optionalBool("Allow admin group elevation."),
		"template_user_id":            optionalString("Template user ID for auto-created users."),
		"auto_create_users":           optionalBool("Auto-create users on first login."),
		"require_idp_mfa":             optionalBool("Require IdP MFA."),
		"bypass_plugin_two_fa":        optionalBool("Bypass plugin 2FA for this provider."),
		"enabled":                     optionalBool("Whether this provider is enabled."),
		"show_login_button":           optionalBool("Show login button for this provider."),
		"force_https":                 optionalBool("Force HTTPS for redirect URI."),
		"allow_private_networks":      optionalBool("Allow private network redirect URIs."),
		"additional_allowed_cidrs":    optionalStringList("Additional allowed CIDRs (comma-delimited on wire)."),
		"sync_profile_picture":        optionalBool("Sync profile picture from IdP."),
		"picture_claim":               optionalString("JWT claim for profile picture."),
		"prompt_select_account":       optionalBool("Prompt for account selection."),
		"omit_prompt_login":           optionalBool("Omit prompt=login from auth request."),
		"apply_role_library_access":   optionalBool("Apply role-based library access."),
		"role_library_mappings": schema.ListNestedAttribute{
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"role":        optionalString("Role name."),
					"library_ids": optionalStringList("Library IDs (comma-delimited on wire)."),
				},
			},
			Description:         "Role-to-library access mappings.",
			MarkdownDescription: "Role-to-library access mappings.",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		},
		"email_claim":           optionalString("JWT claim for email."),
		"sync_email_from_claim": optionalBool("Sync email from claim."),
		"button_text":           optionalString("Login button text."),
		"button_icon_url":       optionalString("Login button icon URL."),
		"force_password_setup":  optionalBool("Force password setup on first login."),
		"created_at": schema.StringAttribute{
			Description:         "Creation timestamp (server-managed).",
			MarkdownDescription: "Creation timestamp (server-managed).",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
	}
}

func (r *JellyfinSecurityPluginConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *JellyfinSecurityPluginConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data JellyfinSecurityPluginConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.requireJellyfinSecurityPluginInstalled(ctx, &resp.Diagnostics); err != nil {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
	if resp.Diagnostics.HasError() {
		return
	}

	r.checkJellyfinSecurityVersionWarning(ctx, &resp.Diagnostics)
}

func (r *JellyfinSecurityPluginConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data JellyfinSecurityPluginConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &data, &resp.Diagnostics, &resp.State)
	if resp.Diagnostics.HasError() {
		return
	}

	r.checkJellyfinSecurityVersionWarning(ctx, &resp.Diagnostics)
}

func (r *JellyfinSecurityPluginConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data JellyfinSecurityPluginConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.requireJellyfinSecurityPluginInstalled(ctx, &resp.Diagnostics); err != nil {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
	if resp.Diagnostics.HasError() {
		return
	}

	r.checkJellyfinSecurityVersionWarning(ctx, &resp.Diagnostics)
}

func (r *JellyfinSecurityPluginConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Plugin configuration cannot truly be deleted.
}

func (r *JellyfinSecurityPluginConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("plugin_id"), req, resp)
}

func (r *JellyfinSecurityPluginConfigurationResource) requireJellyfinSecurityPluginInstalled(ctx context.Context, diags *diag.Diagnostics) error {
	installed, err := r.client.GetInstalledPlugins(ctx)
	if err != nil {
		diags.AddError("Failed to check installed plugins", err.Error())
		return err
	}

	canonical := normalizeGUID(jellyfinSecurityPluginID)
	for _, p := range installed {
		if normalizeGUID(p.ID) == canonical {
			return nil
		}
	}

	diags.AddError(
		"JellyfinSecurity plugin not installed",
		fmt.Sprintf("JellyfinSecurity plugin %s is not installed on the server. Register the plugin repository and install the plugin before managing its configuration, for example with the jellyfin_plugin_repository and jellyfin_plugin resources.", jellyfinSecurityPluginID),
	)
	return fmt.Errorf("JellyfinSecurity plugin not installed")
}

func (r *JellyfinSecurityPluginConfigurationResource) checkJellyfinSecurityVersionWarning(ctx context.Context, diags *diag.Diagnostics) {
	installed, err := r.client.GetInstalledPlugins(ctx)
	if err != nil {
		return
	}
	canonical := normalizeGUID(jellyfinSecurityPluginID)
	for _, p := range installed {
		if normalizeGUID(p.ID) == canonical {
			if detail, ok := versionNewerWarning("JellyfinSecurity plugin", p.Version, supportedSecurityPluginVersion()); ok {
				diags.AddWarning("JellyfinSecurity plugin version newer than supported", detail)
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

func (r *JellyfinSecurityPluginConfigurationResource) apply(ctx context.Context, data *JellyfinSecurityPluginConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetPluginConfiguration(ctx, data.PluginID.ValueString())
	if err != nil {
		diags.AddError("Failed to read JellyfinSecurity plugin configuration", err.Error())
		return
	}

	base, err := parseJSONObject(current)
	if err != nil {
		diags.AddError("Failed to parse JellyfinSecurity plugin configuration", err.Error())
		return
	}

	d := overlayJellyfinSecurity(ctx, base, data)
	if d.HasError() {
		diags.Append(d...)
		return
	}

	payload, err := json.Marshal(base)
	if err != nil {
		diags.AddError("Failed to serialize JellyfinSecurity plugin configuration", err.Error())
		return
	}

	if err := r.client.UpdatePluginConfiguration(ctx, data.PluginID.ValueString(), string(payload)); err != nil {
		diags.AddError("Failed to update JellyfinSecurity plugin configuration", err.Error())
		return
	}

	updated, err := r.client.GetPluginConfiguration(ctx, data.PluginID.ValueString())
	if err != nil {
		diags.AddError("Failed to read JellyfinSecurity plugin configuration after update", err.Error())
		return
	}

	flattenJellyfinSecurity(ctx, updated, data, diags)
	data.ID = data.PluginID

	diags.Append(state.Set(ctx, data)...)
}

func (r *JellyfinSecurityPluginConfigurationResource) read(ctx context.Context, data *JellyfinSecurityPluginConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetPluginConfiguration(ctx, data.PluginID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			state.RemoveResource(ctx)
			return
		}
		diags.AddError("Failed to read JellyfinSecurity plugin configuration", err.Error())
		return
	}

	flattenJellyfinSecurity(ctx, current, data, diags)
	data.ID = data.PluginID

	diags.Append(state.Set(ctx, data)...)
}

// overlayJellyfinSecurity overlays the typed model onto the server config map.
func overlayJellyfinSecurity(ctx context.Context, m map[string]json.RawMessage, data *JellyfinSecurityPluginConfigurationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	// Root scalar fields
	putJSONBool(m, "Enabled", data.Enabled)
	putJSONBool(m, "BlockEmptyPasswordLogin", data.BlockEmptyPasswordLogin)
	putJSONBool(m, "RequireChallengeIpMatch", data.RequireChallengeIPMatch)
	putJSONInt64(m, "RegisteredDeviceMaxAgeDays", data.RegisteredDeviceMaxAgeDays)
	putJSONBool(m, "BareDeviceIdBypassEnabled", data.BareDeviceIDBypassEnabled)
	putJSONBool(m, "RequireTwoFactorToDisable", data.RequireTwoFactorToDisable)
	putJSONString(m, "SelfServiceStepUpMode", data.SelfServiceStepUpMode)
	putJSONString(m, "StepUpLevel", data.StepUpLevel)
	putJSONInt64(m, "StepUpWindowSeconds", data.StepUpWindowSeconds)
	putJSONBool(m, "AllowIndefiniteTrust", data.AllowIndefiniteTrust)
	putJSONBool(m, "HideBuiltInTwoFactorButton", data.HideBuiltinTwoFactorButton)
	putJSONBool(m, "HideBuiltInPasskeyButton", data.HideBuiltinPasskeyButton)
	putJSONString(m, "EnforcementScope", data.EnforcementScope)
	putJSONBool(m, "LanBypassEnabled", data.LanBypassEnabled)
	putJSONBool(m, "TrustForwardedFor", data.TrustForwardedFor)
	putJSONBool(m, "EmailOtpEnabled", data.EmailOtpEnabled)
	putJSONBool(m, "HibpEnabled", data.HibpEnabled)
	putJSONInt64(m, "EmailOtpTtlSeconds", data.EmailOTPTTLSeconds)
	putJSONInt64(m, "ChallengeTokenTtlSeconds", data.ChallengeTokenTTLSeconds)
	putJSONInt64(m, "PairingCodeTtlSeconds", data.PairingCodeTTLSeconds)
	putJSONInt64(m, "MaxFailedAttempts", data.MaxFailedAttempts)
	putJSONInt64(m, "LockoutDurationMinutes", data.LockoutDurationMinutes)
	putJSONBool(m, "ExemptAdministratorsFromLockout", data.ExemptAdministratorsFromLockout)
	putJSONBool(m, "DisablePasswordLogin", data.DisablePasswordLogin)
	putJSONBool(m, "AllowAdminPasswordLogin", data.AllowAdminPasswordLogin)
	putJSONBool(m, "AllowPasswordLoginOnLan", data.AllowPasswordLoginOnLan)
	putJSONBool(m, "EnablePasswordRecovery", data.EnablePasswordRecovery)
	putJSONBool(m, "HideBuiltInForgotPassword", data.HideBuiltinForgotPassword)
	putJSONBool(m, "LoginLinksBelowQuickConnect", data.LoginLinksBelowQuickConnect)
	putJSONInt64(m, "AuditLogMaxEntries", data.AuditLogMaxEntries)
	putJSONString(m, "NtfyUrl", data.NtfyURL)
	putJSONString(m, "NtfyTopic", data.NtfyTopic)
	putJSONString(m, "GotifyUrl", data.GotifyURL)
	putJSONString(m, "GotifyAppToken", data.GotifyAppToken)
	putJSONBool(m, "AllowPrivateNotificationTargets", data.AllowPrivateNotificationTargets)
	putJSONString(m, "SmtpHost", data.SMTPHost)
	putJSONInt64(m, "SmtpPort", data.SMTPPort)
	putJSONBool(m, "SmtpUseSsl", data.SMTPUseSsl)
	putJSONString(m, "SmtpUsername", data.SMTPUsername)
	putJSONString(m, "SmtpPassword", data.SMTPPassword)
	putJSONString(m, "SmtpFromAddress", data.SMTPFromAddress)
	putJSONString(m, "SmtpFromName", data.SMTPFromName)
	putJSONString(m, "TotpIssuerName", data.TotpIssuerName)
	putJSONString(m, "DefaultLanguage", data.DefaultLanguage)
	putJSONInt64(m, "PreVerifyWindowSeconds", data.PreVerifyWindowSeconds)
	putJSONInt64(m, "TrustCookieTtlDays", data.TrustCookieTTLDays)
	putJSONBool(m, "NatHairpinSelfIpBypass", data.NatHairpinSelfIPBypass)
	putJSONInt64(m, "DefaultMaxConcurrentSessions", data.DefaultMaxConcurrentSessions)
	putJSONString(m, "EnrollmentDeadline", data.EnrollmentDeadline)
	putJSONString(m, "WebhookUrl", data.WebhookURL)
	putJSONString(m, "WebhookSecret", data.WebhookSecret)
	putJSONString(m, "GeoIpAsnDbPath", data.GeoIPAsnDbPath)
	putJSONString(m, "GeoIpCountryDbPath", data.GeoIPCountryDbPath)
	putJSONString(m, "WebAuthnRpId", data.WebauthnRpID)
	putJSONBool(m, "BypassForExternalAuthProviders", data.BypassForExternalAuthProviders)
	putJSONString(m, "GeoIpCityDbPath", data.GeoIPCityDbPath)
	putJSONBool(m, "IpBanEnabled", data.IPBanEnabled)
	putJSONInt64(m, "IpBanFailureThreshold", data.IPBanFailureThreshold)
	putJSONInt64(m, "IpBanFailureWindowMinutes", data.IPBanFailureWindowMinutes)
	putJSONInt64(m, "IpBanDurationHours", data.IPBanDurationHours)
	putJSONBool(m, "ImpossibleTravelEnabled", data.ImpossibleTravelEnabled)
	putJSONInt64(m, "ImpossibleTravelMaxKmh", data.ImpossibleTravelMaxKmh)
	putJSONString(m, "WebhookEd25519PrivateKey", data.WebhookEd25519PrivateKey)
	putJSONInt64(m, "OnboardingPasswordMinLength", data.OnboardingPasswordMinLength)
	putJSONBool(m, "OnboardingPasswordRequireUppercase", data.OnboardingPasswordRequireUppercase)
	putJSONBool(m, "OnboardingPasswordRequireLowercase", data.OnboardingPasswordRequireLowercase)
	putJSONBool(m, "OnboardingPasswordRequireDigit", data.OnboardingPasswordRequireDigit)
	putJSONBool(m, "OnboardingPasswordRequireSymbol", data.OnboardingPasswordRequireSymbol)

	// Root string[] fields (JSON array wire format)
	if d := putJSONStringList(ctx, m, "LanBypassCidrs", data.LanBypassCidrs); d.HasError() {
		return append(diags, d...)
	}
	if d := putJSONStringList(ctx, m, "TrustedProxyCidrs", data.TrustedProxyCidrs); d.HasError() {
		return append(diags, d...)
	}
	if d := putJSONStringList(ctx, m, "PasswordLoginExemptCidrs", data.PasswordLoginExemptCidrs); d.HasError() {
		return append(diags, d...)
	}
	if d := putJSONStringList(ctx, m, "NotifyEmailAddresses", data.NotifyEmailAddresses); d.HasError() {
		return append(diags, d...)
	}
	if d := putJSONStringList(ctx, m, "WebAuthnOrigins", data.WebauthnOrigins); d.HasError() {
		return append(diags, d...)
	}
	if d := putJSONStringList(ctx, m, "IpBanExemptCidrs", data.IPBanExemptCidrs); d.HasError() {
		return append(diags, d...)
	}

	// UserEmails — List<UserEmailEntry>
	if !data.UserEmails.IsNull() && !data.UserEmails.IsUnknown() {
		var entries []UserEmailEntryModel
		if d := data.UserEmails.ElementsAs(ctx, &entries, false); d.HasError() {
			return append(diags, d...)
		}
		objs := make([]map[string]json.RawMessage, len(entries))
		for i, e := range entries {
			entry := map[string]json.RawMessage{}
			putJSONString(entry, "UserId", e.UserID)
			putJSONString(entry, "Email", e.Email)
			objs[i] = entry
		}
		b, err := json.Marshal(objs)
		if err != nil {
			return append(diags, diag.NewErrorDiagnostic("Failed to marshal user emails", err.Error()))
		}
		m["UserEmails"] = b
	}

	// OidcProviders — preserve existing CreatedAt per Id
	if !data.OidcProviders.IsNull() && !data.OidcProviders.IsUnknown() {
		var providers []OidcProviderModel
		if d := data.OidcProviders.ElementsAs(ctx, &providers, false); d.HasError() {
			return append(diags, d...)
		}

		// Build map of existing CreatedAt by Id
		existingCreatedAt := map[string]string{}
		if raw, ok := m["OidcProviders"]; ok && !isJSONNull(raw) {
			var existing []map[string]json.RawMessage
			if err := json.Unmarshal(raw, &existing); err == nil {
				for _, e := range existing {
					id := getJSONString(e, "Id")
					createdAt := getJSONString(e, "CreatedAt")
					if !id.IsNull() && !createdAt.IsNull() {
						existingCreatedAt[id.ValueString()] = createdAt.ValueString()
					}
				}
			}
		}

		objs := make([]map[string]json.RawMessage, len(providers))
		for i, p := range providers {
			entry := map[string]json.RawMessage{}
			overlayOidcProvider(ctx, entry, &p)
			// Preserve CreatedAt from existing entry if present
			if id := p.Id; !id.IsNull() {
				if ca, ok := existingCreatedAt[id.ValueString()]; ok {
					b, _ := json.Marshal(ca)
					entry["CreatedAt"] = b
				}
			}
			objs[i] = entry
		}
		b, err := json.Marshal(objs)
		if err != nil {
			return append(diags, diag.NewErrorDiagnostic("Failed to marshal OIDC providers", err.Error()))
		}
		m["OidcProviders"] = b
	}

	return diags
}

// overlayOidcProvider overlays a single OIDC provider onto the entry map.
func overlayOidcProvider(ctx context.Context, m map[string]json.RawMessage, p *OidcProviderModel) {
	putJSONString(m, "Id", p.Id)
	putJSONString(m, "DisplayName", p.DisplayName)
	putJSONString(m, "Preset", p.Preset)
	putJSONString(m, "DiscoveryUrl", p.DiscoveryURL)
	putJSONString(m, "ClientId", p.ClientID)
	putJSONString(m, "ClientSecret", p.ClientSecret)
	putDelimitedString(ctx, m, "Scopes", p.Scopes, " ")
	putDelimitedString(ctx, m, "AcrValues", p.AcrValues, " ")
	putJSONString(m, "UsernameClaim", p.UsernameClaim)
	putDelimitedString(ctx, m, "AllowedGroups", p.AllowedGroups, ",")
	putDelimitedString(ctx, m, "AdminGroups", p.AdminGroups, ",")
	putJSONBool(m, "AllowAdminGroupElevation", p.AllowAdminGroupElevation)
	putJSONString(m, "TemplateUserId", p.TemplateUserID)
	putJSONBool(m, "AutoCreateUsers", p.AutoCreateUsers)
	putJSONBool(m, "RequireIdpMfa", p.RequireIdpMfa)
	putJSONBool(m, "BypassPluginTwoFa", p.BypassPluginTwoFa)
	putJSONBool(m, "Enabled", p.Enabled)
	putJSONBool(m, "ShowLoginButton", p.ShowLoginButton)
	putJSONBool(m, "ForceHttps", p.ForceHTTPS)
	putJSONBool(m, "AllowPrivateNetworks", p.AllowPrivateNetworks)
	putDelimitedString(ctx, m, "AdditionalAllowedCidrs", p.AdditionalAllowedCidrs, ",")
	putJSONBool(m, "SyncProfilePicture", p.SyncProfilePicture)
	putJSONString(m, "PictureClaim", p.PictureClaim)
	putJSONBool(m, "PromptSelectAccount", p.PromptSelectAccount)
	putJSONBool(m, "OmitPromptLogin", p.OmitPromptLogin)
	putJSONBool(m, "ApplyRoleLibraryAccess", p.ApplyRoleLibraryAccess)
	putJSONString(m, "EmailClaim", p.EmailClaim)
	putJSONBool(m, "SyncEmailFromClaim", p.SyncEmailFromClaim)
	putJSONString(m, "ButtonText", p.ButtonText)
	putJSONString(m, "ButtonIconUrl", p.ButtonIconURL)
	putJSONBool(m, "ForcePasswordSetup", p.ForcePasswordSetup)

	// RoleLibraryMappings — nested list
	if !p.RoleLibraryMappings.IsNull() && !p.RoleLibraryMappings.IsUnknown() {
		var mappings []RoleLibraryMappingModel
		if d := p.RoleLibraryMappings.ElementsAs(ctx, &mappings, false); d.HasError() {
			return
		}
		objs := make([]map[string]json.RawMessage, len(mappings))
		for i, rlm := range mappings {
			entry := map[string]json.RawMessage{}
			putJSONString(entry, "Role", rlm.Role)
			putDelimitedString(ctx, entry, "LibraryIds", rlm.LibraryIDs, ",")
			objs[i] = entry
		}
		b, _ := json.Marshal(objs)
		m["RoleLibraryMappings"] = b
	}
}

// putDelimitedString joins a types.List of strings with the given delimiter and writes it as a JSON string.
func putDelimitedString(ctx context.Context, m map[string]json.RawMessage, key string, v types.List, delim string) {
	if v.IsNull() || v.IsUnknown() {
		return
	}
	var elements []types.String
	if d := v.ElementsAs(ctx, &elements, false); d.HasError() {
		return
	}
	values := make([]string, len(elements))
	for i, elem := range elements {
		values[i] = elem.ValueString()
	}
	joined := strings.Join(values, delim)
	b, _ := json.Marshal(joined)
	m[key] = b
}

// getDelimitedStringList reads a delimited string from the JSON map and splits it into a types.List.
func getDelimitedStringList(ctx context.Context, m map[string]json.RawMessage, key, delim string) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	s := getJSONString(m, key)
	if s.IsNull() {
		return types.ListNull(types.StringType), diags
	}
	parts := strings.Split(s.ValueString(), delim)
	elements := make([]types.String, 0, len(parts))
	for _, p := range parts {
		// Trim empty strings from split when the source was empty
		if p == "" && len(parts) == 1 {
			break
		}
		elements = append(elements, types.StringValue(p))
	}
	list, d := types.ListValueFrom(ctx, types.StringType, elements)
	return list, append(diags, d...)
}

// flattenJellyfinSecurity reads the server response into the typed model.
func flattenJellyfinSecurity(ctx context.Context, raw string, data *JellyfinSecurityPluginConfigurationResourceModel, diags *diag.Diagnostics) {
	m, err := parseJSONObject(raw)
	if err != nil {
		diags.AddError("Failed to parse JellyfinSecurity plugin configuration", err.Error())
		return
	}

	// Root scalar fields
	data.Enabled = getJSONBool(m, "Enabled")
	data.BlockEmptyPasswordLogin = getJSONBool(m, "BlockEmptyPasswordLogin")
	data.RequireChallengeIPMatch = getJSONBool(m, "RequireChallengeIpMatch")
	data.RegisteredDeviceMaxAgeDays = getJSONInt64(m, "RegisteredDeviceMaxAgeDays")
	data.BareDeviceIDBypassEnabled = getJSONBool(m, "BareDeviceIdBypassEnabled")
	data.RequireTwoFactorToDisable = getJSONBool(m, "RequireTwoFactorToDisable")
	data.SelfServiceStepUpMode = getJSONString(m, "SelfServiceStepUpMode")
	data.StepUpLevel = getJSONString(m, "StepUpLevel")
	data.StepUpWindowSeconds = getJSONInt64(m, "StepUpWindowSeconds")
	data.AllowIndefiniteTrust = getJSONBoolDefaultFalse(m, "AllowIndefiniteTrust")
	data.HideBuiltinTwoFactorButton = getJSONBool(m, "HideBuiltInTwoFactorButton")
	data.HideBuiltinPasskeyButton = getJSONBool(m, "HideBuiltInPasskeyButton")
	data.EnforcementScope = getJSONString(m, "EnforcementScope")
	data.LanBypassEnabled = getJSONBool(m, "LanBypassEnabled")
	data.TrustForwardedFor = getJSONBool(m, "TrustForwardedFor")
	data.EmailOtpEnabled = getJSONBool(m, "EmailOtpEnabled")
	data.HibpEnabled = getJSONBool(m, "HibpEnabled")
	data.EmailOTPTTLSeconds = getJSONInt64(m, "EmailOtpTtlSeconds")
	data.ChallengeTokenTTLSeconds = getJSONInt64(m, "ChallengeTokenTtlSeconds")
	data.PairingCodeTTLSeconds = getJSONInt64(m, "PairingCodeTtlSeconds")
	data.MaxFailedAttempts = getJSONInt64(m, "MaxFailedAttempts")
	data.LockoutDurationMinutes = getJSONInt64(m, "LockoutDurationMinutes")
	data.ExemptAdministratorsFromLockout = getJSONBool(m, "ExemptAdministratorsFromLockout")
	data.DisablePasswordLogin = getJSONBool(m, "DisablePasswordLogin")
	data.AllowAdminPasswordLogin = getJSONBool(m, "AllowAdminPasswordLogin")
	data.AllowPasswordLoginOnLan = getJSONBool(m, "AllowPasswordLoginOnLan")
	data.EnablePasswordRecovery = getJSONBool(m, "EnablePasswordRecovery")
	data.HideBuiltinForgotPassword = getJSONBool(m, "HideBuiltInForgotPassword")
	data.LoginLinksBelowQuickConnect = getJSONBool(m, "LoginLinksBelowQuickConnect")
	data.AuditLogMaxEntries = getJSONInt64(m, "AuditLogMaxEntries")
	data.NtfyURL = getJSONString(m, "NtfyUrl")
	data.NtfyTopic = getJSONString(m, "NtfyTopic")
	data.GotifyURL = getJSONString(m, "GotifyUrl")
	data.GotifyAppToken = getJSONString(m, "GotifyAppToken")
	data.AllowPrivateNotificationTargets = getJSONBool(m, "AllowPrivateNotificationTargets")
	data.SMTPHost = getJSONString(m, "SmtpHost")
	data.SMTPPort = getJSONInt64(m, "SmtpPort")
	data.SMTPUseSsl = getJSONBool(m, "SmtpUseSsl")
	data.SMTPUsername = getJSONString(m, "SmtpUsername")
	data.SMTPPassword = getJSONString(m, "SmtpPassword")
	data.SMTPFromAddress = getJSONString(m, "SmtpFromAddress")
	data.SMTPFromName = getJSONString(m, "SmtpFromName")
	data.TotpIssuerName = getJSONString(m, "TotpIssuerName")
	data.DefaultLanguage = getJSONString(m, "DefaultLanguage")
	data.PreVerifyWindowSeconds = getJSONInt64(m, "PreVerifyWindowSeconds")
	data.TrustCookieTTLDays = getJSONInt64(m, "TrustCookieTtlDays")
	data.NatHairpinSelfIPBypass = getJSONBool(m, "NatHairpinSelfIpBypass")
	data.DefaultMaxConcurrentSessions = getJSONInt64(m, "DefaultMaxConcurrentSessions")
	data.EnrollmentDeadline = getJSONString(m, "EnrollmentDeadline")
	data.WebhookURL = getJSONString(m, "WebhookUrl")
	data.WebhookSecret = getJSONString(m, "WebhookSecret")
	data.GeoIPAsnDbPath = getJSONString(m, "GeoIpAsnDbPath")
	data.GeoIPCountryDbPath = getJSONString(m, "GeoIpCountryDbPath")
	data.WebauthnRpID = getJSONString(m, "WebAuthnRpId")
	data.BypassForExternalAuthProviders = getJSONBool(m, "BypassForExternalAuthProviders")
	data.GeoIPCityDbPath = getJSONString(m, "GeoIpCityDbPath")
	data.IPBanEnabled = getJSONBool(m, "IpBanEnabled")
	data.IPBanFailureThreshold = getJSONInt64(m, "IpBanFailureThreshold")
	data.IPBanFailureWindowMinutes = getJSONInt64(m, "IpBanFailureWindowMinutes")
	data.IPBanDurationHours = getJSONInt64(m, "IpBanDurationHours")
	data.ImpossibleTravelEnabled = getJSONBool(m, "ImpossibleTravelEnabled")
	data.ImpossibleTravelMaxKmh = getJSONInt64(m, "ImpossibleTravelMaxKmh")
	data.WebhookEd25519PrivateKey = getJSONString(m, "WebhookEd25519PrivateKey")
	data.OnboardingPasswordMinLength = getJSONInt64(m, "OnboardingPasswordMinLength")
	data.OnboardingPasswordRequireUppercase = getJSONBoolDefaultFalse(m, "OnboardingPasswordRequireUppercase")
	data.OnboardingPasswordRequireLowercase = getJSONBoolDefaultFalse(m, "OnboardingPasswordRequireLowercase")
	data.OnboardingPasswordRequireDigit = getJSONBoolDefaultFalse(m, "OnboardingPasswordRequireDigit")
	data.OnboardingPasswordRequireSymbol = getJSONBoolDefaultFalse(m, "OnboardingPasswordRequireSymbol")

	// Root string[] fields (JSON array wire format)
	var d diag.Diagnostics
	data.LanBypassCidrs, d = getJSONStringList(ctx, m, "LanBypassCidrs")
	diags.Append(d...)
	data.TrustedProxyCidrs, d = getJSONStringList(ctx, m, "TrustedProxyCidrs")
	diags.Append(d...)
	data.PasswordLoginExemptCidrs, d = getJSONStringList(ctx, m, "PasswordLoginExemptCidrs")
	diags.Append(d...)
	data.NotifyEmailAddresses, d = getJSONStringList(ctx, m, "NotifyEmailAddresses")
	diags.Append(d...)
	data.WebauthnOrigins, d = getJSONStringList(ctx, m, "WebAuthnOrigins")
	diags.Append(d...)
	data.IPBanExemptCidrs, d = getJSONStringList(ctx, m, "IpBanExemptCidrs")
	diags.Append(d...)

	// UserEmails
	data.UserEmails = flattenUserEmails(ctx, m, diags)

	// OidcProviders
	data.OidcProviders = flattenOidcProviders(ctx, m, diags)
}

// flattenUserEmails reads the UserEmails array from the server response.
func flattenUserEmails(_ context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["UserEmails"]
	if !ok || isJSONNull(raw) {
		return types.ListNull(types.ObjectType{AttrTypes: userEmailEntryObjectTypes()})
	}

	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse user emails", err.Error())
		return types.ListNull(types.ObjectType{AttrTypes: userEmailEntryObjectTypes()})
	}

	objType := types.ObjectType{AttrTypes: userEmailEntryObjectTypes()}
	objects := make([]attr.Value, len(entries))
	for i, entry := range entries {
		attrs := map[string]attr.Value{
			"user_id": getJSONString(entry, "UserId"),
			"email":   getJSONString(entry, "Email"),
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

// flattenOidcProviders reads the OidcProviders array from the server response.
func flattenOidcProviders(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["OidcProviders"]
	if !ok || isJSONNull(raw) {
		return types.ListNull(types.ObjectType{AttrTypes: oidcProviderObjectTypes()})
	}

	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse OIDC providers", err.Error())
		return types.ListNull(types.ObjectType{AttrTypes: oidcProviderObjectTypes()})
	}

	objType := types.ObjectType{AttrTypes: oidcProviderObjectTypes()}
	objects := make([]attr.Value, len(entries))
	for i, entry := range entries {
		attrs := oidcProviderAttrs(ctx, entry, diags)
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

// flattenRoleLibraryMappings reads the RoleLibraryMappings array from an OIDC provider entry.
func flattenRoleLibraryMappings(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["RoleLibraryMappings"]
	if !ok || isJSONNull(raw) {
		return types.ListNull(types.ObjectType{AttrTypes: roleLibraryMappingObjectTypes()})
	}

	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse role library mappings", err.Error())
		return types.ListNull(types.ObjectType{AttrTypes: roleLibraryMappingObjectTypes()})
	}

	objType := types.ObjectType{AttrTypes: roleLibraryMappingObjectTypes()}
	objects := make([]attr.Value, len(entries))
	for i, entry := range entries {
		libraryIDs, d := getDelimitedStringList(ctx, entry, "LibraryIds", ",")
		diags.Append(d...)
		attrs := map[string]attr.Value{
			"role":        getJSONString(entry, "Role"),
			"library_ids": libraryIDs,
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

func userEmailEntryObjectTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"user_id": types.StringType,
		"email":   types.StringType,
	}
}

func roleLibraryMappingObjectTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"role":        types.StringType,
		"library_ids": types.ListType{ElemType: types.StringType},
	}
}

func oidcProviderObjectTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id":                          types.StringType,
		"display_name":                types.StringType,
		"preset":                      types.StringType,
		"discovery_url":               types.StringType,
		"client_id":                   types.StringType,
		"client_secret":               types.StringType,
		"scopes":                      types.ListType{ElemType: types.StringType},
		"acr_values":                  types.ListType{ElemType: types.StringType},
		"username_claim":              types.StringType,
		"allowed_groups":              types.ListType{ElemType: types.StringType},
		"admin_groups":                types.ListType{ElemType: types.StringType},
		"allow_admin_group_elevation": types.BoolType,
		"template_user_id":            types.StringType,
		"auto_create_users":           types.BoolType,
		"require_idp_mfa":             types.BoolType,
		"bypass_plugin_two_fa":        types.BoolType,
		"enabled":                     types.BoolType,
		"show_login_button":           types.BoolType,
		"force_https":                 types.BoolType,
		"allow_private_networks":      types.BoolType,
		"additional_allowed_cidrs":    types.ListType{ElemType: types.StringType},
		"sync_profile_picture":        types.BoolType,
		"picture_claim":               types.StringType,
		"prompt_select_account":       types.BoolType,
		"omit_prompt_login":           types.BoolType,
		"apply_role_library_access":   types.BoolType,
		"role_library_mappings":       types.ListType{ElemType: types.ObjectType{AttrTypes: roleLibraryMappingObjectTypes()}},
		"email_claim":                 types.StringType,
		"sync_email_from_claim":       types.BoolType,
		"button_text":                 types.StringType,
		"button_icon_url":             types.StringType,
		"force_password_setup":        types.BoolType,
		"created_at":                  types.StringType,
	}
}

func oidcProviderAttrs(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) map[string]attr.Value {
	attrs := map[string]attr.Value{}
	attrs["id"] = getJSONString(m, "Id")
	attrs["display_name"] = getJSONString(m, "DisplayName")
	attrs["preset"] = getJSONString(m, "Preset")
	attrs["discovery_url"] = getJSONString(m, "DiscoveryUrl")
	attrs["client_id"] = getJSONString(m, "ClientId")
	attrs["client_secret"] = getJSONString(m, "ClientSecret")

	scopes, d := getDelimitedStringList(ctx, m, "Scopes", " ")
	diags.Append(d...)
	attrs["scopes"] = scopes

	acrValues, d := getDelimitedStringList(ctx, m, "AcrValues", " ")
	diags.Append(d...)
	attrs["acr_values"] = acrValues

	attrs["username_claim"] = getJSONString(m, "UsernameClaim")

	allowedGroups, d := getDelimitedStringList(ctx, m, "AllowedGroups", ",")
	diags.Append(d...)
	attrs["allowed_groups"] = allowedGroups

	adminGroups, d := getDelimitedStringList(ctx, m, "AdminGroups", ",")
	diags.Append(d...)
	attrs["admin_groups"] = adminGroups

	attrs["allow_admin_group_elevation"] = getJSONBool(m, "AllowAdminGroupElevation")
	attrs["template_user_id"] = getJSONString(m, "TemplateUserId")
	attrs["auto_create_users"] = getJSONBool(m, "AutoCreateUsers")
	attrs["require_idp_mfa"] = getJSONBool(m, "RequireIdpMfa")
	attrs["bypass_plugin_two_fa"] = getJSONBool(m, "BypassPluginTwoFa")
	attrs["enabled"] = getJSONBool(m, "Enabled")
	attrs["show_login_button"] = getJSONBool(m, "ShowLoginButton")
	attrs["force_https"] = getJSONBool(m, "ForceHttps")
	attrs["allow_private_networks"] = getJSONBool(m, "AllowPrivateNetworks")

	additionalCidrs, d := getDelimitedStringList(ctx, m, "AdditionalAllowedCidrs", ",")
	diags.Append(d...)
	attrs["additional_allowed_cidrs"] = additionalCidrs

	attrs["sync_profile_picture"] = getJSONBool(m, "SyncProfilePicture")
	attrs["picture_claim"] = getJSONString(m, "PictureClaim")
	attrs["prompt_select_account"] = getJSONBool(m, "PromptSelectAccount")
	attrs["omit_prompt_login"] = getJSONBool(m, "OmitPromptLogin")
	attrs["apply_role_library_access"] = getJSONBool(m, "ApplyRoleLibraryAccess")
	attrs["role_library_mappings"] = flattenRoleLibraryMappings(ctx, m, diags)
	attrs["email_claim"] = getJSONString(m, "EmailClaim")
	attrs["sync_email_from_claim"] = getJSONBool(m, "SyncEmailFromClaim")
	attrs["button_text"] = getJSONString(m, "ButtonText")
	attrs["button_icon_url"] = getJSONString(m, "ButtonIconUrl")
	attrs["force_password_setup"] = getJSONBool(m, "ForcePasswordSetup")
	attrs["created_at"] = getJSONString(m, "CreatedAt")
	return attrs
}
