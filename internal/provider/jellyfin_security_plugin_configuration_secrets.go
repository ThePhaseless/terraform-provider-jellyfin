// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/release"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

// maskedMarker is what JellyfinSecurity 2.6.5 and later serve in place of a
// stored, non-empty secret, and what a write takes as "keep the stored one".
const maskedMarker = "(saved)"

// securityPluginSecrets are the top-level secrets the plugin masks, each with
// a write-only form named with the suffix _wo and its version _wo_version.
var securityPluginSecrets = []string{
	"smtp_password", "ntfy_token", "ntfy_password", "gotify_app_token",
	"telegram_bot_token", "webhook_secret", "webhook_ed25519_private_key",
}

const (
	webhookHeadersAttr = "webhook_headers"
	oidcProvidersAttr  = "oidc_providers"
	oidcWriteOnlyMap   = "oidc_client_secrets_wo"
	woSuffix           = "_wo"
	woVersionSuffix    = "_wo_version"
	woRequires         = " Requires Terraform 1.11 or later and JellyfinSecurity 2.6.5 or later."
	woHidesDrift       = " Terraform cannot read the stored value back, so it does not see a change made outside Terraform."
)

var headerNamePattern = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func deprecatedFor(oldName, newName string) string {
	return fmt.Sprintf("Use `%[1]s` and `%[1]s_version` instead, which keep the secret out of the plan and state (Terraform 1.11 or later). `%[2]s` will be removed in the next major release.", newName, oldName)
}

func woVersionAttribute(woName string) schema.Int64Attribute {
	desc := fmt.Sprintf("Change this to send `%s` again, for example after rotating the secret.", woName)
	return schema.Int64Attribute{
		Description:         desc,
		MarkdownDescription: desc,
		Optional:            true,
		Validators:          []validator.Int64{int64validator.AlsoRequires(path.MatchRoot(woName))},
	}
}

func woMapAttribute(desc string, validators ...validator.Map) schema.MapAttribute {
	return schema.MapAttribute{
		ElementType:         types.StringType,
		Description:         desc,
		MarkdownDescription: desc,
		Optional:            true,
		Sensitive:           true,
		WriteOnly:           true,
		Validators:          validators,
	}
}

