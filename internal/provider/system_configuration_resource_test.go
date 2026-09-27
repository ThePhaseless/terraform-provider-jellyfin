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
				Config: testAccSystemConfigurationResourceConfig("TestServer", systemConfigurationTestValues{
					itemIDFlags:            false,
					imageSavingConvention:  "Compatible",
					chapterImageResolution: "P720",
					scanBehavior:           "Blocking",
					processPriority:        "Idle",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "server_name", "TestServer"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "enable_normalized_item_by_name_ids", "false"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "enable_case_sensitive_item_ids", "false"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "image_saving_convention", "Compatible"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "chapter_image_resolution", "P720"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "trickplay_options.scan_behavior", "Blocking"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "trickplay_options.process_priority", "Idle"),
				),
			},
			// ImportState.
			{
				ResourceName:            "jellyfin_system_configuration.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateId:           "system",
				ImportStateVerifyIgnore: []string{"server_name"},
			},
			// Update back to a fresh server's values so later tests start from the defaults.
			{
				Config: testAccSystemConfigurationResourceConfig("UpdatedServer", systemConfigurationTestValues{
					itemIDFlags:            true,
					imageSavingConvention:  "Legacy",
					chapterImageResolution: "MatchSource",
					scanBehavior:           "NonBlocking",
					processPriority:        "BelowNormal",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "server_name", "UpdatedServer"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "enable_normalized_item_by_name_ids", "true"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "enable_case_sensitive_item_ids", "true"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "image_saving_convention", "Legacy"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "chapter_image_resolution", "MatchSource"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "trickplay_options.scan_behavior", "NonBlocking"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "trickplay_options.process_priority", "BelowNormal"),
				),
			},
		},
	})
}

type systemConfigurationTestValues struct {
	itemIDFlags            bool
	imageSavingConvention  string
	chapterImageResolution string
	scanBehavior           string
	processPriority        string
}

func testAccSystemConfigurationResourceConfig(serverName string, v systemConfigurationTestValues) string {
	itemIDFlags := "false"
	if v.itemIDFlags {
		itemIDFlags = "true"
	}
	return `
resource "jellyfin_system_configuration" "test" {
  server_name                        = "` + serverName + `"
  enable_normalized_item_by_name_ids = ` + itemIDFlags + `
  enable_case_sensitive_item_ids     = ` + itemIDFlags + `
  image_saving_convention            = "` + v.imageSavingConvention + `"
  chapter_image_resolution           = "` + v.chapterImageResolution + `"

  trickplay_options = {
    scan_behavior    = "` + v.scanBehavior + `"
    process_priority = "` + v.processPriority + `"
  }
}
`
}
