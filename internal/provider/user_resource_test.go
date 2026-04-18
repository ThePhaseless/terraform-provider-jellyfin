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
				Config: testAccUserResourceConfigWithPassword("testuser1", "testpass123"),
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
			// Update name.
			{
				Config: testAccUserResourceConfigWithPassword("testuser1_updated", "testpass123"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_user.test", "name", "testuser1_updated"),
				),
			},
			// Update password (regression test: previously failed because the
			// API requires the old password unless we reset first).
			{
				Config: testAccUserResourceConfigWithPassword("testuser1_updated", "newpass456"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_user.test", "name", "testuser1_updated"),
					resource.TestCheckResourceAttr("jellyfin_user.test", "password", "newpass456"),
				),
			},
		},
	})
}

func testAccUserResourceConfigWithPassword(name, password string) string {
	return fmt.Sprintf(`
resource "jellyfin_user" "test" {
  name     = %[1]q
  password = %[2]q
}
`, name, password)
}
