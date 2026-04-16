// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccLibraryResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read.
			{
				Config: `
resource "jellyfin_library" "test" {
  name            = "TestMovies"
  collection_type = "movies"
  paths           = ["/media/movies"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_library.test", "name", "TestMovies"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "collection_type", "movies"),
					resource.TestCheckResourceAttrSet("jellyfin_library.test", "item_id"),
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_library.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
