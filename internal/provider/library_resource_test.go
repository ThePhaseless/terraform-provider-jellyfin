// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func TestAccLibraryResource(t *testing.T) {
	testAccPreCheck(t)
	similarItemsSupported := testAccLibrarySimilarItemsSupported(t)

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
			// Set the metadata fetcher order and an image option's minimum width,
			// with path_infos and an empty image_options list elsewhere as in #116.
			{
				Config: `
resource "jellyfin_library" "test" {
  name            = "TestMovies"
  collection_type = "movies"
  paths           = ["/media/movies"]

  library_options = {
    enable_realtime_monitor = false
    save_local_metadata     = false
    path_infos              = [{ path = "/media/movies" }]
    type_options = [
      {
        type                   = "Movie"
        metadata_fetchers      = ["TheMovieDb"]
        metadata_fetcher_order = ["TheMovieDb", "The Open Movie Database"]
        image_fetchers         = ["TheMovieDb"]
        image_fetcher_order    = ["TheMovieDb"]
        image_options          = [{ type = "Backdrop", limit = 1, min_width = 1280 }]
      },
      {
        type          = "Trailer"
        image_options = []
      }
    ]
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.path_infos.0.path", "/media/movies"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.0.metadata_fetcher_order.1", "The Open Movie Database"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.0.image_options.0.min_width", "1280"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.1.image_options.#", "0"),
				),
			},
			// Similar item providers exist on Jellyfin 12 and later only; on an
			// older server they are rejected at plan time.
			{
				SkipFunc:    func() (bool, error) { return similarItemsSupported, nil },
				Config:      testAccLibrarySimilarItemsConfig,
				ExpectError: testAccLibrarySimilarItemsRejected,
			},
			{
				SkipFunc: func() (bool, error) { return !similarItemsSupported, nil },
				Config:   testAccLibrarySimilarItemsConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.0.similar_item_providers.0", "Local Genre/Tag"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.0.similar_item_provider_order.1", "TheMovieDb"),
				),
			},
			// Insert an entry ahead of Movie and leave most of Movie unset: the
			// unset values stay on the server's Movie entry and do not move to
			// the new entry at Movie's former index.
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
        type              = "Trailer"
        metadata_fetchers = []
      },
      {
        type              = "Movie"
        metadata_fetchers = ["The Open Movie Database"]
      }
    ]
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.0.type", "Trailer"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.0.image_fetchers.#", "0"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.1.type", "Movie"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.1.image_fetchers.0", "TheMovieDb"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.1.image_options.0.min_width", "1280"),
					testAccCheckLibraryTypeOptions(t, "TestMovies", "Trailer", map[string][]string{
						"ImageFetchers":        {},
						"SimilarItemProviders": {},
					}),
					testAccCheckLibraryTypeOptions(t, "TestMovies", "Movie", map[string][]string{
						"MetadataFetchers":     {"The Open Movie Database"},
						"MetadataFetcherOrder": {"TheMovieDb", "The Open Movie Database"},
						"ImageFetchers":        {"TheMovieDb"},
						"ImageFetcherOrder":    {"TheMovieDb"},
					}),
					func(s *terraform.State) error {
						if !similarItemsSupported {
							return nil
						}
						return testAccCheckLibraryTypeOptions(t, "TestMovies", "Movie", map[string][]string{
							"SimilarItemProviders":     {"Local Genre/Tag"},
							"SimilarItemProviderOrder": {"Local Genre/Tag", "TheMovieDb"},
						})(s)
					},
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

func TestAccLibraryResourceMusicVideos(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNoLibraryNamed(t, "TestMusicVideos"),
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_library" "test" {
  name            = "TestMusicVideos"
  collection_type = "musicvideos"
  paths           = ["/media/movies"]
}
`,
				Check: resource.TestCheckResourceAttr("jellyfin_library.test", "collection_type", "musicvideos"),
			},
		},
	})
}

func TestAccLibraryResourceDocumentedExample(t *testing.T) {
	example, err := os.ReadFile("../../examples/resources/jellyfin_library/resource.tf")
	if err != nil {
		t.Fatalf("reading the library example: %v", err)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNoLibraryNamed(t, "Movies"),
		Steps: []resource.TestStep{
			{
				Config: string(example),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_library.movies", "library_options.disabled", "false"),
					resource.TestCheckResourceAttr("jellyfin_library.movies", "library_options.preferred_metadata_language", "en"),
					resource.TestCheckResourceAttr("jellyfin_library.movies", "library_options.type_options.0.image_options.0.min_width", "1280"),
				),
			},
		},
	})
}

