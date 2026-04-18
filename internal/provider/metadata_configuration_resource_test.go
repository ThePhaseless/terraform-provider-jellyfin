// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccMetadataConfigurationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read.
			{
				Config: `
resource "jellyfin_metadata_configuration" "test" {
  configuration_json = jsonencode({
    UseFileCreationTimeForDateAdded = true
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_metadata_configuration.test", "configuration_json"),
				),
			},
			// ImportState.
			{
				ResourceName:            "jellyfin_metadata_configuration.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateId:           "metadata",
				ImportStateVerifyIgnore: []string{"configuration_json"},
			},
			// Update.
			{
				Config: `
resource "jellyfin_metadata_configuration" "test" {
  configuration_json = jsonencode({
    UseFileCreationTimeForDateAdded = false
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_metadata_configuration.test", "configuration_json"),
				),
			},
		},
	})
}
