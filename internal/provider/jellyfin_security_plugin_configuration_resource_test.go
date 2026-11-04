// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

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
	updatedValues := securityPluginTestValues{
		pluginID:           jellyfinSecurityPluginID,
		publicBaseURL:      "https://media.example.com/jellyfin",
		pairDevice:         false,
		stepUpWindow:       300,
		enrollmentDeadline: "2031-06-01T12:00:00+02:00",
		displayName:        "Renamed IdP",
		linkByUsername:     false,
	}
	updated := testAccSecurityPluginConfigurationConfig(updatedValues)
	cleared, notBumped := updatedValues, updatedValues
	cleared.smtpPasswordVersion = 2
	notBumped.smtpPassword, notBumped.smtpPasswordVersion = "p2", 2

	// An import cannot read the versions back, and the write-only values are
	// never in state.
	importIgnore := []string{"enrollment_deadline", "webhook_headers_wo_version", "oidc_client_secrets_wo_version"}
	for _, s := range securityPluginSecrets {
		importIgnore = append(importIgnore, s+"_wo_version")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
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
					resource.TestCheckResourceAttr(name, "user_emails.0.email", "admin@example.com"),
					resource.TestCheckResourceAttr(name, "oidc_providers.#", "1"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.display_name", "Terraform IdP"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.link_existing_users_by_username", "true"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.scopes.#", "2"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.role_library_mappings.0.library_ids.1", "b"),
					resource.TestCheckResourceAttrSet(name, "oidc_providers.0.created_at"),
					// The plugin serves each saved secret masked, and the
					// write-only values stay out of state.
					resource.TestCheckResourceAttr(name, "smtp_password", maskedMarker),
					resource.TestCheckResourceAttr(name, "webhook_headers.0", "X-Api-Key: "+maskedMarker),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.client_secret", maskedMarker),
					resource.TestCheckResourceAttr(name, "smtp_password_wo_version", "1"),
					resource.TestCheckResourceAttr(name, "oidc_client_secrets_wo_version", "1"),
					resource.TestCheckNoResourceAttr(name, "smtp_password_wo"),
					resource.TestCheckNoResourceAttr(name, "webhook_headers_wo.%"),
					resource.TestCheckNoResourceAttr(name, "oidc_client_secrets_wo.%"),
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
				ImportStateVerifyIgnore: importIgnore,
			},
			{
				Config: updated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate),
						// updated spells the GUID with dashes, which names the
						// same plugin as the spelling in state.
						plancheck.ExpectKnownValue(name, tfjsonpath.New("plugin_id"), knownvalue.StringExact(dashFreeID)),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", dashFreeID),
					resource.TestCheckResourceAttr(name, "public_base_url", "https://media.example.com/jellyfin"),
					resource.TestCheckResourceAttr(name, "pair_device_on_second_screen_approval", "false"),
					resource.TestCheckResourceAttr(name, "step_up_window_seconds", "300"),
					resource.TestCheckResourceAttr(name, "enrollment_deadline", "2031-06-01T12:00:00+02:00"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.display_name", "Renamed IdP"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.link_existing_users_by_username", "false"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.client_secret", maskedMarker),
				),
			},
			// An empty write-only value at a new version clears the secret.
			{
				Config: testAccSecurityPluginConfigurationConfig(cleared),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "smtp_password", ""),
					resource.TestCheckResourceAttr(name, "smtp_password_wo_version", "2"),
				),
			},
			// A new write-only value is not sent until its version changes.
			{
				Config: testAccSecurityPluginConfigurationConfig(notBumped),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			// The versions cannot be imported, so an import block always plans
			// them in, but it plans no change to the deadline the server
			// serves in its own layout.
			{
				ResourceName:       name,
				ImportState:        true,
				ImportStateKind:    resource.ImportBlockWithID,
				ImportStateId:      jellyfinSecurityPluginID,
				ExpectNonEmptyPlan: true,
				ImportPlanChecks: resource.ImportPlanChecks{
					PreApply: []plancheck.PlanCheck{attributeUnchanged{address: name, attribute: "enrollment_deadline"}},
				},
			},
		},
	})
}

