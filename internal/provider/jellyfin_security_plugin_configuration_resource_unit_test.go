// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnitJellyfinSecurityNotificationAuthRoundTrip(t *testing.T) {
	ctx := context.Background()
	fixture := `{"NtfyToken":"tk_abc","NtfyUsername":"alice","NtfyPassword":"s3cret","WebhookHeaders":["X-Api-Key: k","Authorization: Bearer t"]}`

	var data JellyfinSecurityPluginConfigurationResourceModel
	var diags diag.Diagnostics
	flattenJellyfinSecurity(ctx, fixture, &data, &diags)
	if diags.HasError() {
		t.Fatalf("flatten: %v", diags.Errors())
	}

	if got := data.NtfyToken.ValueString(); got != "tk_abc" {
		t.Errorf("ntfy_token = %q, want %q", got, "tk_abc")
	}
	if got := data.NtfyUsername.ValueString(); got != "alice" {
		t.Errorf("ntfy_username = %q, want %q", got, "alice")
	}
	if got := data.NtfyPassword.ValueString(); got != "s3cret" {
		t.Errorf("ntfy_password = %q, want %q", got, "s3cret")
	}
	var headers []string
	if d := data.WebhookHeaders.ElementsAs(ctx, &headers, false); d.HasError() {
		t.Fatalf("webhook_headers: %v", d.Errors())
	}
	if len(headers) != 2 || headers[0] != "X-Api-Key: k" || headers[1] != "Authorization: Bearer t" {
		t.Errorf("webhook_headers = %q", headers)
	}

	base := map[string]json.RawMessage{}
	if d := overlayJellyfinSecurity(ctx, base, &data); d.HasError() {
		t.Fatalf("overlay: %v", d.Errors())
	}

	for key, want := range map[string]string{
		"NtfyToken":      `"tk_abc"`,
		"NtfyUsername":   `"alice"`,
		"NtfyPassword":   `"s3cret"`,
		"WebhookHeaders": `["X-Api-Key: k","Authorization: Bearer t"]`,
	} {
		if got := string(base[key]); got != want {
			t.Errorf("%s = %s, want %s", key, got, want)
		}
	}
}

func TestUnitOidcProviderRpInitiatedLogoutRoundTrip(t *testing.T) {
	ctx := context.Background()
	m := map[string]json.RawMessage{
		"RpInitiatedLogoutEnabled":     json.RawMessage(`true`),
		"RpInitiatedLogoutRedirectUri": json.RawMessage(`"https://example.com/bye"`),
	}

	var diags diag.Diagnostics
	attrs := oidcProviderAttrs(ctx, m, &diags)
	if diags.HasError() {
		t.Fatalf("attrs: %v", diags.Errors())
	}

	enabled, ok := attrs["rp_initiated_logout_enabled"].(types.Bool)
	if !ok || !enabled.ValueBool() {
		t.Errorf("rp_initiated_logout_enabled = %v, want true", attrs["rp_initiated_logout_enabled"])
	}
	redirect, ok := attrs["rp_initiated_logout_redirect_uri"].(types.String)
	if !ok || redirect.ValueString() != "https://example.com/bye" {
		t.Errorf("rp_initiated_logout_redirect_uri = %v", attrs["rp_initiated_logout_redirect_uri"])
	}

	p := OidcProviderModel{
		RpInitiatedLogoutEnabled:     types.BoolValue(true),
		RpInitiatedLogoutRedirectURI: types.StringValue("https://example.com/bye"),
	}
	out := map[string]json.RawMessage{}
	overlayOidcProvider(ctx, out, &p)

	if got := string(out["RpInitiatedLogoutEnabled"]); got != "true" {
		t.Errorf("RpInitiatedLogoutEnabled = %s, want true", got)
	}
	if got := string(out["RpInitiatedLogoutRedirectUri"]); got != `"https://example.com/bye"` {
		t.Errorf("RpInitiatedLogoutRedirectUri = %s", got)
	}
}

