// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSystemConfigurationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read.
			{
				Config: testAccSystemConfigurationResourceConfig("TestServer"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "server_name", "TestServer"),
					resource.TestCheckResourceAttrSet("jellyfin_system_configuration.test", "configuration_json"),
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_system_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update.
			{
				Config: testAccSystemConfigurationResourceConfig("UpdatedServer"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "server_name", "UpdatedServer"),
				),
			},
		},
	})
}

func testAccSystemConfigurationResourceConfig(serverName string) string {
	return `
resource "jellyfin_system_configuration" "test" {
  server_name = "` + serverName + `"
}
`
}
