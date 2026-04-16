// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPluginConfigurationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create: configure MusicBrainz plugin (built-in, always available).
			{
				Config: `
resource "jellyfin_plugin_configuration" "test" {
  plugin_id          = "8c95c4d2e50c4fb0a4f36c06ff0f9a1a"
  configuration_json = jsonencode({
    Server            = "https://musicbrainz.org"
    RateLimit         = 1
    ReplaceArtistName = false
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_plugin_configuration.test", "plugin_id", "8c95c4d2e50c4fb0a4f36c06ff0f9a1a"),
					resource.TestCheckResourceAttrSet("jellyfin_plugin_configuration.test", "configuration_json"),
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_plugin_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update: change rate limit.
			{
				Config: `
resource "jellyfin_plugin_configuration" "test" {
  plugin_id          = "8c95c4d2e50c4fb0a4f36c06ff0f9a1a"
  configuration_json = jsonencode({
    Server            = "https://musicbrainz.org"
    RateLimit         = 2
    ReplaceArtistName = false
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_plugin_configuration.test", "configuration_json"),
				),
			},
		},
	})
}
