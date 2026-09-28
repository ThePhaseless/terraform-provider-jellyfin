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
	const address = "jellyfin_plugin_repository.test"
	repositoryName := fmt.Sprintf("Terraform Provider Test Repo %s", t.Name())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPluginRepositoryResourceConfig(repositoryName, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "name", repositoryName),
					resource.TestCheckResourceAttr(address, "url", testAccPluginRepositoryURL),
					resource.TestCheckResourceAttr(address, "enabled", "true"),
				),
			},
			{
				ResourceName:                         address,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateId:                        repositoryName,
			},
			{
				Config: testAccPluginRepositoryResourceConfig(repositoryName, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "name", repositoryName),
					resource.TestCheckResourceAttr(address, "enabled", "false"),
				),
			},
		},
	})
}

func testAccPluginRepositoryResourceConfig(repositoryName string, enabled bool) string {
	return fmt.Sprintf(`
resource "jellyfin_plugin_repository" "test" {
  name    = %q
  url     = %q
  enabled = %t
}
`, repositoryName, testAccPluginRepositoryURL, enabled)
}