func TestUnitJellyfinSecurityPlugin263FieldsRoundTrip(t *testing.T) {
	ctx := context.Background()
	fixture := `{"PairDeviceOnSecondScreenApproval":true,"PublicBaseUrl":"https://jf.example.com","OidcProviders":[{"Id":"idp","LinkExistingUsersByUsername":true}]}`

	var data JellyfinSecurityPluginConfigurationResourceModel
	var diags diag.Diagnostics
	flattenJellyfinSecurity(ctx, fixture, &data, &diags)
	if diags.HasError() {
		t.Fatalf("flatten: %v", diags.Errors())
	}

	if !data.PairDeviceOnSecondScreenApproval.ValueBool() {
		t.Errorf("pair_device_on_second_screen_approval = %v, want true", data.PairDeviceOnSecondScreenApproval)
	}
	if got := data.PublicBaseURL.ValueString(); got != "https://jf.example.com" {
		t.Errorf("public_base_url = %q", got)
	}
	var providers []OidcProviderModel
	if d := data.OidcProviders.ElementsAs(ctx, &providers, false); d.HasError() {
		t.Fatalf("oidc_providers: %v", d.Errors())
	}
	if len(providers) != 1 || !providers[0].LinkExistingUsersByUsername.ValueBool() {
		t.Fatalf("oidc_providers = %+v, want one provider with link_existing_users_by_username", providers)
	}

	base := map[string]json.RawMessage{}
	if d := overlayJellyfinSecurity(ctx, base, &data); d.HasError() {
		t.Fatalf("overlay: %v", d.Errors())
	}
	if got := string(base["PairDeviceOnSecondScreenApproval"]); got != "true" {
		t.Errorf("PairDeviceOnSecondScreenApproval = %s", got)
	}
	if got := string(base["PublicBaseUrl"]); got != `"https://jf.example.com"` {
		t.Errorf("PublicBaseUrl = %s", got)
	}
	var written []map[string]json.RawMessage
	if err := json.Unmarshal(base["OidcProviders"], &written); err != nil {
		t.Fatalf("OidcProviders: %v", err)
	}
	if len(written) != 1 || string(written[0]["LinkExistingUsersByUsername"]) != "true" {
		t.Errorf("OidcProviders = %s", base["OidcProviders"])
	}
}

func TestUnitJellyfinSecurityPlugin263FieldsAbsentStayNullAndUnwritten(t *testing.T) {
	ctx := context.Background()

	var data JellyfinSecurityPluginConfigurationResourceModel
	var diags diag.Diagnostics
	flattenJellyfinSecurity(ctx, `{"Enabled":true}`, &data, &diags)
	if diags.HasError() {
		t.Fatalf("flatten: %v", diags.Errors())
	}

	if !data.PairDeviceOnSecondScreenApproval.IsNull() || !data.PublicBaseURL.IsNull() {
		t.Errorf("pair_device_on_second_screen_approval = %v, public_base_url = %v, want both null", data.PairDeviceOnSecondScreenApproval, data.PublicBaseURL)
	}

	base := map[string]json.RawMessage{}
	if d := overlayJellyfinSecurity(ctx, base, &data); d.HasError() {
		t.Fatalf("overlay: %v", d.Errors())
	}
	for _, key := range []string{"PairDeviceOnSecondScreenApproval", "PublicBaseUrl"} {
		if raw, ok := base[key]; ok {
			t.Errorf("%s written as %s, want it left out", key, raw)
		}
	}
}

func TestUnitKeepSameInstant(t *testing.T) {
	cases := []struct {
		name   string
		prior  types.String
		served types.String
		want   types.String
	}{
		{"dotnet layout of the configured instant", types.StringValue("2030-01-01T00:00:00Z"), types.StringValue("2030-01-01T00:00:00.0000000Z"), types.StringValue("2030-01-01T00:00:00Z")},
		{"offset spelling of the same instant", types.StringValue("2030-01-01T02:00:00+02:00"), types.StringValue("2030-01-01T00:00:00.0000000Z"), types.StringValue("2030-01-01T02:00:00+02:00")},
		{"different instant", types.StringValue("2030-01-01T00:00:00Z"), types.StringValue("2031-01-01T00:00:00.0000000Z"), types.StringValue("2031-01-01T00:00:00.0000000Z")},
		{"nothing configured", types.StringNull(), types.StringValue("2030-01-01T00:00:00.0000000Z"), types.StringValue("2030-01-01T00:00:00.0000000Z")},
		{"cleared on the server", types.StringValue("2030-01-01T00:00:00Z"), types.StringNull(), types.StringNull()},
		{"unparseable prior", types.StringValue("soon"), types.StringValue("2030-01-01T00:00:00.0000000Z"), types.StringValue("2030-01-01T00:00:00.0000000Z")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := keepSameInstant(c.prior, c.served); !got.Equal(c.want) {
				t.Errorf("keepSameInstant(%v, %v) = %v, want %v", c.prior, c.served, got, c.want)
			}
		})
	}
}

func TestUnitSameInstantPlanModifier(t *testing.T) {
	ctx := context.Background()
	state := types.StringValue("2030-01-01T00:00:00.0000000Z")

	for _, c := range []struct {
		name   string
		config types.String
		want   types.String
	}{
		{"same instant plans the state value", types.StringValue("2030-01-01T00:00:00Z"), state},
		{"different instant plans the configured value", types.StringValue("2031-01-01T00:00:00Z"), types.StringValue("2031-01-01T00:00:00Z")},
	} {
		t.Run(c.name, func(t *testing.T) {
			req := planmodifier.StringRequest{ConfigValue: c.config, StateValue: state, PlanValue: c.config}
			resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
			sameInstantPlanModifier{}.PlanModifyString(ctx, req, resp)
			if !resp.PlanValue.Equal(c.want) {
				t.Errorf("plan = %v, want %v", resp.PlanValue, c.want)
			}
		})
	}
}
