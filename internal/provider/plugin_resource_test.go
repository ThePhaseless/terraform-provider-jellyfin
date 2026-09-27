// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

const stableRepoURL = "https://repo.jellyfin.org/files/plugin/manifest.json"

func TestAccPluginResource(t *testing.T) {
	pluginName, pluginVersion := testAccFindInstallablePlugin(t, stableRepoURL)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Install plugin.
			{
				Config: fmt.Sprintf(`
resource "jellyfin_plugin" "test" {
  name           = %q
  version        = %q
  repository_url = %q
}
`, pluginName, pluginVersion, stableRepoURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_plugin.test", "id"),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "name", pluginName),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "version", pluginVersion),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "installed_version", pluginVersion),
				),
			},
			// ImportState by the resource's own ID (round-trip verification).
			{
				ResourceName:      "jellyfin_plugin.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import by plugin *name* — the scenario from issue #84: the user
			// runs `terraform import jellyfin_plugin.x "SSO-Auth"` using the
			// plugin name, not the server-assigned UUID. ImportStateId overrides
			// the default behaviour (which imports using the state ID) so we can
			// pass the plugin name explicitly.
			{
				ResourceName:            "jellyfin_plugin.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"repository_url"},
				ImportStateId:           pluginName,
			},
			// GET /Plugins lists the dash-free GUID, but a GUID copied from
			// elsewhere usually has dashes.
			{
				ResourceName:      "jellyfin_plugin.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					id := s.RootModule().Resources["jellyfin_plugin.test"].Primary.ID
					if len(id) != 32 {
						return "", fmt.Errorf("plugin id %q is not a dash-free GUID", id)
					}
					return id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:], nil
				},
			},
			// Verify Create is idempotent when the plugin is already installed
			// (issue #84): adding a second resource for the same plugin name
			// must not 404. Create detects the plugin is already present and
			// reuses the existing install instead of POSTing again.
			{
				Config: fmt.Sprintf(`
resource "jellyfin_plugin" "test" {
  name           = %q
  version        = %q
  repository_url = %q
}

resource "jellyfin_plugin" "duplicate" {
  name           = %q
  version        = %q
  repository_url = %q
}
`, pluginName, pluginVersion, stableRepoURL,
					pluginName, pluginVersion, stableRepoURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_plugin.duplicate", "id"),
					resource.TestCheckResourceAttr("jellyfin_plugin.duplicate", "name", pluginName),
				),
			},
		},
	})
}

func TestAccPluginResourceDestroyRemovesUpdatedVersion(t *testing.T) {
	pkg := testAccFindUninstalledPackage(t, stableRepoURL, 2)
	older, newer := pkg.Versions[1].Version, pkg.Versions[0].Version
	config := fmt.Sprintf(`
resource "jellyfin_plugin" "test" {
  name           = %q
  version        = %q
  repository_url = %q
}
`, pkg.Name, older, stableRepoURL)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPluginNotListed(t, pkg.Name),
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				// Installing the newer version is what Jellyfin's plugin update
				// task does: it lands next to the older one, which Jellyfin then
				// lists as superseded until the next restart.
				PreConfig: func() {
					// A client from before the step is signed out by the
					// provider's own sign-in, which shares its device ID.
					c := testAccClient(t)
					if err := c.InstallPlugin(t.Context(), pkg.Name, newer, stableRepoURL); err != nil {
						t.Fatalf("installing %s %s: %v", pkg.Name, newer, err)
					}
					installer := &PluginResource{client: c}
					if _, err := installer.waitForPlugin(t.Context(), pkg.Name, newer, pluginInstallTimeout); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "version", older),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "installed_version", older),
				),
			},
		},
	})
}

