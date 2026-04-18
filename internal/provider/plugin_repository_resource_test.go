// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPluginRepositoryResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read.
			{
				Config: testAccPluginRepositoryResourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_plugin_repository.test", "name", "Jellyfin Stable"),
					resource.TestCheckResourceAttr("jellyfin_plugin_repository.test", "url", "https://repo.jellyfin.org/files/plugin/manifest.json"),
					resource.TestCheckResourceAttr("jellyfin_plugin_repository.test", "enabled", "true"),
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_plugin_repository.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "Jellyfin Stable",
			},
			// Update.
			{
				Config: testAccPluginRepositoryResourceConfigUpdated(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_plugin_repository.test", "name", "Jellyfin Stable"),
					resource.TestCheckResourceAttr("jellyfin_plugin_repository.test", "enabled", "false"),
				),
			},
		},
	})
}

func testAccPluginRepositoryResourceConfig() string {
	return `
resource "jellyfin_plugin_repository" "test" {
  name    = "Jellyfin Stable"
  url     = "https://repo.jellyfin.org/files/plugin/manifest.json"
  enabled = true
}
`
}

func testAccPluginRepositoryResourceConfigUpdated() string {
	return `
resource "jellyfin_plugin_repository" "test" {
  name    = "Jellyfin Stable"
  url     = "https://repo.jellyfin.org/files/plugin/manifest.json"
  enabled = false
}
`
}
