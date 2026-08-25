// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