// Moving from the deprecated secret arguments to the write-only ones takes the
// secrets out of state.
func TestAccSecurityPluginDeprecatedSecrets(t *testing.T) {
	testAccSecurityPluginPreCheck(t)
	testAccInstallSecurityPlugin(t)

	const name = "jellyfin_security_plugin_configuration.test"
	values := securityPluginTestValues{
		pluginID:           jellyfinSecurityPluginID,
		publicBaseURL:      "https://deprecated.example.com",
		pairDevice:         true,
		stepUpWindow:       600,
		enrollmentDeadline: "2033-01-01T00:00:00Z",
		displayName:        "Deprecated IdP",
		linkByUsername:     true,
		secrets:            deprecatedSecrets,
	}
	writeOnly := values
	writeOnly.secrets = writeOnlySecrets

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		Steps: []resource.TestStep{
			{
				Config: testAccSecurityPluginConfigurationConfig(values),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "smtp_password", "p1"),
					resource.TestCheckResourceAttr(name, "webhook_headers.0", "X-Api-Key: k"),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.client_secret", "s3cret"),
				),
			},
			{
				Config: testAccSecurityPluginConfigurationConfig(writeOnly),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "smtp_password", maskedMarker),
					resource.TestCheckResourceAttr(name, "webhook_headers.0", "X-Api-Key: "+maskedMarker),
					resource.TestCheckResourceAttr(name, "oidc_providers.0.client_secret", maskedMarker),
				),
			},
		},
	})
}

// attributeUnchanged checks that the planned change of address leaves
// attribute as it was.
type attributeUnchanged struct{ address, attribute string }

func (c attributeUnchanged) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	for _, rc := range req.Plan.ResourceChanges {
		if rc.Address != c.address || rc.Change == nil {
			continue
		}
		before, _ := rc.Change.Before.(map[string]any)
		after, _ := rc.Change.After.(map[string]any)
		if before[c.attribute] == nil || before[c.attribute] != after[c.attribute] {
			resp.Error = fmt.Errorf("%s: %s planned from %v to %v", c.address, c.attribute, before[c.attribute], after[c.attribute])
		}
		return
	}
	resp.Error = fmt.Errorf("%s: no planned change", c.address)
}

