// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

const jellyfinSecurityPluginID = "94879a0c-da24-4eb1-aa06-f28b4b9333b1"

// isoDateTimePattern also takes an empty string, which the plugin reads as no
// date-time and then leaves out of its configuration.
var isoDateTimePattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?)?$`)

var (
	_ resource.Resource                = &JellyfinSecurityPluginConfigurationResource{}
	_ resource.ResourceWithImportState = &JellyfinSecurityPluginConfigurationResource{}
	_ wireBound                        = &JellyfinSecurityPluginConfigurationResource{}
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
	PairDeviceOnSecondScreenApproval   types.Bool   `tfsdk:"pair_device_on_second_screen_approval"`
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
	NtfyToken                          types.String `tfsdk:"ntfy_token"`
	NtfyUsername                       types.String `tfsdk:"ntfy_username"`
	NtfyPassword                       types.String `tfsdk:"ntfy_password"`
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
	WebhookHeaders                     types.List   `tfsdk:"webhook_headers"`
	GeoIPAsnDbPath                     types.String `tfsdk:"geo_ip_asn_db_path"`
	GeoIPCountryDbPath                 types.String `tfsdk:"geo_ip_country_db_path"`
	WebauthnRpID                       types.String `tfsdk:"webauthn_rp_id"`
	WebauthnOrigins                    types.List   `tfsdk:"webauthn_origins"`
	PublicBaseURL                      types.String `tfsdk:"public_base_url"`
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

var securityPluginWire = sync.OnceValues(func() (*wire.Binding, error) {
	opts := []wire.Option{
		wire.Identity("id", "plugin_id"),
		wire.Key("hide_builtin_forgot_password", "HideBuiltInForgotPassword"),
		wire.Key("hide_builtin_passkey_button", "HideBuiltInPasskeyButton"),
		wire.Key("hide_builtin_two_factor_button", "HideBuiltInTwoFactorButton"),
		wire.Key("webauthn_origins", "WebAuthnOrigins"),
		wire.Key("webauthn_rp_id", "WebAuthnRpId"),
		wire.CarryServed("oidc_providers", "CreatedAt", "id"),
		wire.WithCodec("enrollment_deadline", sameInstantCodec{}),
		wire.ReadMissingAs("enrollment_deadline", types.StringValue("")),
	}
	for attrPath, sep := range oidcDelimited {
		opts = append(opts, wire.Delimited(attrPath, sep))
	}
	omittedWhenFalse := []string{
		"allow_indefinite_trust", "onboarding_password_require_uppercase",
		"onboarding_password_require_lowercase", "onboarding_password_require_digit",
		"onboarding_password_require_symbol",
	}
	for _, name := range omittedWhenFalse {
		opts = append(opts, wire.ReadMissingAs(name, types.BoolValue(false)))
	}
	return wire.Bind(schemaOf(&JellyfinSecurityPluginConfigurationResource{}), "JellyfinSecurity", opts...)
})

// oidcDelimited maps each list attribute of an OIDC provider that the plugin
// stores as one string to the separator it joins the values with.
var oidcDelimited = map[string]string{
	"oidc_providers.scopes":                            " ",
	"oidc_providers.acr_values":                        " ",
	"oidc_providers.allowed_groups":                    ",",
	"oidc_providers.admin_groups":                      ",",
	"oidc_providers.additional_allowed_cidrs":          ",",
	"oidc_providers.role_library_mappings.library_ids": ",",
}

// delimitedValues rejects values that would read back as other values once
// joined with sep: one holding sep splits in two, and an empty one vanishes.
func delimitedValues(a schema.ListAttribute, attrPath string) schema.ListAttribute {
	sep := oidcDelimited[attrPath]
	name := map[string]string{" ": "a space", ",": "a comma"}[sep]
	a.Validators = append(a.Validators, listvalidator.ValueStringsAre(
		stringvalidator.LengthAtLeast(1),
		stringvalidator.RegexMatches(regexp.MustCompile("^[^"+regexp.QuoteMeta(sep)+"]*$"),
			"must not contain "+name+", which separates the values the plugin stores"),
	))
	return a
}

func (r *JellyfinSecurityPluginConfigurationResource) Wire() (*wire.Binding, error) {
	return securityPluginWire()
}

// NewJellyfinSecurityPluginConfigurationResource creates a new JellyfinSecurity plugin configuration resource.
func NewJellyfinSecurityPluginConfigurationResource() resource.Resource {
	return &JellyfinSecurityPluginConfigurationResource{}
}

func (r *JellyfinSecurityPluginConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_plugin_configuration"
}

func (r *JellyfinSecurityPluginConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
	sensitiveStringList := func(desc string) schema.ListAttribute {
		a := optionalStringList(desc)
		a.Sensitive = true
		return a
	}

	// No UseStateForUnknown on element attributes: it pairs by index, so a
	// reorder would plan another element's values, secrets included.
	elementSensitiveString := func(desc string) schema.StringAttribute {
		a := sensitiveString(desc)
		a.PlanModifiers = nil
		return a
	}

	enrollmentDeadline := optionalString("2FA enrollment deadline as an ISO 8601 date-time, e.g. `2030-01-01T00:00:00Z`, or an empty string for none, which clears a deadline set before.")
	enrollmentDeadline.Validators = []validator.String{
		stringvalidator.RegexMatches(isoDateTimePattern, "must be an ISO 8601 date-time such as 2030-01-01T00:00:00Z, or empty"),
	}
	enrollmentDeadline.PlanModifiers = []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown(), sameInstantPlanModifier{}}

	resp.Schema = schema.Schema{
		Description:         "Manages the JellyfinSecurity plugin configuration with typed attributes.",
		MarkdownDescription: "Manages the JellyfinSecurity plugin configuration with typed attributes.",
		Attributes: map[string]schema.Attribute{
			"plugin_id": pluginIDAttribute(),
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
			"ntfy_token":                         sensitiveString("ntfy access token; takes precedence over ntfy_username/ntfy_password."),
			"ntfy_username":                      optionalString("ntfy username for HTTP Basic auth; used only when ntfy_token is empty."),
			"ntfy_password":                      sensitiveString("ntfy password for HTTP Basic auth."),
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
						"user_id": elementString("Jellyfin user ID."),
						"email":   elementString("User email address."),
					},
				},
				Description:         "User email mappings.",
				MarkdownDescription: "User email mappings.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
					useStateForUnknownByKey([]string{"user_id"}),
				},
			},
			"totp_issuer_name":                   optionalString("TOTP issuer name."),
			"default_language":                   optionalString("Default language code."),
			"pre_verify_window_seconds":          optionalInt("Pre-verify window in seconds."),
			"trust_cookie_ttl_days":              optionalInt("Trust cookie TTL in days."),
			"nat_hairpin_self_ip_bypass":         optionalBool("Enable NAT hairpin self-IP bypass."),
			"default_max_concurrent_sessions":    optionalInt("Default max concurrent sessions per user (0 = unlimited)."),
			"enrollment_deadline":                enrollmentDeadline,
			"webhook_url":                        sensitiveString("Webhook notification URL. Sensitive, as receivers such as Discord and Slack take a token in the URL."),
			"webhook_secret":                     sensitiveString("Webhook signing secret."),
			"webhook_headers":                    sensitiveStringList("Extra webhook headers, one \"Name: Value\" entry each. Sensitive, as receivers authenticate with a header such as Authorization."),
			"geo_ip_asn_db_path":                 optionalString("Path to GeoIP ASN database."),
			"geo_ip_country_db_path":             optionalString("Path to GeoIP country database."),
			"webauthn_rp_id":                     optionalString("WebAuthn relying party ID."),
			"webauthn_origins":                   optionalStringList("WebAuthn allowed origins."),
			"bypass_for_external_auth_providers": optionalBool("Bypass 2FA for external auth providers."),
			"oidc_providers": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: oidcProviderAttributes(elementBool, elementString, elementSensitiveString, elementStringList),
				},
				Description:         "List of OIDC provider configurations.",
				MarkdownDescription: "List of OIDC provider configurations.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
					useStateForUnknownByKey([]string{"id"}),
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
			"pair_device_on_second_screen_approval": optionalBool("Remember a device signed in through a second-screen approval (OIDC device flow or Quick Connect) in the user's paired devices. Requires plugin 2.6.3 or later."),
			"public_base_url":                       optionalString("Public base URL of the server, used for OIDC redirect URIs, pairing QR codes and password reset links. Empty uses the request host. Requires plugin 2.6.3 or later."),
		},
	}
}

func oidcProviderAttributes(
	optionalBool func(string) schema.BoolAttribute,
	optionalString func(string) schema.StringAttribute,
	sensitiveString func(string) schema.StringAttribute,
	optionalStringList func(string) schema.ListAttribute,
) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":                          optionalString("Provider ID (callback URL slug)."),
		"display_name":                optionalString("Display name for the provider."),
		"preset":                      optionalString("Provider preset (e.g. generic, authentik)."),
		"discovery_url":               optionalString("OIDC discovery URL."),
		"client_id":                   optionalString("OIDC client ID."),
		"client_secret":               sensitiveString("OIDC client secret."),
		"scopes":                      delimitedValues(optionalStringList("OIDC scopes (space-delimited on wire)."), "oidc_providers.scopes"),
		"acr_values":                  delimitedValues(optionalStringList("OIDC ACR values (space-delimited on wire)."), "oidc_providers.acr_values"),
		"username_claim":              optionalString("JWT claim for username."),
		"allowed_groups":              delimitedValues(optionalStringList("Groups allowed to log in (comma-delimited on wire)."), "oidc_providers.allowed_groups"),
		"admin_groups":                delimitedValues(optionalStringList("Groups granted admin (comma-delimited on wire)."), "oidc_providers.admin_groups"),
		"allow_admin_group_elevation": optionalBool("Allow admin group elevation."),
		"template_user_id":            optionalString("Template user ID for auto-created users."),
		"auto_create_users":           optionalBool("Auto-create users on first login."),
		"require_idp_mfa":             optionalBool("Require IdP MFA."),
		"bypass_plugin_two_fa":        optionalBool("Bypass plugin 2FA for this provider."),
		"enabled":                     optionalBool("Whether this provider is enabled."),
		"show_login_button":           optionalBool("Show login button for this provider."),
		"force_https":                 optionalBool("Force HTTPS for redirect URI."),
		"allow_private_networks":      optionalBool("Allow private network redirect URIs."),
		"additional_allowed_cidrs":    delimitedValues(optionalStringList("Additional allowed CIDRs (comma-delimited on wire)."), "oidc_providers.additional_allowed_cidrs"),
		"sync_profile_picture":        optionalBool("Sync profile picture from IdP."),
		"picture_claim":               optionalString("JWT claim for profile picture."),
		"prompt_select_account":       optionalBool("Prompt for account selection."),
		"omit_prompt_login":           optionalBool("Omit prompt=login from auth request."),
		"apply_role_library_access":   optionalBool("Apply role-based library access."),
		"role_library_mappings": schema.ListNestedAttribute{
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"role":        optionalString("Role name."),
					"library_ids": delimitedValues(optionalStringList("Library IDs (comma-delimited on wire)."), "oidc_providers.role_library_mappings.library_ids"),
				},
			},
			Description:         "Role-to-library access mappings.",
			MarkdownDescription: "Role-to-library access mappings.",
			Optional:            true,
			Computed:            true,
		},
		"email_claim":                      optionalString("JWT claim for email."),
		"sync_email_from_claim":            optionalBool("Sync email from claim."),
		"button_text":                      optionalString("Login button text."),
		"button_icon_url":                  optionalString("Login button icon URL."),
		"force_password_setup":             optionalBool("Force password setup on first login."),
		"link_existing_users_by_username":  optionalBool("Link an unlinked identity to an existing non-administrator user with the same username."),
		"rp_initiated_logout_enabled":      optionalBool("End the provider session on Jellyfin sign-out (OIDC RP-Initiated Logout)."),
		"rp_initiated_logout_redirect_uri": optionalString("Absolute https post_logout_redirect_uri; must be registered at the IdP."),
		"created_at": schema.StringAttribute{
			Description:         "Creation timestamp (server-managed).",
			MarkdownDescription: "Creation timestamp (server-managed).",
			Computed:            true,
		},
	}
}

func (r *JellyfinSecurityPluginConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *JellyfinSecurityPluginConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *JellyfinSecurityPluginConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.read(ctx, req.State, &resp.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, version, err := r.securityPlugin(ctx); err == nil {
		warnIfNewerThanSupported(version, &resp.Diagnostics)
	}
}

func (r *JellyfinSecurityPluginConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *JellyfinSecurityPluginConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Plugin configuration cannot truly be deleted.
}

func (r *JellyfinSecurityPluginConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("plugin_id"), req, resp)
}

// securityPlugin reports whether GET /Plugins lists the JellyfinSecurity
// plugin, and the version jellyfin_plugin reads for it.
func (r *JellyfinSecurityPluginConfigurationResource) securityPlugin(ctx context.Context) (installed bool, version string, err error) {
	plugins, err := r.client.GetInstalledPlugins(ctx)
	if err != nil {
		return false, "", err
	}
	installed = slices.ContainsFunc(plugins, func(p client.InstalledPlugin) bool {
		return normalizeGUID(p.ID) == normalizeGUID(jellyfinSecurityPluginID)
	})
	if p, found := selectInstalledPlugin(plugins, jellyfinSecurityPluginID, "", ""); found {
		version = p.Version
	}
	return installed, version, nil
}

// requireInstalled reports, unless the JellyfinSecurity plugin is installed,
// why its configuration cannot be written, and otherwise returns its version.
func (r *JellyfinSecurityPluginConfigurationResource) requireInstalled(ctx context.Context, diags *diag.Diagnostics) (version string, ok bool) {
	installed, version, err := r.securityPlugin(ctx)
	switch {
	case err != nil:
		diags.AddError("Failed to check installed plugins", err.Error())
		return "", false
	case !installed:
		diags.AddError(
			"JellyfinSecurity plugin not installed",
			fmt.Sprintf("JellyfinSecurity plugin %s is not installed on the server. Register the plugin repository and install the plugin before managing its configuration, for example with the jellyfin_plugin_repository and jellyfin_plugin resources.", jellyfinSecurityPluginID),
		)
		return "", false
	}
	return version, true
}

func warnIfNewerThanSupported(version string, diags *diag.Diagnostics) {
	if detail, ok := versionNewerWarning("JellyfinSecurity plugin", pluginRelease(version), pluginRelease(supportedSecurityPluginVersion())); ok {
		diags.AddWarning("JellyfinSecurity plugin version newer than supported", detail)
	}
}

func (r *JellyfinSecurityPluginConfigurationResource) apply(ctx context.Context, plan tfsdk.Plan, state *tfsdk.State, diags *diag.Diagnostics) {
	var data JellyfinSecurityPluginConfigurationResourceModel
	diags.Append(plan.Get(ctx, &data)...)
	if diags.HasError() {
		return
	}
	version, ok := r.requireInstalled(ctx, diags)
	if !ok {
		return
	}
	b := wireBinding(diags, securityPluginWire)
	if b == nil || !r.document(data.PluginID.ValueString()).write(ctx, b, &data, diags) {
		return
	}
	data.ID = data.PluginID
	diags.Append(state.Set(ctx, &data)...)
	if diags.HasError() {
		return
	}
	warnIfNewerThanSupported(version, diags)
}

func (r *JellyfinSecurityPluginConfigurationResource) document(pluginID string) document {
	return document{
		what: "JellyfinSecurity plugin configuration",
		get: func(ctx context.Context) (string, error) {
			return r.client.GetPluginConfiguration(ctx, pluginID)
		},
		put: func(ctx context.Context, raw string) error {
			return r.client.UpdatePluginConfiguration(ctx, pluginID, raw)
		},
	}
}

func (r *JellyfinSecurityPluginConfigurationResource) read(ctx context.Context, prior tfsdk.State, state *tfsdk.State, diags *diag.Diagnostics) {
	var data JellyfinSecurityPluginConfigurationResourceModel
	diags.Append(prior.Get(ctx, &data)...)
	if diags.HasError() {
		return
	}
	b := wireBinding(diags, securityPluginWire)
	if b == nil {
		return
	}
	doc := r.document(data.PluginID.ValueString())
	doc.gone = state.RemoveResource
	if !doc.read(ctx, b, &data, diags) {
		return
	}
	data.ID = data.PluginID
	diags.Append(state.Set(ctx, &data)...)
}

// keepSameInstant returns prior when served names the same instant, so a
// configured date-time survives .NET's round-trip rewrite.
func keepSameInstant(prior, served types.String) types.String {
	if sameInstant(prior, served) {
		return prior
	}
	return served
}

func sameInstant(a, b types.String) bool {
	if a.IsNull() || a.IsUnknown() || b.IsNull() || b.IsUnknown() {
		return false
	}
	at, ok := parseISODateTime(a.ValueString())
	if !ok {
		return false
	}
	bt, ok := parseISODateTime(b.ValueString())
	return ok && at.Equal(bt)
}

// sameInstantCodec reads a date-time back as its prior value when both name
// the same instant; see keepSameInstant.
type sameInstantCodec struct{}

func (sameInstantCodec) String() string { return "same-instant" }

func (sameInstantCodec) Encode(_ context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics) {
	s, _ := v.(basetypes.StringValue)
	b, err := json.Marshal(s.ValueString())
	if err != nil {
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("Failed to encode a date-time", err.Error())}
	}
	return b, nil
}

func (sameInstantCodec) Decode(_ context.Context, raw json.RawMessage, prior attr.Value, _ attr.Type) (attr.Value, diag.Diagnostics) {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return types.StringNull(), nil
	}
	p, _ := prior.(basetypes.StringValue)
	return keepSameInstant(p, types.StringValue(s)), nil
}

// sameInstantPlanModifier plans the prior value when the configuration names
// the same instant.
type sameInstantPlanModifier struct{}

func (sameInstantPlanModifier) Description(context.Context) string {
	return "Keeps the prior value when the configured date-time names the same instant."
}

func (m sameInstantPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (sameInstantPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if sameInstant(req.ConfigValue, req.StateValue) {
		resp.PlanValue = req.StateValue
	}
}

func parseISODateTime(v string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
