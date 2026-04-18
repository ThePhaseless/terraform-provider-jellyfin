// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPluginsDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "jellyfin_plugins" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.jellyfin_plugins.test", "plugins.#"),
				),
			},
		},
	})
}

// testAccFindInstalledPlugin returns the name and id of any installed plugin, or empty strings if none.
func testAccFindInstalledPlugin(t *testing.T) (name, id string) {
	t.Helper()

	endpoint := os.Getenv("JELLYFIN_ENDPOINT")
	apiKey := os.Getenv("JELLYFIN_API_KEY")
	if endpoint == "" || apiKey == "" {
		t.Skip("JELLYFIN_ENDPOINT or JELLYFIN_API_KEY not set")
	}

	c := client.NewClient(endpoint, apiKey)
	plugins, err := c.GetInstalledPlugins()
	if err != nil {
		t.Fatalf("failed to list installed plugins: %v", err)
	}
	if len(plugins) == 0 {
		return "", ""
	}
	return plugins[0].Name, plugins[0].Id
}
