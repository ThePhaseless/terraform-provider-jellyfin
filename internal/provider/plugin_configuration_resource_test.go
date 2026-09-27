// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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
				ResourceName:                         "jellyfin_plugin_configuration.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "plugin_id",
				ImportStateId:                        "8c95c4d2e50c4fb0a4f36c06ff0f9a1a",
				ImportStateVerifyIgnore:              []string{"configuration_json"},
			},
			// Importing by the dashed spelling of the GUID the configuration
			// holds dash-free must not plan a replacement.
			{
				ResourceName:    "jellyfin_plugin_configuration.test",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				ImportStateId:   "8c95c4d2-e50c-4fb0-a4f3-6c06ff0f9a1a",
			},
			// Nor does switching the configuration to that spelling.
			{
				Config: `
resource "jellyfin_plugin_configuration" "test" {
  plugin_id          = "8c95c4d2-e50c-4fb0-a4f3-6c06ff0f9a1a"
  configuration_json = jsonencode({
    Server            = "https://musicbrainz.org"
    RateLimit         = 1
    ReplaceArtistName = false
  })
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: resource.TestCheckResourceAttr("jellyfin_plugin_configuration.test", "plugin_id", "8c95c4d2e50c4fb0a4f36c06ff0f9a1a"),
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
