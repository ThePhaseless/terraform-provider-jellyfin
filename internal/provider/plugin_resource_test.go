// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

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
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_plugin.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccFindInstallablePlugin temporarily registers the given repository, queries
// available packages, and returns the name and version of the first package that is
// not already installed. The repository is restored to its original state after the test.
func testAccFindInstallablePlugin(t *testing.T, repoURL string) (name, version string) {
	t.Helper()

	testAccPreCheck(t)
	c := testAccClient(t)
	ctx := t.Context()

	// Get currently registered repos.
	repos, err := c.GetPluginRepositories(ctx)
	if err != nil {
		t.Fatalf("failed to get plugin repositories: %v", err)
	}

	// Register the stable repo temporarily if it's not already there.
	repoAlreadyRegistered := false
	for _, r := range repos {
		if r.URL == repoURL {
			repoAlreadyRegistered = true
			break
		}
	}

	if !repoAlreadyRegistered {
		tempRepo := client.PluginRepository{Name: "jellyfin-stable-temp", URL: repoURL, Enabled: true}
		if err := c.SetPluginRepositories(ctx, append(repos, tempRepo)); err != nil {
			t.Fatalf("failed to register stable repository for package listing: %v", err)
		}
		t.Cleanup(func() {
			if err := c.SetPluginRepositories(ctx, repos); err != nil {
				t.Errorf("failed to restore plugin repositories: %v", err)
			}
		})
	}

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
		if !installedNames[pkg.Name] && len(pkg.Versions) > 0 {
			return pkg.Name, pkg.Versions[0].Version
		}
	}

	t.Skip("no installable packages found (all packages already installed)")
	return "", ""
}
