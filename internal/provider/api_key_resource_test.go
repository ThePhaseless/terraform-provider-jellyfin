// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccAPIKeyResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read.
			{
				Config: `
resource "jellyfin_api_key" "test" {
  app_name = "terraform-test-key"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_api_key.test", "app_name", "terraform-test-key"),
					resource.TestCheckResourceAttrSet("jellyfin_api_key.test", "access_token"),
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_api_key.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["jellyfin_api_key.test"]
					if !ok {
						return "", fmt.Errorf("resource not found: jellyfin_api_key.test")
					}
					return rs.Primary.Attributes["access_token"], nil
				},
				ImportStateVerifyIgnore: []string{"access_token"},
			},
		},
	})
}
