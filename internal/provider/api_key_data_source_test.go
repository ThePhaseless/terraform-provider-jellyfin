// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAPIKeyDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_api_key" "test" {
  app_name = "ds-api-key-lookup"
}

data "jellyfin_api_key" "test" {
  app_name   = jellyfin_api_key.test.app_name
  depends_on = [jellyfin_api_key.test]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jellyfin_api_key.test", "app_name", "ds-api-key-lookup"),
					resource.TestCheckResourceAttrSet("data.jellyfin_api_key.test", "access_token"),
				),
			},
		},
	})
}