func TestAccLibraryResourceMappedAndUnsupportedOptions(t *testing.T) {
	testAccPreCheck(t)
	similarItemsSupported := testAccLibrarySimilarItemsSupported(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNoLibraryNamed(t, "TestOptions"),
		Steps: []resource.TestStep{
			// Rejected at plan time, before anything is created.
			{
				Config: `
resource "jellyfin_library" "test" {
  name            = "TestOptions"
  collection_type = "movies"
  paths           = ["/media/movies"]

  library_options = {
    import_missing_episodes = true
  }
}
`,
				ExpectError: regexp.MustCompile(`Unsupported\s+library\s+option`),
			},
			// Jellyfin 10.11 removed network paths.
			{
				Config: `
resource "jellyfin_library" "test" {
  name            = "TestOptions"
  collection_type = "movies"
  paths           = ["/media/movies"]

  library_options = {
    path_infos = [{ path = "/media/movies", network_path = "smb://nas/movies" }]
  }
}
`,
				ExpectError: regexp.MustCompile(`Jellyfin\s+10\.11\s+removed\s+network\s+paths[\s\S]*Remove\s+library_options\.path_infos\[0\]\.network_path`),
			},
			{
				SkipFunc: func() (bool, error) { return similarItemsSupported, nil },
				Config: `
resource "jellyfin_library" "test" {
  name            = "TestOptions"
  collection_type = "movies"
  paths           = ["/media/movies"]

  library_options = {
    type_options = [{ type = "Movie", similar_item_providers = ["Local Genre/Tag"] }]
  }
}
`,
				ExpectError: testAccLibrarySimilarItemsRejected,
			},
			// disabled and extract_chapters_during_library_scan are stored as
			// Jellyfin's Enabled and ExtractChapterImagesDuringLibraryScan.
			{
				Config: `
resource "jellyfin_library" "test" {
  name            = "TestOptions"
  collection_type = "movies"
  paths           = ["/media/movies"]

  library_options = {
    disabled                             = true
    extract_chapters_during_library_scan = true
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.disabled", "true"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.extract_chapters_during_library_scan", "true"),
					testAccCheckLibraryOption(t, "TestOptions", "Enabled", "false"),
					testAccCheckLibraryOption(t, "TestOptions", "ExtractChapterImagesDuringLibraryScan", "true"),
				),
			},
		},
	})
}

const testAccLibrarySimilarItemsConfig = `
resource "jellyfin_library" "test" {
  name            = "TestMovies"
  collection_type = "movies"
  paths           = ["/media/movies"]

  library_options = {
    enable_realtime_monitor = false
    save_local_metadata     = false
    type_options = [
      {
        type                        = "Movie"
        metadata_fetchers           = ["TheMovieDb"]
        image_fetchers              = ["TheMovieDb"]
        image_fetcher_order         = ["TheMovieDb"]
        similar_item_providers      = ["Local Genre/Tag"]
        similar_item_provider_order = ["Local Genre/Tag", "TheMovieDb"]
      }
    ]
  }
}
`

// testAccLibrarySimilarItemsRejected matches the plan-time error only, not the
// one reported after apply when the server drops the settings.
var testAccLibrarySimilarItemsRejected = regexp.MustCompile(`similar\s+item\s+providers\s+need\s+Jellyfin\s+12\s+or\s+later\.\s+Remove\s+library_options\.type_options\[0\]\.similar_item_providers`)

func testAccLibrarySimilarItemsSupported(t *testing.T) bool {
	t.Helper()

	info, err := client.NewClient(os.Getenv("JELLYFIN_ENDPOINT"), "").GetPublicSystemInfo(context.Background())
	if err != nil {
		t.Fatalf("reading the Jellyfin version: %v", err)
	}
	return compareDottedVersions(info.Version, "12") >= 0
}

// testAccCheckLibraryTypeOptions checks string lists of the server's type
// options entry, where values the configuration leaves unset live.
func testAccCheckLibraryTypeOptions(t *testing.T, library, typ string, want map[string][]string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		folders, err := testAccClient(t).GetVirtualFolders(context.Background())
		if err != nil {
			return err
		}
		idx := slices.IndexFunc(folders, func(f client.VirtualFolder) bool { return f.Name == library })
		if idx < 0 {
			return fmt.Errorf("library %q not found", library)
		}
		var opts struct {
			TypeOptions []map[string]json.RawMessage
		}
		if err := json.Unmarshal(folders[idx].LibraryOptions, &opts); err != nil {
			return fmt.Errorf("parsing library options of %q: %w", library, err)
		}
		entry := slices.IndexFunc(opts.TypeOptions, func(e map[string]json.RawMessage) bool {
			var got string
			return json.Unmarshal(e["Type"], &got) == nil && got == typ
		})
		if entry < 0 {
			return fmt.Errorf("library %q has no %s type options", library, typ)
		}
		for key, values := range want {
			var got []string
			if raw, ok := opts.TypeOptions[entry][key]; ok {
				if err := json.Unmarshal(raw, &got); err != nil {
					return fmt.Errorf("parsing %s of %s: %w", key, typ, err)
				}
			}
			if !slices.Equal(got, values) {
				return fmt.Errorf("%s type options %s = %q on the server, want %q", typ, key, got, values)
			}
		}
		return nil
	}
}

// testAccCheckLibraryOption compares one of the server's library options with
// a JSON literal, to check the key the provider writes an attribute to.
func testAccCheckLibraryOption(t *testing.T, library, key, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		folders, err := testAccClient(t).GetVirtualFolders(context.Background())
		if err != nil {
			return err
		}
		idx := slices.IndexFunc(folders, func(f client.VirtualFolder) bool { return f.Name == library })
		if idx < 0 {
			return fmt.Errorf("library %q not found", library)
		}
		var opts map[string]json.RawMessage
		if err := json.Unmarshal(folders[idx].LibraryOptions, &opts); err != nil {
			return fmt.Errorf("parsing library options of %q: %w", library, err)
		}
		if got := string(opts[key]); got != want {
			return fmt.Errorf("library option %s = %s on the server, want %s", key, got, want)
		}
		return nil
	}
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
