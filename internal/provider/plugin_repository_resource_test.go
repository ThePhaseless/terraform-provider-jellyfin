// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const testAccPluginRepositoryURL = "https://example.com/terraform-provider-jellyfin/plugin/manifest.json"

func TestAccPluginRepositoryResource(t *testing.T) {
	repositoryName := fmt.Sprintf("Terraform Provider Test Repo %s", t.Name())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read.
			{
				Config: testAccPluginRepositoryResourceConfig(repositoryName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_plugin_repository.test", "name", repositoryName),
					resource.TestCheckResourceAttr("jellyfin_plugin_repository.test", "url", testAccPluginRepositoryURL),
					resource.TestCheckResourceAttr("jellyfin_plugin_repository.test", "enabled", "true"),
				),
			},
			// ImportState.
			{
				ResourceName:                         "jellyfin_plugin_repository.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateId:                        repositoryName,
			},
			// Update.
			{
				Config: testAccPluginRepositoryResourceConfigUpdated(repositoryName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_plugin_repository.test", "name", repositoryName),
					resource.TestCheckResourceAttr("jellyfin_plugin_repository.test", "enabled", "false"),
				),
			},
		},
	})
}

func testAccPluginRepositoryResourceConfig(repositoryName string) string {
	return fmt.Sprintf(`
resource "jellyfin_plugin_repository" "test" {
  name    = %q
  url     = %q
  enabled = true
}
`, repositoryName, testAccPluginRepositoryURL)
}

func testAccPluginRepositoryResourceConfigUpdated(repositoryName string) string {
	return fmt.Sprintf(`
resource "jellyfin_plugin_repository" "test" {
  name    = %q
  url     = %q
  enabled = false
}
`, repositoryName, testAccPluginRepositoryURL)
}
