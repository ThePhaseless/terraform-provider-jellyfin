// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const ssoManifestURL = "https://raw.githubusercontent.com/9p4/jellyfin-plugin-sso/manifest-release/manifest.json"

func TestAccSSOPluginConfigurationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Install plugin and create a full OID config.
			{
				Config: testAccSSOPluginConfigurationResourceConfig(
					false, // disable_pushed_authorization
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_sso_plugin_configuration.test", "plugin_id", ssoPluginID),
					resource.TestCheckResourceAttr("jellyfin_sso_plugin_configuration.test", "id", ssoPluginID),
					resource.TestCheckResourceAttrSet("jellyfin_sso_plugin_configuration.test", "oid_configs.%"),
					resource.TestCheckResourceAttr("jellyfin_sso_plugin_configuration.test", "oid_configs.authentik.enabled", "true"),
					resource.TestCheckResourceAttr("jellyfin_sso_plugin_configuration.test", "oid_configs.authentik.roles.#", "2"),
				),
			},
			// Inject a server-managed CanonicalLink and assert an empty plan.
			{
				PreConfig: func() { testAccSSOInjectCanonicalLink(t) },
				Config:    testAccSSOPluginConfigurationResourceConfig(false),
				PlanOnly:  true,
			},
			// Update a field and verify the injected link survived.
			{
				Config: testAccSSOPluginConfigurationResourceConfig(
					true, // disable_pushed_authorization
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_sso_plugin_configuration.test", "oid_configs.authentik.disable_pushed_authorization", "true"),
					testAccSSOCanonicalLinkSurvived(t),
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_sso_plugin_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     ssoPluginID,
			},
			// Precheck: uninstalled plugin GUID should error clearly.
			{
				Config:      testAccSSOPluginConfigurationResourceUninstalledConfig(),
				ExpectError: testAccSSOUninstalledErrorRegex(),
			},
		},
	})
}

func testAccSSOPluginConfigurationResourceConfig(disablePushedAuthorization bool) string {
	return fmt.Sprintf(`
resource "jellyfin_plugin_repository" "sso_auth" {
  enabled = true
  name    = "SSO-Auth"
  url     = %q
}

resource "jellyfin_plugin" "sso_auth" {
  name           = "SSO-Auth"
  version        = %q
  repository_url = jellyfin_plugin_repository.sso_auth.url
}

resource "jellyfin_sso_plugin_configuration" "test" {
  plugin_id = jellyfin_plugin.sso_auth.id

  oid_configs = {
    authentik = {
      oid_endpoint                = "https://auth.example.com/application/o/jellyfin"
      oid_client_id               = "client-id"
      oid_secret                  = "client-secret"
      enabled                     = true
      enable_authorization        = true
      enable_all_folders          = true
      enabled_folders             = []
      admin_roles                 = ["admins"]
      roles                       = ["watchers", "admins"]
      enable_folder_roles         = false
      enable_live_tv_roles        = false
      enable_live_tv              = false
      enable_live_tv_management   = false
      live_tv_roles               = []
      live_tv_management_roles    = []
      folder_role_mapping         = []
      role_claim                  = "groups"
      oid_scopes                  = ["groups"]
      default_provider            = ""
      scheme_override             = "https"
      port_override               = 0
      new_path                    = true
      default_username_claim      = "preferred_username"
      avatar_url_format           = ""
      disable_https               = false
      disable_pushed_authorization = %t
      do_not_validate_endpoints   = false
      do_not_validate_issuer_name = false
      do_not_load_profile         = false
    }
  }

  saml_configs = {}
}
`, ssoManifestURL, supportedSSOPluginVersion(), disablePushedAuthorization)
}

func testAccSSOPluginConfigurationResourceUninstalledConfig() string {
	return `
resource "jellyfin_sso_plugin_configuration" "uninstalled" {
  plugin_id = "00000000-0000-0000-0000-000000000000"

  oid_configs = {}
  saml_configs = {}
}
`
}

func testAccSSOUninstalledErrorRegex() *regexp.Regexp {
	return regexp.MustCompile(`SSO-Auth plugin [0-9a-fA-F-]{36} is not installed on the server`)
}

func testAccSSOInjectCanonicalLink(t *testing.T) {
	t.Helper()
	c := testAccClient(t)
	ctx := t.Context()

	raw, err := c.GetPluginConfiguration(ctx, ssoPluginID)
	if err != nil {
		t.Fatalf("failed to read SSO config for injection: %v", err)
	}

	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("failed to parse SSO config for injection: %v", err)
	}

	var oid map[string]map[string]json.RawMessage
	if err := json.Unmarshal(cfg["OidConfigs"], &oid); err != nil {
		t.Fatalf("failed to parse OidConfigs for injection: %v", err)
	}

	authentik, ok := oid["authentik"]
	if !ok {
		t.Fatalf("authentik OID config not found")
	}

	authentik["CanonicalLinks"] = json.RawMessage(`{"ThePhaseless":"bc2a3d075f7b42bc82521b32e7ba18a1"}`)
	oid["authentik"] = authentik

	oidJSON, err := json.Marshal(oid)
	if err != nil {
		t.Fatalf("failed to marshal OidConfigs for injection: %v", err)
	}
	cfg["OidConfigs"] = json.RawMessage(oidJSON)

	payload, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal SSO config for injection: %v", err)
	}

	if err := c.UpdatePluginConfiguration(ctx, ssoPluginID, string(payload)); err != nil {
		t.Fatalf("failed to inject canonical link: %v", err)
	}
}

func testAccSSOCanonicalLinkSurvived(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		ctx := context.Background()

		raw, err := c.GetPluginConfiguration(ctx, ssoPluginID)
		if err != nil {
			return fmt.Errorf("failed to read SSO config after update: %w", err)
		}

		var cfg struct {
			OidConfigs map[string]struct {
				CanonicalLinks map[string]string `json:"CanonicalLinks"`
			} `json:"OidConfigs"`
		}
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return fmt.Errorf("failed to parse SSO config after update: %w", err)
		}

		if cfg.OidConfigs["authentik"].CanonicalLinks["ThePhaseless"] != "bc2a3d075f7b42bc82521b32e7ba18a1" {
			return fmt.Errorf("canonical link did not survive update")
		}
		return nil
	}
}
