// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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

func TestAccLibraryResourceMissingPath(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNoLibraryNamed(t, "TestMissingPath"),
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_library" "test" {
  name            = "TestMissingPath"
  collection_type = "movies"
  paths           = ["/media/movies", "/media/does-not-exist"]
}
`,
				ExpectError: regexp.MustCompile(`These\s+paths\s+do\s+not\s+exist\s+on\s+the\s+Jellyfin\s+server:\s+"/media/does-not-exist"\.`),
			},
		},
	})
}

func TestAccLibraryResourceDuplicateName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNoLibraryNamed(t, "TestDuplicate"),
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_library" "first" {
  name            = "TestDuplicate"
  collection_type = "movies"
  paths           = ["/media/movies"]
}

resource "jellyfin_library" "second" {
  name            = "TestDuplicate"
  collection_type = "tvshows"
  paths           = ["/media/tvshows"]

  depends_on = [jellyfin_library.first]
}
`,
				ExpectError: regexp.MustCompile(`A\s+library\s+named\s+"TestDuplicate"\s+already\s+exists`),
			},
		},
	})
}

// testAccCheckNoLibraryNamed also catches the numbered copies Jellyfin makes
// of a duplicate name.
func testAccCheckNoLibraryNamed(t *testing.T, prefix string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		folders, err := testAccClient(t).GetVirtualFolders(context.Background())
		if err != nil {
			return err
		}
		for _, f := range folders {
			if strings.HasPrefix(f.Name, prefix) {
				return fmt.Errorf("library %q is still on the server", f.Name)
			}
		}
		return nil
	}
}