// Destroying jellyfin_plugin uninstalls JellyfinSecurity, so this shares the
// gate of the tests that install it.
func TestAccSecurityPluginSupportedVersionKeyword(t *testing.T) {
	testAccSecurityPluginPreCheck(t)
	testAccRegisterRepository(t, "JellyfinSecurity", securityPluginRepoURL)

	installer := &PluginResource{client: testAccClient(t)}
	build, err := installer.resolvePluginVersion(t.Context(), securityPluginName, types.StringValue(pluginVersionSupported), new(diag.Diagnostics))
	if err != nil {
		t.Fatalf("resolving the supported JellyfinSecurity build: %v", err)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "jellyfin_plugin" "test" {
  name           = %q
  version        = "supported"
  repository_url = %q
}
`, securityPluginName, securityPluginRepoURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "id", normalizeGUID(jellyfinSecurityPluginID)),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "version", "supported"),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "installed_version", build),
				),
			},
		},
	})
}

// secretMode picks how a security plugin test configuration sets its
// secrets.
type secretMode int

const (
	// writeOnlySecrets sets the write-only arguments and their versions.
	writeOnlySecrets secretMode = iota
	// deprecatedSecrets sets smtp_password, webhook_headers and the OIDC
	// provider's client_secret.
	deprecatedSecrets
	// noSecrets sets none, which provider 0.3.8 can apply against a plugin
	// that serves saved secrets masked.
	noSecrets
)

type securityPluginTestValues struct {
	pluginID           string
	publicBaseURL      string
	pairDevice         bool
	stepUpWindow       int
	enrollmentDeadline string
	displayName        string
	linkByUsername     bool
	secrets            secretMode
	// smtpPassword and smtpPasswordVersion set smtp_password_wo and its
	// version under writeOnlySecrets; a zero version stands for "p1" at 1.
	smtpPassword        string
	smtpPasswordVersion int
}

func testAccSecurityPluginConfigurationConfig(v securityPluginTestValues) string {
	var secrets, clientSecret string
	switch v.secrets {
	case writeOnlySecrets:
		smtp, version := v.smtpPassword, v.smtpPasswordVersion
		if version == 0 {
			smtp, version = "p1", 1
		}
		secrets = fmt.Sprintf(`
  smtp_password_wo               = %q
  smtp_password_wo_version       = %d
  webhook_headers_wo             = { "X-Api-Key" = "k" }
  webhook_headers_wo_version     = 1
  oidc_client_secrets_wo         = { "tf-acc" = "s3cret" }
  oidc_client_secrets_wo_version = 1
`, smtp, version)
	case deprecatedSecrets:
		secrets = `
  smtp_password   = "p1"
  webhook_headers = ["X-Api-Key: k"]
`
		clientSecret = `
    client_secret = "s3cret"`
	case noSecrets:
	}

	return fmt.Sprintf(`
resource "jellyfin_security_plugin_configuration" "test" {
  plugin_id = %[1]q

  public_base_url                       = %[2]q
  pair_device_on_second_screen_approval = %[3]t
  step_up_window_seconds                = %[4]d
  enrollment_deadline                   = %[5]q
  trusted_proxy_cidrs                   = ["10.0.0.0/8"]
%[8]s
  user_emails = [{
    user_id = "00000000000000000000000000000001"
    email   = "admin@example.com"
  }]

  oidc_providers = [{
    id                              = "tf-acc"
    display_name                    = %[6]q
    discovery_url                   = "https://idp.example.com/.well-known/openid-configuration"
    client_id                       = "jellyfin"%[9]s
    scopes                          = ["openid", "profile"]
    enabled                         = false
    link_existing_users_by_username = %[7]t
    role_library_mappings = [{
      role        = "kids"
      library_ids = ["a", "b"]
    }]
  }]
}
`, v.pluginID, v.publicBaseURL, v.pairDevice, v.stepUpWindow, v.enrollmentDeadline, v.displayName, v.linkByUsername, secrets, clientSecret)
}

func testAccSecurityPluginPreCheck(t *testing.T) {
	t.Helper()
	testAccRestartPreCheck(t, "tests that install the JellyfinSecurity plugin and restart the server")
}

// testAccInstallSecurityPlugin installs the supported JellyfinSecurity build
// and restarts the server to load it, unless that build is already active. It
// registers the JellyfinSecurity repository for the rest of the test and puts
// the previous repository list back when the test ends; the plugin stays
// installed and plugin updates stay disabled.
func testAccInstallSecurityPlugin(t *testing.T) *client.Client {
	t.Helper()

	testAccRegisterRepository(t, "JellyfinSecurity", securityPluginRepoURL)
	// testAccRegisterRepository signs in with its own client, and a sign-in
	// signs out every earlier client sharing its device ID, so c comes after.
	c := testAccClient(t)
	ctx := t.Context()

	installed, err := findSecurityPlugin(ctx, c)
	if err != nil {
		t.Fatalf("listing installed plugins: %v", err)
	}

	installer := &PluginResource{client: c}
	version, err := installer.resolvePluginVersion(ctx, securityPluginName, types.StringNull(), new(diag.Diagnostics))
	if err != nil {
		t.Fatalf("resolving the supported JellyfinSecurity build: %v", err)
	}
	if version == "" {
		t.Fatalf("the repositories Jellyfin reads do not offer %s", securityPluginName)
	}
	active := func(p *client.InstalledPlugin) bool {
		return p != nil && p.Status == "Active" && samePluginVersion(p.Version, version)
	}
	if active(installed) {
		return c
	}
	if installed == nil || !samePluginVersion(installed.Version, version) {
		if err := c.InstallPlugin(ctx, securityPluginName, version, securityPluginRepoURL); err != nil {
			t.Fatalf("installing JellyfinSecurity %s: %v", version, err)
		}
		if _, err := installer.waitForPlugin(ctx, securityPluginName, version, pluginInstallTimeout); err != nil {
			t.Fatalf("waiting for JellyfinSecurity %s: %v", version, err)
		}
	}

	// Jellyfin updates plugins at startup: once upstream offers a newer release,
	// it replaces the build under test at this or any later restart. These tests
	// only run against a disposable server, so the schedule is not put back.
	if err := disablePluginUpdates(ctx, c); err != nil {
		t.Fatalf("disabling plugin updates: %v", err)
	}
	if err := c.RestartServer(ctx); err != nil {
		t.Fatalf("restarting Jellyfin: %v", err)
	}
	if err := awaitRestart(ctx, c, 2*time.Minute, startupStatusDelay); err != nil {
		t.Fatalf("waiting for Jellyfin to restart: %v", err)
	}

	deadline := time.Now().Add(time.Minute)
	for {
		installed, err = findSecurityPlugin(ctx, c)
		if err == nil && active(installed) {
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

// testAccPutBackSecurityPluginConfiguration reads the JellyfinSecurity
// configuration through c and puts it back when the test ends, through a fresh
// client, since the provider's sign-in has signed c out by then.
func testAccPutBackSecurityPluginConfiguration(t *testing.T, c *client.Client) {
	t.Helper()

	original, err := c.GetPluginConfiguration(t.Context(), jellyfinSecurityPluginID)
	if err != nil {
		t.Fatalf("reading the JellyfinSecurity configuration: %v", err)
	}
	t.Cleanup(func() {
		if err := testAccClient(t).UpdatePluginConfiguration(context.WithoutCancel(t.Context()), jellyfinSecurityPluginID, original); err != nil {
			t.Errorf("putting back the JellyfinSecurity configuration: %v", err)
		}
	})
}

// testAccSecurityPluginPayloadShape reduces the payload the plugin serves for
// its defaults plus one entry in each list of objects and
// payloadListPlaceholder in every list still empty.
func testAccSecurityPluginPayloadShape(t *testing.T, c *client.Client) []string {
	t.Helper()

	testAccPutBackSecurityPluginConfiguration(t, c)
	ctx := t.Context()

	// Properties left out take the plugin's defaults, and a null one is not
	// served at all, so the deadline is set to make it part of the shape. A
	// list given an entry here is not filled with payloadListPlaceholder.
	probe := `{"UserEmails":[{}],"OidcProviders":[{"RoleLibraryMappings":[{}]}],"EnrollmentDeadline":"2030-01-01T00:00:00Z"}`
	if err := c.UpdatePluginConfiguration(ctx, jellyfinSecurityPluginID, probe); err != nil {
		t.Fatalf("writing the probe configuration: %v", err)
	}
	defaults, err := c.GetPluginConfiguration(ctx, jellyfinSecurityPluginID)
	if err != nil {
		t.Fatalf("reading the probe configuration back: %v", err)
	}

	filled, lists, err := fillEmptyPayloadLists(defaults)
	if err != nil {
		t.Fatalf("filling the empty lists: %v", err)
	}
	if err := c.UpdatePluginConfiguration(ctx, jellyfinSecurityPluginID, filled); err != nil {
		t.Fatalf("writing %q into each list the plugin serves empty (%s): %v\n\nOne of them no longer takes that entry: it holds objects or values a numeric string cannot be read as, or the plugin now checks its entries. The server log names the list, for a type mismatch as the Path of the JsonException:\n\n  docker compose --env-file internal/provider/supported_jellyfin_version.env logs jellyfin\n\nGive that list an entry the plugin accepts in the probe in testAccSecurityPluginPayloadShape.", payloadListPlaceholder, strings.Join(lists, ", "), err)
	}

	served, err := c.GetPluginConfiguration(ctx, jellyfinSecurityPluginID)
	if err != nil {
		t.Fatalf("reading the filled configuration back: %v", err)
	}
	lines, err := reduceSecurityPluginPayload(served)
	if err != nil {
		t.Fatalf("reducing the served configuration: %v", err)
	}

	var dropped []string
	for _, line := range lines {
		if list, ok := strings.CutSuffix(line, ": array"); ok && slices.Contains(lists, list) {
			dropped = append(dropped, list)
		}
	}
	if len(dropped) > 0 {
		t.Fatalf("the plugin serves %s empty after %q was written into each, so the shape cannot type them. It now drops entries it does not accept: give each an entry the plugin keeps in the probe in testAccSecurityPluginPayloadShape.", strings.Join(dropped, ", "), payloadListPlaceholder)
	}
	return lines
}
