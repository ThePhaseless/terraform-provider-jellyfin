// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSystemInfoDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "jellyfin_system_info" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.jellyfin_system_info.test", "id"),
					resource.TestCheckResourceAttrSet("data.jellyfin_system_info.test", "version"),
					resource.TestCheckResourceAttrSet("data.jellyfin_system_info.test", "server_name"),
				),
			},
		},
	})
}