// addSecurityPluginWriteOnly adds the write-only form of each secret to attrs
// and deprecates the secret attributes they replace.
func addSecurityPluginWriteOnly(attrs map[string]schema.Attribute) {
	for _, s := range securityPluginSecrets {
		a, ok := attrs[s].(schema.StringAttribute)
		if !ok {
			panic(fmt.Sprintf("%s is a %T, not a string attribute", s, attrs[s]))
		}
		a.DeprecationMessage = deprecatedFor(s, s+woSuffix)
		attrs[s] = a
		desc := fmt.Sprintf("Write-only form of `%[1]s`, never stored in plan or state. Sent on create and whenever `%[1]s_wo_version` changes; otherwise the plugin keeps what it has. An empty string clears it.", s) + woHidesDrift + woRequires
		attrs[s+woSuffix] = schema.StringAttribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Sensitive:           true,
			WriteOnly:           true,
			Validators: []validator.String{
				stringvalidator.ConflictsWith(path.MatchRoot(s)),
				stringvalidator.AlsoRequires(path.MatchRoot(s + woVersionSuffix)),
			},
		}
		attrs[s+woVersionSuffix] = woVersionAttribute(s + woSuffix)
	}

	headers, ok := attrs[webhookHeadersAttr].(schema.ListAttribute)
	if !ok {
		panic(fmt.Sprintf("%s is a %T, not a list attribute", webhookHeadersAttr, attrs[webhookHeadersAttr]))
	}
	headers.DeprecationMessage = deprecatedFor(webhookHeadersAttr, webhookHeadersAttr+woSuffix)
	attrs[webhookHeadersAttr] = headers
	attrs[webhookHeadersAttr+woSuffix] = woMapAttribute(
		"Write-only extra webhook headers, keyed by header name, never stored in plan or state. Sent as `Name: value` lines sorted by name, which replace every header, on create and whenever `webhook_headers_wo_version` changes; otherwise the plugin keeps what it has. It cannot hold two headers with the same name."+woHidesDrift+woRequires,
		mapvalidator.ConflictsWith(path.MatchRoot(webhookHeadersAttr)),
		mapvalidator.AlsoRequires(path.MatchRoot(webhookHeadersAttr+woVersionSuffix)),
		mapvalidator.KeysAre(stringvalidator.RegexMatches(headerNamePattern, "must be an HTTP header name")),
		mapvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
		mapvalidator.NoNullValues(),
	)
	attrs[webhookHeadersAttr+woVersionSuffix] = woVersionAttribute(webhookHeadersAttr + woSuffix)

	providers, ok := attrs[oidcProvidersAttr].(schema.ListNestedAttribute)
	if !ok {
		panic(fmt.Sprintf("%s is a %T, not a nested list", oidcProvidersAttr, attrs[oidcProvidersAttr]))
	}
	secret, ok := providers.NestedObject.Attributes["client_secret"].(schema.StringAttribute)
	if !ok {
		panic(fmt.Sprintf("%s.client_secret is a %T, not a string attribute", oidcProvidersAttr, providers.NestedObject.Attributes["client_secret"]))
	}
	secret.DeprecationMessage = deprecatedFor("client_secret", oidcWriteOnlyMap)
	providers.NestedObject.Attributes["client_secret"] = secret
	attrs[oidcWriteOnlyMap] = woMapAttribute(
		"Write-only OIDC client secrets keyed by provider `id`, never stored in plan or state. Sent on create, whenever `oidc_client_secrets_wo_version` changes, and for a provider `id` the plugin does not have yet; otherwise the plugin keeps what it has. An empty string clears a provider's secret."+woHidesDrift+woRequires,
		mapvalidator.AlsoRequires(path.MatchRoot(oidcWriteOnlyMap+"_version")),
		mapvalidator.ConflictsWith(path.MatchRoot(oidcProvidersAttr).AtAnyListIndex().AtName("client_secret")),
		mapvalidator.NoNullValues(),
	)
	attrs[oidcWriteOnlyMap+"_version"] = woVersionAttribute(oidcWriteOnlyMap)
}

// securityPluginSecretOptions binds the write-only attributes, which apply
// sends itself, and reads the masked secrets back as configured.
func securityPluginSecretOptions() []wire.Option {
	const reason = "write-only: apply sends it from the configuration"
	opts := []wire.Option{
		wire.WithCodec(webhookHeadersAttr, savedHeadersCodec{}),
		wire.WithCodec(oidcProvidersAttr+".client_secret", savedSecretCodec{}),
	}
	for _, name := range append(slices.Clone(securityPluginSecrets), webhookHeadersAttr) {
		opts = append(opts, wire.Elsewhere(name+woSuffix, reason), wire.Elsewhere(name+woVersionSuffix, reason))
	}
	for _, s := range securityPluginSecrets {
		opts = append(opts, wire.WithCodec(s, savedSecretCodec{}))
	}
	return append(opts, wire.Elsewhere(oidcWriteOnlyMap, reason), wire.Elsewhere(oidcWriteOnlyMap+"_version", reason))
}

// maskedSecret reports whether s is what the plugin serves for a secret: the
// marker for a stored one, or empty for none.
func maskedSecret(s string) bool { return s == "" || s == maskedMarker }

// maskHeader masks a "Name: Value" line the way the plugin serves it.
func maskHeader(line string) string {
	name, value, ok := strings.Cut(line, ":")
	name, value = strings.TrimSpace(name), strings.TrimSpace(value)
	if !ok || name == "" || value == "" {
		return line
	}
	return name + ": " + maskedMarker
}

// savedSecretCodec reads a served marker back as the prior value it stands
// for, so a configured secret keeps its configured value in state. Inside
// oidc_providers, wire pairs the prior element by index while the list keeps
// its length, so a reorder made outside Terraform reads the marker as is.
type savedSecretCodec struct{}

func (savedSecretCodec) String() string { return "saved-secret" }

func (savedSecretCodec) Encode(_ context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics) {
	s, _ := v.(basetypes.StringValue)
	return marshalJSON(s.ValueString())
}

