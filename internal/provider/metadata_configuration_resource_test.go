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
  use_file_creation_time_for_date_added = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
resource.TestCheckResourceAttr("jellyfin_metadata_configuration.test", "use_file_creation_time_for_date_added", "true"),
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_metadata_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "metadata",
			},
			// Update.
			{
				Config: `
resource "jellyfin_metadata_configuration" "test" {
  use_file_creation_time_for_date_added = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
resource.TestCheckResourceAttr("jellyfin_metadata_configuration.test", "use_file_creation_time_for_date_added", "false"),
				),
			},
		},
	})
}
