// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPluginResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Install plugin.
			{
				Config: `
resource "jellyfin_plugin_repository" "stable" {
  name    = "Jellyfin Stable"
  url     = "https://repo.jellyfin.org/files/plugin/manifest.json"
  enabled = true
}

resource "jellyfin_plugin" "test" {
  name           = "Bookshelf"
  version        = "13.0.0.0"
  repository_url = jellyfin_plugin_repository.stable.url
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_plugin.test", "id"),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "name", "Bookshelf"),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "version", "13.0.0.0"),
				),
			},
		},
	})
}
