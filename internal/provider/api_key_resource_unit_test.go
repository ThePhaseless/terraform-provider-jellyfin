// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func apiKeyClient(t *testing.T, keys []client.APIKey) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Auth/Keys" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": keys})
	}))
	t.Cleanup(srv.Close)
	return client.NewClient(srv.URL, "test-key")
}

func TestAPIKeyImportState(t *testing.T) {
	t.Parallel()

	keys := []client.APIKey{
		{AccessToken: "tok-sonarr", AppName: "Sonarr"},
		{AccessToken: "tok-east", AppName: "Grafana"},
		{AccessToken: "tok-west", AppName: "Grafana"},
	}

	tests := map[string]struct {
		importID  string
		wantToken string
		wantName  string
		wantError string
	}{
		"by access token":                  {importID: "tok-sonarr", wantToken: "tok-sonarr", wantName: "Sonarr"},
		"by access token of a shared name": {importID: "tok-west", wantToken: "tok-west", wantName: "Grafana"},
		"by unique app name":               {importID: "Sonarr", wantToken: "tok-sonarr", wantName: "Sonarr"},
		"by shared app name":               {importID: "Grafana", wantError: `2 API keys have the app name "Grafana". Import one of them by its access token.`},
		"by app name in another case":      {importID: "sonarr", wantError: `No API key has "sonarr" as its access token or app name.`},
		"by unknown ID":                    {importID: "missing", wantError: `No API key has "missing" as its access token or app name.`},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			r := &APIKeyResource{}
			configure(t, r, apiKeyClient(t, keys))
			s := schemaOf(r)
			resp := resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}}
			r.ImportState(ctx, resource.ImportStateRequest{ID: tc.importID}, &resp)

			if tc.wantError != "" {
				if len(resp.Diagnostics.Errors()) != 1 || resp.Diagnostics.Errors()[0].Detail() != tc.wantError {
					t.Fatalf("ImportState() = %v, want one error %q", resp.Diagnostics, tc.wantError)
				}
				for _, k := range keys {
					if strings.Contains(resp.Diagnostics.Errors()[0].Detail(), k.AccessToken) {
						t.Errorf("error %q holds the access token %s", resp.Diagnostics.Errors()[0].Detail(), k.AccessToken)
					}
				}
				return
			}
			if resp.Diagnostics.HasError() {
				t.Fatalf("ImportState() = %v", resp.Diagnostics)
			}
			var got APIKeyResourceModel
			if d := resp.State.Get(ctx, &got); d.HasError() {
				t.Fatal(d)
			}
			if got.ID.ValueString() != tc.wantToken || got.AccessToken.ValueString() != tc.wantToken || got.AppName.ValueString() != tc.wantName {
				t.Errorf("imported id %v, access_token %v, app_name %v; want id and access_token %s, app_name %s", got.ID, got.AccessToken, got.AppName, tc.wantToken, tc.wantName)
			}
		})
	}
}
