// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPluginDataSource(t *testing.T) {
	pluginName, _ := testAccFindInstalledPlugin(t)
	if pluginName == "" {
		t.Skip("no installed plugin available for lookup test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "jellyfin_plugin" "test" {
  name = %q
}
`, pluginName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jellyfin_plugin.test", "name", pluginName),
					resource.TestCheckResourceAttrSet("data.jellyfin_plugin.test", "id"),
					resource.TestCheckResourceAttrSet("data.jellyfin_plugin.test", "version"),
				),
			},
		},
	})
}