func TestAccPluginResourceLatestVersion(t *testing.T) {
	pluginName, latest := testAccFindInstallablePlugin(t, stableRepoURL)
	config := func(version string) string {
		return fmt.Sprintf(`
resource "jellyfin_plugin" "test" {
  name           = %q
  version        = %q
  repository_url = %q
}
`, pluginName, version, stableRepoURL)
	}
	inPlace := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{
			plancheck.ExpectResourceAction("jellyfin_plugin.test", plancheck.ResourceActionUpdate),
		},
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("latest"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "version", "latest"),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "installed_version", latest),
				),
			},
			// An import has no keyword to keep, so version holds the installed
			// version, and the keyword in the configuration, which resolves to
			// that version, only needs to be written to state.
			{
				ResourceName:       "jellyfin_plugin.test",
				ImportState:        true,
				ImportStateKind:    resource.ImportBlockWithID,
				ImportStateId:      pluginName,
				ExpectNonEmptyPlan: true,
				ImportPlanChecks: resource.ImportPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("jellyfin_plugin.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue("jellyfin_plugin.test", tfjsonpath.New("version"), knownvalue.StringExact("latest")),
					},
				},
			},
			{
				ResourceName:            "jellyfin_plugin.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"version"},
			},
			// Naming the installed version in place of the keyword, or the
			// other way round, leaves the plugin installed.
			{
				Config:           config(latest),
				ConfigPlanChecks: inPlace,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "version", latest),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "installed_version", latest),
				),
			},
			{
				Config:           config("latest"),
				ConfigPlanChecks: inPlace,
				Check:            resource.TestCheckResourceAttr("jellyfin_plugin.test", "version", "latest"),
			},
		},
	})
}

// Jellyfin answers 204 to the DELETE of a plugin it bundles without removing
// it, and cmd/jellyfin-import writes a jellyfin_plugin for each of them.
func TestAccPluginResourceDestroyLeavesBundledPlugin(t *testing.T) {
	const name = "TMDb"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPluginListed(t, name),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
import {
  to = jellyfin_plugin.test
  id = %[1]q
}

resource "jellyfin_plugin" "test" {
  name = %[1]q
}
`, name),
				Check: resource.TestCheckResourceAttr("jellyfin_plugin.test", "name", name),
			},
		},
	})
}

// TestAccPluginResourcePinSurvivesRestartWithoutUpdateTriggers follows the
// resource's documented way to keep a pinned version: Jellyfin's plugin update
// task runs at startup, so it only stays pinned across a restart with the
// task's triggers removed.
func TestAccPluginResourcePinSurvivesRestartWithoutUpdateTriggers(t *testing.T) {
	if os.Getenv("JELLYFIN_RESTART_ACC") == "" {
		t.Skip("set JELLYFIN_RESTART_ACC=1 to run tests that restart the server; run against a disposable Jellyfin (e.g. the bundled docker-compose) in isolation, not a shared instance")
	}
	pkg := testAccFindUninstalledPackage(t, stableRepoURL, 2)
	pinned := pkg.Versions[1].Version
	taskID := testAccRestorePluginUpdateTriggers(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "jellyfin_scheduled_task" "plugin_updates" {
  task_id  = %q
  triggers = []
}

resource "jellyfin_plugin" "test" {
  name           = %q
  version        = %q
  repository_url = %q

  depends_on = [jellyfin_scheduled_task.plugin_updates]
}

resource "jellyfin_restart" "test" {
  triggers = {
    plugin_version = jellyfin_plugin.test.installed_version
  }
}
`, taskID, pkg.Name, pinned, stableRepoURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "installed_version", pinned),
					testAccCheckPluginStaysAt(t, pkg.Name, pinned),
				),
			},
		},
	})
}

// testAccRestorePluginUpdateTriggers gives Jellyfin's PluginUpdates task its
// default triggers, so that the test's own configuration is what removes
// them even when an earlier test, such as one that installs JellyfinSecurity,
// already did, and puts back the triggers it found afterwards. It returns the
// task's ID.
func testAccRestorePluginUpdateTriggers(t *testing.T) string {
	t.Helper()

	c := testAccClient(t)
	tasks, err := c.GetScheduledTasks(t.Context())
	if err != nil {
		t.Fatalf("listing scheduled tasks: %v", err)
	}
	for _, task := range tasks {
		if task.Key != "PluginUpdates" {
			continue
		}
		found := []byte("[]")
		if task.Triggers != nil {
			if found, err = json.Marshal(task.Triggers); err != nil {
				t.Fatalf("encoding the PluginUpdates triggers: %v", err)
			}
		}
		const defaults = `[{"Type":"StartupTrigger"},{"Type":"IntervalTrigger","IntervalTicks":864000000000}]`
		if err := c.UpdateScheduledTaskTriggers(t.Context(), task.ID, defaults); err != nil {
			t.Fatalf("restoring the PluginUpdates triggers: %v", err)
		}
		t.Cleanup(func() {
			// The provider's sign-in has signed c out by now, since both share
			// a device ID.
			if err := testAccClient(t).UpdateScheduledTaskTriggers(context.Background(), task.ID, string(found)); err != nil {
				t.Errorf("putting back the PluginUpdates triggers: %v", err)
			}
		})
		return task.ID
	}
	t.Fatal("scheduled task PluginUpdates not found")
	return ""
}

