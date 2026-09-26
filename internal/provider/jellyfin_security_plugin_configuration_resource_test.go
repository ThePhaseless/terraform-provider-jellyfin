// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

const (
	securityPluginName    = "Jellyfin Security"
	securityPluginRepoURL = "https://raw.githubusercontent.com/ZL154/JellyfinSecurity/main/manifest.json"
)

func TestAccSecurityPluginConfigurationResource(t *testing.T) {
	testAccSecurityPluginPreCheck(t)
	testAccInstallSecurityPlugin(t)

	const name = "jellyfin_security_plugin_configuration.test"
	// jellyfin_plugin's id holds the dash-free spelling the server lists.
	dashFreeID := normalizeGUID(jellyfinSecurityPluginID)
	created := testAccSecurityPluginConfigurationConfig(securityPluginTestValues{
		pluginID:           dashFreeID,
		publicBaseURL:      "https://jellyfin.example.com",
		pairDevice:         true,
		stepUpWindow:       600,
		enrollmentDeadline: "2030-01-01T00:00:00Z",
		displayName:        "Terraform IdP",
		linkByUsername:     true,
	})
	updated := testAccSecurityPluginConfigurationConfig(securityPluginTestValues{
		pluginID:           jellyfinSecurityPluginID,
		publicBaseURL:      "https://media.example.com/jellyfin",
		pairDevice:         false,
		stepUpWindow:       300,
		enrollmentDeadline: "2031-06-01T12:00:00+02:00",
		displayName:        "Renamed IdP",
		linkByUsername:     false,
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", dashFreeID),
					resource.TestCheckResourceAttr(name, "enabled", "true"),
					resource.TestCheckResourceAttr(name, "public_base_url", "https://jellyfin.example.com"),
					resource.TestCheckResourceAttr(name, "pair_device_on_second_screen_approval", "true"),
					resource.TestCheckResourceAttr(name, "step_up_window_seconds", "600"),
					resource.TestCheckResourceAttr(name, "enrollment_deadline", "2030-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr(name, "trusted_proxy_cidrs.#", "1"),
					resource.TestCheckResourceAttr(name, "webhook_headers.0", "X-Api-Key: k"),
					resource.TestCheckResourceAttr(name, "user_emails.0.email", "admin@example.com"),
					resource.TestCheckResourceAttr(name, "oidc_providers.#", "1"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.display_name", "Terraform IdP"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.link_existing_users_by_username", "true"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.scopes.#", "2"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.role_library_mappings.0.library_ids.1", "b"),
					resource.TestCheckResourceAttrSet(name, "oidc_providers.0.created_at"),
				),
			},
			{
				ResourceName:      name,
				ImportState:       true,
				ImportStateId:     dashFreeID,
				ImportStateVerify: true,
				// An import has no configured spelling to keep, so it holds the
				// server's .NET layout of the same instant; the import block step
				// below shows that this does not plan a change.
				ImportStateVerifyIgnore: []string{"enrollment_deadline"},
			},
			{
				Config: updated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", jellyfinSecurityPluginID),
					resource.TestCheckResourceAttr(name, "public_base_url", "https://media.example.com/jellyfin"),
					resource.TestCheckResourceAttr(name, "pair_device_on_second_screen_approval", "false"),
					resource.TestCheckResourceAttr(name, "step_up_window_seconds", "300"),
					resource.TestCheckResourceAttr(name, "enrollment_deadline", "2031-06-01T12:00:00+02:00"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.display_name", "Renamed IdP"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.link_existing_users_by_username", "false"),
				),
			},
			{
				ResourceName:    name,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				ImportStateId:   jellyfinSecurityPluginID,
			},
		},
	})
}

type securityPluginTestValues struct {
	pluginID           string
	publicBaseURL      string
	pairDevice         bool
	stepUpWindow       int
	enrollmentDeadline string
	displayName        string
	linkByUsername     bool
}

func testAccSecurityPluginConfigurationConfig(v securityPluginTestValues) string {
	return fmt.Sprintf(`
resource "jellyfin_security_plugin_configuration" "test" {
  plugin_id = %[1]q

  public_base_url                       = %[2]q
  pair_device_on_second_screen_approval = %[3]t
  step_up_window_seconds                = %[4]d
  enrollment_deadline                   = %[5]q
  trusted_proxy_cidrs                   = ["10.0.0.0/8"]
  webhook_headers                       = ["X-Api-Key: k"]

  user_emails = [{
    user_id = "00000000000000000000000000000001"
    email   = "admin@example.com"
  }]

  oidc_providers = [{
    id                              = "tf-acc"
    display_name                    = %[6]q
    discovery_url                   = "https://idp.example.com/.well-known/openid-configuration"
    client_id                       = "jellyfin"
    client_secret                   = "s3cret"
    scopes                          = ["openid", "profile"]
    enabled                         = false
    link_existing_users_by_username = %[7]t
    role_library_mappings = [{
      role        = "kids"
      library_ids = ["a", "b"]
    }]
  }]
}
`, v.pluginID, v.publicBaseURL, v.pairDevice, v.stepUpWindow, v.enrollmentDeadline, v.displayName, v.linkByUsername)
}

// testAccSecurityPluginPreCheck gates the tests that install JellyfinSecurity:
// loading the plugin takes a server restart, which disrupts any other test
// sharing the instance.
func testAccSecurityPluginPreCheck(t *testing.T) {
	t.Helper()

	if os.Getenv("JELLYFIN_RESTART_ACC") == "" {
		t.Skip("set JELLYFIN_RESTART_ACC=1 to run tests that install the JellyfinSecurity plugin and restart the server; run against a disposable Jellyfin (e.g. the bundled docker-compose) in isolation, not a shared instance")
	}
	testAccPreCheck(t)
}