func (savedSecretCodec) Decode(_ context.Context, raw json.RawMessage, prior attr.Value, _ attr.Type) (attr.Value, diag.Diagnostics) {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return types.StringNull(), nil
	}
	if p, ok := prior.(basetypes.StringValue); ok && s == maskedMarker && !p.IsNull() && !p.IsUnknown() && p.ValueString() != "" {
		return p, nil
	}
	return types.StringValue(s), nil
}

// savedHeadersCodec reads served header lines back as the prior lines when
// each is that line or that line masked.
type savedHeadersCodec struct{}

func (savedHeadersCodec) String() string { return "saved-headers" }

func (savedHeadersCodec) Encode(_ context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics) {
	return marshalJSON(listStrings(v))
}

func (savedHeadersCodec) Decode(_ context.Context, raw json.RawMessage, prior attr.Value, _ attr.Type) (attr.Value, diag.Diagnostics) {
	var served []string
	if json.Unmarshal(raw, &served) != nil {
		return types.ListNull(types.StringType), nil
	}
	if p, ok := prior.(basetypes.ListValue); ok && !p.IsNull() && !p.IsUnknown() && len(p.Elements()) == len(served) {
		was := listStrings(p)
		same := true
		for i, line := range served {
			if line != was[i] && line != maskHeader(was[i]) {
				same = false
				break
			}
		}
		if same {
			return p, nil
		}
	}
	elems := make([]attr.Value, len(served))
	for i, line := range served {
		elems[i] = types.StringValue(line)
	}
	return types.ListValueMust(types.StringType, elems), nil
}

// listStrings returns the strings of a list value, an unknown one as "".
func listStrings(v attr.Value) []string {
	l, _ := v.(basetypes.ListValue)
	out := make([]string, 0, len(l.Elements()))
	for _, e := range l.Elements() {
		s, _ := e.(basetypes.StringValue)
		out = append(out, s.ValueString())
	}
	return out
}

func marshalJSON(v any) (json.RawMessage, diag.Diagnostics) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("Failed to encode a JellyfinSecurity value", err.Error())}
	}
	return b, nil
}

// woSends reports whether a write-only value is sent: on create, and while
// its configured version differs from the prior one.
func woSends(cfgVersion, priorVersion types.Int64, creating bool) bool {
	return !cfgVersion.IsNull() && (creating || cfgVersion.IsUnknown() || !cfgVersion.Equal(priorVersion))
}

// woVersions reads the configured and prior version named name.
func woVersions(ctx context.Context, config tfsdk.Config, prior *tfsdk.State, name string, diags *diag.Diagnostics) (cfg, was types.Int64) {
	diags.Append(config.GetAttribute(ctx, path.Root(name), &cfg)...)
	if prior != nil && !prior.Raw.IsNull() {
		diags.Append(prior.GetAttribute(ctx, path.Root(name), &was)...)
	}
	return cfg, was
}

