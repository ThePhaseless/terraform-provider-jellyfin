// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccUserResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read.
			{
				Config: testAccUserResourceConfig("testuser1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_user.test", "id"),
					resource.TestCheckResourceAttr("jellyfin_user.test", "name", "testuser1"),
					resource.TestCheckResourceAttr("jellyfin_user.test", "is_administrator", "false"),
				),
			},
			// ImportState.
			{
				ResourceName:            "jellyfin_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			// Update.
			{
				Config: testAccUserResourceConfig("testuser1_updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_user.test", "name", "testuser1_updated"),
				),
			},
		},
	})
}

func testAccUserResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "jellyfin_user" "test" {
  name     = %[1]q
  password = "testpass123"
}
`, name)
}
