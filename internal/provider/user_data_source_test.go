// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccUserDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_user" "test" {
  name     = "ds_user_lookup"
  password = "testpass123"
}

data "jellyfin_user" "test" {
  name       = jellyfin_user.test.name
  depends_on = [jellyfin_user.test]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jellyfin_user.test", "id", "jellyfin_user.test", "id"),
					resource.TestCheckResourceAttr("data.jellyfin_user.test", "name", "ds_user_lookup"),
					resource.TestCheckResourceAttrSet("data.jellyfin_user.test", "is_administrator"),
				),
			},
		},
	})
}