// ModifyPlan plans a secret unknown whenever its write-only form is sent, and
// whenever its write-only form is in use but the plan holds a value the
// plugin would not serve, such as a secret the deprecated attribute stored:
// the read after the apply stores what the plugin serves instead.
func (r *JellyfinSecurityPluginConfigurationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	creating := req.State.Raw.IsNull()
	diags := &resp.Diagnostics
	versions := func(name string) (configured, sends bool) {
		cfg, was := woVersions(ctx, req.Config, &req.State, name, diags)
		return !cfg.IsNull(), woSends(cfg, was, creating)
	}

	for _, s := range securityPluginSecrets {
		configured, sends := versions(s + woVersionSuffix)
		if !configured {
			continue
		}
		var planned types.String
		diags.Append(req.Plan.GetAttribute(ctx, path.Root(s), &planned)...)
		if sends || !planned.IsNull() && !planned.IsUnknown() && !maskedSecret(planned.ValueString()) {
			diags.Append(resp.Plan.SetAttribute(ctx, path.Root(s), types.StringUnknown())...)
		}
	}

	if configured, sends := versions(webhookHeadersAttr + woVersionSuffix); configured {
		var planned types.List
		diags.Append(req.Plan.GetAttribute(ctx, path.Root(webhookHeadersAttr), &planned)...)
		masked := !slices.ContainsFunc(listStrings(planned), func(line string) bool { return maskHeader(line) != line })
		if sends || !planned.IsNull() && !planned.IsUnknown() && !masked {
			diags.Append(resp.Plan.SetAttribute(ctx, path.Root(webhookHeadersAttr), types.ListUnknown(types.StringType))...)
		}
	}

	configured, sends := versions(oidcWriteOnlyMap + "_version")
	if !configured || diags.HasError() {
		return
	}
	var secrets types.Map
	var cfgProviders, planned types.List
	diags.Append(req.Config.GetAttribute(ctx, path.Root(oidcWriteOnlyMap), &secrets)...)
	diags.Append(req.Config.GetAttribute(ctx, path.Root(oidcProvidersAttr), &cfgProviders)...)
	diags.Append(req.Plan.GetAttribute(ctx, path.Root(oidcProvidersAttr), &planned)...)
	if diags.HasError() {
		return
	}
	if !secrets.IsUnknown() && !cfgProviders.IsUnknown() {
		if ids, known := providerIDs(cfgProviders); known {
			for _, k := range slices.Sorted(maps.Keys(secrets.Elements())) {
				if !ids[k] {
					diags.AddAttributeError(path.Root(oidcWriteOnlyMap).AtMapKey(k), "Unknown OIDC provider",
						fmt.Sprintf("no oidc_providers element has id %q", k))
				}
			}
		}
	}
	if planned.IsNull() || planned.IsUnknown() {
		return
	}
	// A provider id new to the state gets its secret sent whatever the version.
	var was types.List
	if !creating {
		diags.Append(req.State.GetAttribute(ctx, path.Root(oidcProvidersAttr), &was)...)
	}
	priorIDs, _ := providerIDs(was)
	for i, e := range planned.Elements() {
		obj, ok := e.(basetypes.ObjectValue)
		if !ok || obj.IsNull() || obj.IsUnknown() {
			continue
		}
		id, _ := obj.Attributes()["id"].(basetypes.StringValue)
		_, targeted := secrets.Elements()[id.ValueString()]
		if !secrets.IsUnknown() && (id.IsNull() || id.IsUnknown() || !targeted) {
			continue
		}
		cs, _ := obj.Attributes()["client_secret"].(basetypes.StringValue)
		if sends || !priorIDs[id.ValueString()] || !cs.IsNull() && !cs.IsUnknown() && !maskedSecret(cs.ValueString()) {
			diags.Append(resp.Plan.SetAttribute(ctx, path.Root(oidcProvidersAttr).AtListIndex(i).AtName("client_secret"), types.StringUnknown())...)
		}
	}
}

// providerIDs returns the ids of the configured providers, and whether each
// one is known.
func providerIDs(providers types.List) (map[string]bool, bool) {
	ids := map[string]bool{}
	for _, e := range providers.Elements() {
		obj, ok := e.(basetypes.ObjectValue)
		if !ok || obj.IsUnknown() {
			return nil, false
		}
		id, ok := obj.Attributes()["id"].(basetypes.StringValue)
		if !ok || id.IsUnknown() {
			return nil, false
		}
		if !id.IsNull() {
			ids[id.ValueString()] = true
		}
	}
	return ids, true
}

// requireMaskingPlugin fails when config sets a write-only secret and the
// plugin is older than 2.6.5, which serves stored secrets in plain text that
// the read after the apply would put in state.
func requireMaskingPlugin(ctx context.Context, config tfsdk.Config, version string, diags *diag.Diagnostics) {
	if release.Compare(pluginRelease(version), "2.6.5") >= 0 {
		return
	}
	for _, name := range append(slices.Clone(securityPluginSecrets), webhookHeadersAttr, "oidc_client_secrets") {
		var v types.Int64
		diags.Append(config.GetAttribute(ctx, path.Root(name+woVersionSuffix), &v)...)
		if !v.IsNull() {
			diags.AddAttributeError(path.Root(name+woVersionSuffix), "JellyfinSecurity plugin too old for write-only secrets", fmt.Sprintf(
				"JellyfinSecurity %s serves stored secrets in plain text, so the read after the apply would put `%s` in state. "+
					"Write-only secrets need JellyfinSecurity 2.6.5 or later: upgrade the plugin, or set the secret with its deprecated attribute.", version, name+woSuffix))
			return
		}
	}
}