// testAccCheckPluginStaysAt fails if Jellyfin lists the named plugin at any
// version other than want during the time its startup tasks take to run.
func testAccCheckPluginStaysAt(t *testing.T, name, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		c := testAccClient(t)
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(pluginPollInterval) {
			plugins, err := c.GetInstalledPlugins(context.Background())
			if err != nil {
				return err
			}
			for _, p := range plugins {
				if p.Name == name && p.Status != pluginStatusDeleted && p.Version != want {
					return fmt.Errorf("%s is listed at %s (%s) next to the pinned %s", name, p.Version, p.Status, want)
				}
			}
		}
		return nil
	}
}

// testAccCheckPluginNotListed fails while Jellyfin lists any version of the
// named plugin other than one it deletes at the next restart.
func testAccCheckPluginNotListed(t *testing.T, name string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		plugins, err := testAccClient(t).GetInstalledPlugins(context.Background())
		if err != nil {
			return err
		}
		for _, p := range plugins {
			if p.Name == name && p.Status != pluginStatusDeleted {
				return fmt.Errorf("plugin %s is still listed at version %s with status %s", name, p.Version, p.Status)
			}
		}
		return nil
	}
}

// testAccCheckPluginListed fails unless Jellyfin lists the named plugin.
func testAccCheckPluginListed(t *testing.T, name string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		plugins, err := testAccClient(t).GetInstalledPlugins(context.Background())
		if err != nil {
			return err
		}
		for _, p := range plugins {
			if p.Name == name && p.Status != pluginStatusDeleted {
				return nil
			}
		}
		return fmt.Errorf("plugin %s is no longer listed", name)
	}
}

// testAccRegisterRepository registers the plugin repository for the rest of the
// test unless it already is, and puts the previous list back afterwards.
func testAccRegisterRepository(t *testing.T, name, repoURL string) {
	t.Helper()

	c := testAccClient(t)
	repos, err := c.GetPluginRepositories(t.Context())
	if err != nil {
		t.Fatalf("failed to get plugin repositories: %v", err)
	}
	for _, r := range repos {
		if r.URL == repoURL {
			return
		}
	}

	if err := c.SetPluginRepositories(t.Context(), append(repos, client.PluginRepository{Name: name, URL: repoURL, Enabled: true})); err != nil {
		t.Fatalf("failed to register repository %s: %v", repoURL, err)
	}
	t.Cleanup(func() {
		// t.Context() is done by now, and the provider's sign-in has signed c
		// out, since both share a device ID.
		if err := testAccClient(t).SetPluginRepositories(context.Background(), repos); err != nil {
			t.Errorf("failed to restore plugin repositories: %v", err)
		}
	})
}

// testAccFindInstallablePlugin temporarily registers the given repository, queries
// available packages, and returns the name and version of the first package that is
// not already installed. The repository is restored to its original state after the test.
func testAccFindInstallablePlugin(t *testing.T, repoURL string) (name, version string) {
	t.Helper()

	pkg := testAccFindUninstalledPackage(t, repoURL, 1)
	return pkg.Name, pkg.Versions[0].Version
}

// testAccFindUninstalledPackage is testAccFindInstallablePlugin for a package
// that offers at least minVersions versions, newest first.
func testAccFindUninstalledPackage(t *testing.T, repoURL string, minVersions int) client.PackageInfo {
	t.Helper()

	testAccPreCheck(t)
	testAccRegisterRepository(t, "jellyfin-stable-temp", repoURL)
	c := testAccClient(t)
	ctx := t.Context()

	// Query available packages.
	pkgs, err := c.GetAvailablePackages(ctx)
	if err != nil {
		t.Skipf("failed to list packages (repository may be unavailable): %v", err)
	}
	if len(pkgs) == 0 {
		t.Skip("no packages available in the stable repository")
	}

	// Get currently installed plugins to avoid picking one that's already installed.
	installed, err := c.GetInstalledPlugins(ctx)
	if err != nil {
		t.Fatalf("failed to get installed plugins: %v", err)
	}
	installedNames := make(map[string]bool, len(installed))
	for _, p := range installed {
		installedNames[p.Name] = true
	}

	// Return the first available package that is not already installed.
	for _, pkg := range pkgs {
		if !installedNames[pkg.Name] && len(pkg.Versions) >= minVersions {
			return pkg
		}
	}

	t.Skip("no installable packages found (all packages already installed)")
	return client.PackageInfo{}
}
