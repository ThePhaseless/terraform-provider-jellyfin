// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBrandingConfigurationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read.
			{
				Config: `
resource "jellyfin_branding_configuration" "test" {
  splashscreen_enabled = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_branding_configuration.test", "splashscreen_enabled", "false"),
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_branding_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "branding",
			},
			// Update.
			{
				Config: `
resource "jellyfin_branding_configuration" "test" {
  splashscreen_enabled = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_branding_configuration.test", "splashscreen_enabled", "true"),
				),
			},
		},
	})
}