// securityPluginPatch returns the document patch that sends the write-only
// values woSends picks from config, and carries each OIDC provider's served
// client secret over by id where the plan writes none, which the plugin then
// keeps. Every value the plan leaves unwritten keeps what the plugin served,
// the marker on 2.6.5, which the plugin resolves to the stored secret.
func securityPluginPatch(ctx context.Context, config tfsdk.Config, prior *tfsdk.State, diags *diag.Diagnostics) func(context.Context, map[string]json.RawMessage, map[string]json.RawMessage) diag.Diagnostics {
	creating := prior == nil || prior.Raw.IsNull()
	sends := func(name string) bool {
		cfg, was := woVersions(ctx, config, prior, name, diags)
		return woSends(cfg, was, creating)
	}
	set := map[string]json.RawMessage{}
	put := func(name string, v any) {
		raw, d := marshalJSON(v)
		diags.Append(d...)
		set[wire.KeyOf(name)] = raw
	}
	for _, s := range securityPluginSecrets {
		if sends(s + woVersionSuffix) {
			var v types.String
			diags.Append(config.GetAttribute(ctx, path.Root(s+woSuffix), &v)...)
			put(s, v.ValueString())
		}
	}
	if sends(webhookHeadersAttr + woVersionSuffix) {
		var headers map[string]string
		diags.Append(config.GetAttribute(ctx, path.Root(webhookHeadersAttr+woSuffix), &headers)...)
		lines := make([]string, 0, len(headers))
		for _, name := range slices.Sorted(maps.Keys(headers)) {
			lines = append(lines, name+": "+headers[name])
		}
		put(webhookHeadersAttr, lines)
	}
	cfg, was := woVersions(ctx, config, prior, oidcWriteOnlyMap+"_version", diags)
	var secrets map[string]string
	if !cfg.IsNull() {
		diags.Append(config.GetAttribute(ctx, path.Root(oidcWriteOnlyMap), &secrets)...)
	}
	all := woSends(cfg, was, creating)
	return func(_ context.Context, served, doc map[string]json.RawMessage) diag.Diagnostics {
		maps.Copy(doc, set)
		return carryClientSecrets(served, doc, secrets, all)
	}
}

// carryClientSecrets sets the client secret of each provider in doc that
// secrets has, when all is set or the plugin serves no provider with its id,
// and gives each other provider without one the secret served for its id.
func carryClientSecrets(served, doc map[string]json.RawMessage, secrets map[string]string, all bool) diag.Diagnostics {
	key, idKey, secretKey := wire.KeyOf(oidcProvidersAttr), wire.KeyOf("id"), wire.KeyOf("client_secret")
	var providers []map[string]json.RawMessage
	if raw, ok := doc[key]; !ok || json.Unmarshal(raw, &providers) != nil || providers == nil {
		return nil
	}
	var was []map[string]json.RawMessage
	// The served secrets are only a courtesy, as for CarryServed.
	_ = json.Unmarshal(served[key], &was)
	servedSecrets := map[string]json.RawMessage{}
	for _, p := range was {
		var id string
		if json.Unmarshal(p[idKey], &id) == nil {
			servedSecrets[id] = p[secretKey]
		}
	}
	var diags diag.Diagnostics
	for _, p := range providers {
		var id string
		if json.Unmarshal(p[idKey], &id) != nil {
			continue
		}
		servedSecret, seen := servedSecrets[id]
		if v, ok := secrets[id]; ok && (all || !seen) {
			raw, d := marshalJSON(v)
			diags.Append(d...)
			p[secretKey] = raw
		} else if _, has := p[secretKey]; !has && servedSecret != nil {
			p[secretKey] = servedSecret
		}
	}
	raw, d := marshalJSON(providers)
	diags.Append(d...)
	doc[key] = raw
	return diags
}
