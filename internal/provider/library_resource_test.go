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
			// Update library_options in place: the options endpoint takes the
			// item id and the options, not the options alone.
			{
				Config: `
resource "jellyfin_library" "test" {
  name            = "TestMovies"
  collection_type = "movies"
  paths           = ["/media/movies"]

  library_options = {
    enable_realtime_monitor = false
    save_local_metadata     = false
    type_options = [
      {
        type                = "Movie"
        metadata_fetchers   = ["TheMovieDb"]
        image_fetchers      = ["TheMovieDb"]
        image_fetcher_order = ["TheMovieDb"]
      }
    ]
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.enable_realtime_monitor", "false"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.0.type", "Movie"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.0.image_fetchers.0", "TheMovieDb"),
				),
			},
			// ImportState.
			{
				ResourceName:                         "jellyfin_library.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateId:                        "TestMovies",
				ImportStateVerifyIgnore:              []string{"library_options"},
			},
		},
	})
}
