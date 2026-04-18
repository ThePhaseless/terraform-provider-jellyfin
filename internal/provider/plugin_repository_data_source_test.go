// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPluginRepositoryDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "jellyfin_plugin_repository" "test" {
  name    = "DSRepoLookup"
  url     = %q
  enabled = true
}

data "jellyfin_plugin_repository" "test" {
  name       = jellyfin_plugin_repository.test.name
  depends_on = [jellyfin_plugin_repository.test]
}
`, stableRepoURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jellyfin_plugin_repository.test", "name", "DSRepoLookup"),
					resource.TestCheckResourceAttr("data.jellyfin_plugin_repository.test", "url", stableRepoURL),
					resource.TestCheckResourceAttr("data.jellyfin_plugin_repository.test", "enabled", "true"),
				),
			},
		},
	})
}
