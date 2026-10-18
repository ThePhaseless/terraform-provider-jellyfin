// Copyright Sofie Finnes 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// Proxy authentication to a real server, but use a replacement-only plugin
// document that can omit fields and change their types between refreshes.
func TestAccPluginConfigurationRefreshOwnership(t *testing.T) {
	testAccPreCheck(t)
	endpoint, err := url.Parse(os.Getenv("JELLYFIN_ENDPOINT"))
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(endpoint) // #nosec G704 -- Acceptance tests explicitly trust JELLYFIN_ENDPOINT.
	var mu sync.Mutex
	stored := `{"section":{"owned":1,"runtime":"keep"}}`
	posts := 0
	set := func(raw string) { mu.Lock(); defer mu.Unlock(); stored = raw }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/Plugins/") || !strings.HasSuffix(r.URL.Path, "/Configuration") {
			proxy.ServeHTTP(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "cannot read", http.StatusBadRequest)
				return
			}
			stored = string(body)
			posts++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, stored)
	}))
	t.Cleanup(server.Close)
	t.Setenv("JELLYFIN_ENDPOINT", server.URL)
	config := func(raw string) string {
		return fmt.Sprintf(`resource "jellyfin_plugin_configuration" "test" {
 plugin_id = "8c95c4d2e50c4fb0a4f36c06ff0f9a1a"
 configuration_json = %q
}`, raw)
	}
	desired := config(`{"section":{"owned":2}}`)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: desired},
			{PreConfig: func() { set(`{}`) }, RefreshState: true, ExpectNonEmptyPlan: true},
			{RefreshState: true, ExpectNonEmptyPlan: true},
			{PreConfig: func() { set(`{"section":{"owned":2,"runtime":"new"}}`) }, Config: desired, ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
			{PreConfig: func() { set(`{"section":false}`) }, RefreshState: true, ExpectNonEmptyPlan: true},
			{RefreshState: true, ExpectNonEmptyPlan: true},
			{Config: desired},
			{PreConfig: func() { set(`{}`) }, Config: config(`{}`)},
			{PreConfig: func() { set(`{"section":{"owned":99}}`) }, Config: config(`{}`), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
		},
	})
	mu.Lock()
	defer mu.Unlock()
	if posts != 2 {
		t.Errorf("got %d writes; want initial change and repair only", posts)
	}
}
