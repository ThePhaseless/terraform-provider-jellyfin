// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestAccPluginConfigurationResource(t *testing.T) {
	const (
		name = "jellyfin_plugin_configuration.test"
		// MusicBrainz is built in, so always available.
		pluginID = "8c95c4d2e50c4fb0a4f36c06ff0f9a1a"
		dashedID = "8c95c4d2-e50c-4fb0-a4f3-6c06ff0f9a1a"
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMusicBrainzConfigurationConfig(pluginID, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "plugin_id", pluginID),
					resource.TestCheckResourceAttrSet(name, "configuration_json"),
				),
			},
			{
				ResourceName:                         name,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "plugin_id",
				ImportStateId:                        pluginID,
				ImportStateVerifyIgnore:              []string{"configuration_json"},
			},
			// Importing by the dashed spelling of the GUID the configuration
			// holds dash-free must not plan a replacement.
			{
				ResourceName:    name,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				ImportStateId:   dashedID,
			},
			// Nor does switching the configuration to that spelling.
			{
				Config: testAccMusicBrainzConfigurationConfig(dashedID, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: resource.TestCheckResourceAttr(name, "plugin_id", pluginID),
			},
			{
				Config: testAccMusicBrainzConfigurationConfig(pluginID, 2),
				Check:  resource.TestCheckResourceAttrSet(name, "configuration_json"),
			},
		},
	})
}

func testAccMusicBrainzConfigurationConfig(pluginID string, rateLimit int) string {
	return fmt.Sprintf(`
resource "jellyfin_plugin_configuration" "test" {
  plugin_id          = %q
  configuration_json = jsonencode({
    Server            = "https://musicbrainz.org"
    RateLimit         = %d
    ReplaceArtistName = false
  })
}
`, pluginID, rateLimit)
}