// testAccInstallSecurityPlugin installs the supported JellyfinSecurity build
// this server accepts and restarts the server so it loads, the way
// jellyfin_plugin and jellyfin_restart do. It returns at once when the plugin
// is already active.
func testAccInstallSecurityPlugin(t *testing.T) *client.Client {
	t.Helper()

	c := testAccClient(t)
	ctx := t.Context()

	installed, err := findSecurityPlugin(ctx, c)
	if err != nil {
		t.Fatalf("listing installed plugins: %v", err)
	}
	if installed != nil && installed.Status == "Active" {
		return c
	}

	repos, err := c.GetPluginRepositories(ctx)
	if err != nil {
		t.Fatalf("listing plugin repositories: %v", err)
	}
	registered := false
	for _, r := range repos {
		if r.URL == securityPluginRepoURL {
			registered = true
			break
		}
	}
	if !registered {
		repos = append(repos, client.PluginRepository{Name: "JellyfinSecurity", URL: securityPluginRepoURL, Enabled: true})
		if err := c.SetPluginRepositories(ctx, repos); err != nil {
			t.Fatalf("registering the JellyfinSecurity repository: %v", err)
		}
	}

	installer := &PluginResource{client: c}
	version, err := installer.resolvePluginVersion(ctx, securityPluginName, types.StringNull())
	if err != nil {
		t.Fatalf("resolving the supported JellyfinSecurity build: %v", err)
	}
	if installed == nil || !samePluginVersion(installed.Version, version) {
		if err := c.InstallPlugin(ctx, securityPluginName, version, securityPluginRepoURL); err != nil {
			t.Fatalf("installing JellyfinSecurity %s: %v", version, err)
		}
		if _, err := installer.waitForPlugin(ctx, securityPluginName, version, pluginInstallTimeout); err != nil {
			t.Fatalf("waiting for JellyfinSecurity %s: %v", version, err)
		}
	}

	// Jellyfin updates plugins at startup, which would load a newer release in
	// place of the build under test at this or any later restart. These tests
	// only run against a disposable server, so the schedule is not put back.
	if err := disablePluginUpdates(ctx, c); err != nil {
		t.Fatalf("disabling plugin updates: %v", err)
	}
	if err := c.RestartServer(ctx); err != nil {
		t.Fatalf("restarting Jellyfin: %v", err)
	}
	if err := waitForServerReady(ctx, c, 2*time.Minute); err != nil {
		t.Fatalf("waiting for Jellyfin to restart: %v", err)
	}

	deadline := time.Now().Add(time.Minute)
	for {
		installed, err = findSecurityPlugin(ctx, c)
		if err == nil && installed != nil && installed.Status == "Active" && samePluginVersion(installed.Version, version) {
			return c
		}
		if time.Now().After(deadline) {
			t.Fatalf("JellyfinSecurity %s is not active after the restart: plugin %+v, error %v", version, installed, err)
		}
		time.Sleep(pluginPollInterval)
	}
}

func disablePluginUpdates(ctx context.Context, c *client.Client) error {
	tasks, err := c.GetScheduledTasks(ctx)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.Key == "PluginUpdates" {
			return c.UpdateScheduledTaskTriggers(ctx, task.ID, "[]")
		}
	}
	return fmt.Errorf("scheduled task PluginUpdates not found")
}

// findSecurityPlugin returns the installed JellyfinSecurity entry, preferring
// the active one while a superseded version is still listed.
func findSecurityPlugin(ctx context.Context, c *client.Client) (*client.InstalledPlugin, error) {
	plugins, err := c.GetInstalledPlugins(ctx)
	if err != nil {
		return nil, err
	}

	var found *client.InstalledPlugin
	for i := range plugins {
		if normalizeGUID(plugins[i].ID) != normalizeGUID(jellyfinSecurityPluginID) {
			continue
		}
		if plugins[i].Status == "Active" {
			return &plugins[i], nil
		}
		if found == nil {
			found = &plugins[i]
		}
	}
	return found, nil
}

// testAccSecurityPluginPayloadShape reduces the payload the plugin serves for
// its defaults plus one entry in each list of objects, so that the nested
// models' keys are served too. It puts back the configuration it found.
func testAccSecurityPluginPayloadShape(t *testing.T, c *client.Client) []string {
	t.Helper()

	ctx := t.Context()
	original, err := c.GetPluginConfiguration(ctx, jellyfinSecurityPluginID)
	if err != nil {
		t.Fatalf("reading the JellyfinSecurity configuration: %v", err)
	}
	t.Cleanup(func() {
		if err := c.UpdatePluginConfiguration(context.Background(), jellyfinSecurityPluginID, original); err != nil {
			t.Errorf("restoring the JellyfinSecurity configuration: %v", err)
		}
	})

	// Properties left out take the plugin's defaults, and a null one is not
	// served at all, so the deadline is set to make it part of the shape.
	probe := `{"UserEmails":[{}],"OidcProviders":[{"RoleLibraryMappings":[{}]}],"EnrollmentDeadline":"2030-01-01T00:00:00Z"}`
	if err := c.UpdatePluginConfiguration(ctx, jellyfinSecurityPluginID, probe); err != nil {
		t.Fatalf("writing the probe configuration: %v", err)
	}

	served, err := c.GetPluginConfiguration(ctx, jellyfinSecurityPluginID)
	if err != nil {
		t.Fatalf("reading the probe configuration back: %v", err)
	}
	lines, err := reduceSecurityPluginPayload(served)
	if err != nil {
		t.Fatalf("reducing the served configuration: %v", err)
	}
	return lines
}
