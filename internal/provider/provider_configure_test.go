// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestConfigureClientBootstrapsUnconfiguredServer(t *testing.T) {
	t.Parallel()

	var calls []string
	mux := http.NewServeMux()
	mux.HandleFunc("/System/Info/Public", func(w http.ResponseWriter, _ *http.Request) {
		writeProviderJSON(t, w, map[string]bool{"StartupWizardCompleted": false})
	})
	mux.HandleFunc("/Startup/Configuration", func(_ http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.Method != http.MethodPost {
			t.Fatalf("startup configuration method = %s, want POST", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding startup configuration body: %v", err)
		}
		if body["UICulture"] != "en-US" || body["MetadataCountryCode"] != "US" || body["PreferredMetadataLanguage"] != "en" {
			t.Fatalf("unexpected startup configuration body: %#v", body)
		}
	})
	mux.HandleFunc("/Startup/User", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.Method == http.MethodGet {
			writeProviderJSON(t, w, map[string]string{"Name": "root"})
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("startup user method = %s, want GET or POST", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding startup user body: %v", err)
		}
		if body["Name"] != "admin" || body["Password"] != "Admin123!" {
			t.Fatalf("unexpected startup user body: %#v", body)
		}
	})
	mux.HandleFunc("/Startup/Complete", func(_ http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.Method != http.MethodPost {
			t.Fatalf("startup complete method = %s, want POST", r.Method)
		}
	})
	mux.HandleFunc("/Users/AuthenticateByName", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding auth body: %v", err)
		}
		if body["Username"] != "admin" || body["Pw"] != "Admin123!" {
			t.Fatalf("unexpected auth body: %#v", body)
		}
		writeProviderJSON(t, w, map[string]string{"AccessToken": "new-token"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c, _, err := configureClient(context.Background(), server.URL, "", "admin", "Admin123!")
	if err != nil {
		t.Fatalf("configureClient() error = %v", err)
	}
	if c.APIKey != "new-token" {
		t.Fatalf("APIKey = %q, want new-token", c.APIKey)
	}

	// Jellyfin initializes the placeholder user on GET /Startup/User before it can be updated with POST /Startup/User.
	wantCalls := []string{"/Startup/Configuration", "/Startup/User", "/Startup/User", "/Startup/Complete", "/Users/AuthenticateByName"}
	if strings.Join(calls, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
}

func TestConfigureClientRequiresCredentialsForUnconfiguredServer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/System/Info/Public" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		writeProviderJSON(t, w, map[string]bool{"StartupWizardCompleted": false})
	}))
	defer server.Close()

	_, _, err := configureClient(context.Background(), server.URL, "stale-api-key", "", "")
	if err == nil {
		t.Fatal("configureClient() error = nil, want missing credentials error")
	}
	if !strings.Contains(err.Error(), "has not been bootstrapped") {
		t.Fatalf("error = %q, want bootstrap credentials message", err.Error())
	}
}

func TestConfigureClientUsesAPIKeyForConfiguredServer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/System/Info/Public" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		writeProviderJSON(t, w, map[string]bool{"StartupWizardCompleted": true})
	}))
	defer server.Close()

	c, _, err := configureClient(context.Background(), server.URL, "api-key", "", "")
	if err != nil {
		t.Fatalf("configureClient() error = %v", err)
	}
	if c.APIKey != "api-key" {
		t.Fatalf("APIKey = %q, want api-key", c.APIKey)
	}
}

func TestConfigureClientAuthenticatesConfiguredServerWithCredentials(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/System/Info/Public", func(w http.ResponseWriter, _ *http.Request) {
		writeProviderJSON(t, w, map[string]bool{"StartupWizardCompleted": true})
	})
	mux.HandleFunc("/Users/AuthenticateByName", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding auth body: %v", err)
		}
		if body["Username"] != "admin" || body["Pw"] != "Admin123!" {
			t.Fatalf("unexpected auth body: %#v", body)
		}
		writeProviderJSON(t, w, map[string]string{"AccessToken": "login-token"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c, _, err := configureClient(context.Background(), server.URL, "", "admin", "Admin123!")
	if err != nil {
		t.Fatalf("configureClient() error = %v", err)
	}
	if c.APIKey != "login-token" {
		t.Fatalf("APIKey = %q, want login-token", c.APIKey)
	}
}

func writeProviderJSON(t *testing.T, w http.ResponseWriter, v interface{}) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encoding response: %v", err)
	}
}

// An endpoint unknown until apply must not fall back to JELLYFIN_ENDPOINT,
// which may name another server.
func TestConfigureRefusesUnknownConfiguration(t *testing.T) {
	t.Setenv("JELLYFIN_ENDPOINT", "http://another-server.invalid")
	t.Setenv("JELLYFIN_API_KEY", "key-of-another-server")
	ctx := context.Background()
	p := New("test")()
	var schemaResp provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)
	config := tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), map[string]tftypes.Value{
		"endpoint": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"api_key":  tftypes.NewValue(tftypes.String, nil),
		"username": tftypes.NewValue(tftypes.String, nil),
		"password": tftypes.NewValue(tftypes.String, nil),
	})}

	var resp provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: config}, &resp)
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "endpoint is unknown until apply") || resp.ResourceData != nil {
		t.Errorf("Configure() = %v with resource data %v, want an unknown endpoint refused", resp.Diagnostics, resp.ResourceData)
	}

	var deferred provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: config, ClientCapabilities: provider.ConfigureProviderClientCapabilities{DeferralAllowed: true}}, &deferred)
	if deferred.Diagnostics.HasError() || deferred.Deferred == nil || deferred.Deferred.Reason != provider.DeferredReasonProviderConfigUnknown {
		t.Errorf("Configure() with deferral allowed = %v, deferred %v, want the configuration deferred", deferred.Diagnostics, deferred.Deferred)
	}
}
