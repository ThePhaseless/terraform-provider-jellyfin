// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBrandingConfigurationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_branding_configuration" "test" {
  splashscreen_location = ""
}
`,
				ExpectError: regexp.MustCompile(`Unsupported branding option`),
			},
			{
				Config: `
resource "jellyfin_branding_configuration" "test" {
  login_disclaimer     = "Authorized users only."
  custom_css           = ".skinHeader { opacity: 0.9; }"
  splashscreen_enabled = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_branding_configuration.test", "login_disclaimer", "Authorized users only."),
					resource.TestCheckResourceAttr("jellyfin_branding_configuration.test", "custom_css", ".skinHeader { opacity: 0.9; }"),
					resource.TestCheckResourceAttr("jellyfin_branding_configuration.test", "splashscreen_enabled", "false"),
					resource.TestCheckNoResourceAttr("jellyfin_branding_configuration.test", "splashscreen_location"),
				),
			},
			{
				ResourceName:      "jellyfin_branding_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "branding",
			},
			{
				Config: `
resource "jellyfin_branding_configuration" "test" {
  login_disclaimer     = ""
  custom_css           = ""
  splashscreen_enabled = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_branding_configuration.test", "login_disclaimer", ""),
					resource.TestCheckResourceAttr("jellyfin_branding_configuration.test", "custom_css", ""),
					resource.TestCheckResourceAttr("jellyfin_branding_configuration.test", "splashscreen_enabled", "true"),
				),
			},
		},
	})
}
