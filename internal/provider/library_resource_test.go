// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func TestAccLibraryResource(t *testing.T) {
	testAccPreCheck(t)
	similarItemsSupported := testAccJellyfin12OrLater(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
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
			// Update library_options in place.
			{
				Config: testAccLibraryConfig("TestMovies", `
    enable_realtime_monitor = false
    save_local_metadata     = false
    type_options = [
      {
        type                = "Movie"
        metadata_fetchers   = ["TheMovieDb"]
        image_fetchers      = ["TheMovieDb"]
        image_fetcher_order = ["TheMovieDb"]
      }
    ]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.enable_realtime_monitor", "false"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.0.type", "Movie"),
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.0.image_fetchers.0", "TheMovieDb"),
				),
			},
			{
				Config: testAccLibraryConfig("TestMovies", `
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
    ]`),
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
			// Insert an entry ahead of Movie: Movie's unset values stay on Movie,
			// not on the entry at its old index.
			{
				Config: testAccLibraryConfig("TestMovies", `
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
    ]`),
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
						"MetadataFetcherOrder": {"The Open Movie Database", "TheMovieDb"},
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

func TestAccLibraryResourceImportWithoutCollectionTypeAsMixed(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNoLibraryNamed(t, "TestUntyped"),
		Steps: []resource.TestStep{
			{
				// Jellyfin's web UI creates a Mixed Movies and Shows library
				// like this, without a collection type.
				PreConfig: func() {
					if err := testAccClient(t).AddVirtualFolder(t.Context(), "TestUntyped", "", []string{"/media/movies"}, nil); err != nil {
						t.Fatalf("creating a library without a collection type: %v", err)
					}
				},
				Config: `
import {
  to = jellyfin_library.test
  id = "TestUntyped"
}

resource "jellyfin_library" "test" {
  name            = "TestUntyped"
  collection_type = "mixed"
  paths           = ["/media/movies"]
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("jellyfin_library.test", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.TestCheckResourceAttr("jellyfin_library.test", "collection_type", "mixed"),
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
	similarItemsSupported := testAccJellyfin12OrLater(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNoLibraryNamed(t, "TestOptions"),
		Steps: []resource.TestStep{
			// Rejected at plan time, before anything is created.
			{
				Config:      testAccLibraryConfig("TestOptions", `import_missing_episodes = true`),
				ExpectError: regexp.MustCompile(`Unsupported\s+library\s+option`),
			},
			// Jellyfin 10.10 removed network paths.
			{
				Config:      testAccLibraryConfig("TestOptions", `path_infos = [{ path = "/media/movies", network_path = "smb://nas/movies" }]`),
				ExpectError: testAccLibraryNetworkPathRejected,
			},
			// A value unknown at plan time is checked when Terraform plans again
			// during apply.
			{
				Config: `
resource "terraform_data" "network_path" {
  input = "smb://nas/movies"
}
` + testAccLibraryConfig("TestOptions", `path_infos = [{ path = "/media/movies", network_path = terraform_data.network_path.output }]`),
				ExpectError: testAccLibraryNetworkPathRejected,
			},
			{
				SkipFunc:    func() (bool, error) { return similarItemsSupported, nil },
				Config:      testAccLibraryConfig("TestOptions", `type_options = [{ type = "Movie", similar_item_providers = ["Local Genre/Tag"] }]`),
				ExpectError: testAccLibrarySimilarItemsRejected,
			},
			// disabled and extract_chapters_... map to Enabled and
			// ExtractChapterImagesDuringLibraryScan; unset network path plans null.
			{
				PreConfig: func() {
					if err := testAccCheckNoLibraryNamed(t, "TestOptions")(nil); err != nil {
						t.Fatalf("a rejected configuration created the library: %v", err)
					}
				},
				Config: testAccLibraryConfig("TestOptions", `
    disabled                             = true
    extract_chapters_during_library_scan = true
    path_infos                           = [{ path = "/media/movies" }]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("jellyfin_library.test", plancheck.ResourceActionCreate),
						plancheck.ExpectKnownValue("jellyfin_library.test",
							tfjsonpath.New("library_options").AtMapKey("path_infos").AtSliceIndex(0).AtMapKey("network_path"),
							knownvalue.Null()),
					},
				},
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

// A fresh server offers no subtitle fetchers, as they all come from plugins,
// so subtitle_fetchers can only enable none of them here.
func TestAccLibraryResourceProviderLists(t *testing.T) {
	movie := tfjsonpath.New("library_options").AtMapKey("type_options").AtSliceIndex(0)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNoLibraryNamed(t, "TestLists"),
		Steps: []resource.TestStep{
			{
				Config: testAccLibraryConfig("TestLists", `
    subtitle_fetchers = []
    type_options = [{
      type              = "Movie"
      metadata_fetchers = ["TheMovieDb", "The Open Movie Database"]
      image_fetchers    = ["TheMovieDb", "The Open Movie Database"]
    }]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.subtitle_fetchers.#", "0"),
					testAccCheckLibraryOption(t, "TestLists", "DisabledSubtitleFetchers", `[]`),
					testAccCheckLibraryOption(t, "TestLists", "SubtitleFetcherOrder", `[]`),
					testAccCheckLibraryTypeOptions(t, "TestLists", "Movie", map[string][]string{
						"MetadataFetchers":     {"TheMovieDb", "The Open Movie Database"},
						"MetadataFetcherOrder": {"TheMovieDb", "The Open Movie Database"},
						"ImageFetchers":        {"TheMovieDb", "The Open Movie Database"},
						"ImageFetcherOrder":    {"TheMovieDb", "The Open Movie Database"},
					}),
				),
			},
			// Reordering a list reorders the server's order, and leaving a
			// fetcher out disables it and moves it behind the enabled ones.
			{
				Config: testAccLibraryConfig("TestLists", `
    subtitle_fetchers = []
    type_options = [{
      type              = "Movie"
      metadata_fetchers = ["The Open Movie Database", "TheMovieDb"]
      image_fetchers    = ["The Open Movie Database"]
    }]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue("jellyfin_library.test", movie.AtMapKey("metadata_fetcher_order")),
						plancheck.ExpectUnknownValue("jellyfin_library.test", movie.AtMapKey("image_fetcher_order")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.0.metadata_fetcher_order.0", "The Open Movie Database"),
					testAccCheckLibraryTypeOptions(t, "TestLists", "Movie", map[string][]string{
						"MetadataFetchers":     {"The Open Movie Database", "TheMovieDb"},
						"MetadataFetcherOrder": {"The Open Movie Database", "TheMovieDb"},
						"ImageFetchers":        {"The Open Movie Database"},
						"ImageFetcherOrder":    {"The Open Movie Database", "TheMovieDb"},
					}),
				),
			},
			{
				Config: testAccLibraryConfig("TestLists", `
    subtitle_fetchers = []
    type_options = [{
      type              = "Movie"
      metadata_fetchers = ["The Open Movie Database", "TheMovieDb"]
      image_fetchers    = ["TheMovieDb", "The Open Movie Database"]
    }]`),
				Check: testAccCheckLibraryTypeOptions(t, "TestLists", "Movie", map[string][]string{
					"ImageFetchers":     {"TheMovieDb", "The Open Movie Database"},
					"ImageFetcherOrder": {"TheMovieDb", "The Open Movie Database"},
				}),
			},
			// The deprecated order attribute still overrides the order the list
			// would set.
			{
				Config: testAccLibraryConfig("TestLists", `
    subtitle_fetchers = []
    type_options = [{
      type                   = "Movie"
      metadata_fetchers      = ["TheMovieDb"]
      metadata_fetcher_order = ["The Open Movie Database", "TheMovieDb"]
      image_fetchers         = ["TheMovieDb", "The Open Movie Database"]
    }]`),
				Check: testAccCheckLibraryTypeOptions(t, "TestLists", "Movie", map[string][]string{
					"MetadataFetchers":     {"TheMovieDb"},
					"MetadataFetcherOrder": {"The Open Movie Database", "TheMovieDb"},
				}),
			},
			{
				Config: testAccLibraryConfig("TestLists", `
    subtitle_fetchers          = []
    disabled_subtitle_fetchers = []`),
				ExpectError: regexp.MustCompile(`Attribute\s+"library_options.disabled_subtitle_fetchers"\s+cannot\s+be\s+specified\s+when\s+"library_options.subtitle_fetchers"\s+is\s+specified`),
			},
			{
				Config:      testAccLibraryConfig("TestLists", `subtitle_fetchers = ["Open Subtitles"]`),
				ExpectError: testAccLibraryUnofferedSubtitleFetcher,
			},
			// The attributes subtitle_fetchers replaces still work alone.
			{
				Config: testAccLibraryConfig("TestLists", `
    disabled_subtitle_fetchers = ["Open Subtitles"]
    subtitle_fetcher_order     = ["Open Subtitles"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue("jellyfin_library.test", tfjsonpath.New("library_options").AtMapKey("subtitle_fetchers")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.subtitle_fetchers.#", "0"),
					testAccCheckLibraryOption(t, "TestLists", "DisabledSubtitleFetchers", `["Open Subtitles"]`),
					testAccCheckLibraryOption(t, "TestLists", "SubtitleFetcherOrder", `["Open Subtitles"]`),
				),
			},
		},
	})
}

// Jellyfin enables a fetcher whatever the case of its name, but ranks only the
// name it offers, and a new library's order holds no name to take it from.
func TestAccLibraryResourceOrderTakesTheOfferedSpelling(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNoLibraryNamed(t, "TestLists"),
		Steps: []resource.TestStep{
			{
				Config: testAccLibraryConfig("TestLists", `
    type_options = [{
      type           = "Movie"
      image_fetchers = ["the open movie database", "TheMovieDb"]
    }]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_library.test", "library_options.type_options.0.image_fetchers.0", "the open movie database"),
					testAccCheckLibraryTypeOptions(t, "TestLists", "Movie", map[string][]string{
						"ImageFetchers":     {"the open movie database", "TheMovieDb"},
						"ImageFetcherOrder": {"the open movie database", "The Open Movie Database", "TheMovieDb"},
					}),
				),
			},
		},
	})
}

func TestAccLibraryResourceCreateWithUnofferedSubtitleFetcherLeavesNoLibrary(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNoLibraryNamed(t, "TestLists"),
		Steps: []resource.TestStep{
			{
				Config:      testAccLibraryConfig("TestLists", `subtitle_fetchers = ["Open Subtitles"]`),
				ExpectError: testAccLibraryUnofferedSubtitleFetcher,
			},
			{
				PreConfig: func() {
					if err := testAccCheckNoLibraryNamed(t, "TestLists")(nil); err != nil {
						t.Fatal(err)
					}
				},
				Config: testAccLibraryConfig("TestLists", `subtitle_fetchers = []`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("jellyfin_library.test", plancheck.ResourceActionCreate)},
				},
			},
		},
	})
}

func testAccLibraryConfig(name, options string) string {
	return `
resource "jellyfin_library" "test" {
  name            = "` + name + `"
  collection_type = "movies"
  paths           = ["/media/movies"]

  library_options = {
    ` + options + `
  }
}
`
}

var testAccLibrarySimilarItemsConfig = testAccLibraryConfig("TestMovies", `
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
    ]`)

var testAccLibraryUnofferedSubtitleFetcher = regexp.MustCompile(`library_options.subtitle_fetchers\s+lists\s+"Open\s+Subtitles",\s+which\s+is\s+not\s+one\s+of\s+the\s+SubtitleFetchers\s+the\s+Jellyfin\s+server\s+offers:\s+none`)

var testAccLibraryNetworkPathRejected = regexp.MustCompile(`Jellyfin\s+10\.10\s+removed\s+network\s+paths[\s\S]*Remove\s+library_options\.path_infos\[0\]\.network_path`)

// testAccLibrarySimilarItemsRejected matches the plan-time error only, not the
// one reported after apply when the server drops the settings.
var testAccLibrarySimilarItemsRejected = regexp.MustCompile(`similar\s+item\s+providers\s+need\s+Jellyfin\s+12\s+or\s+later\.\s+Remove\s+library_options\.type_options\[0\]\.similar_item_providers`)

// testAccCheckLibraryTypeOptions checks string lists of the server's type
// options entry, where values the configuration leaves unset live.
func testAccCheckLibraryTypeOptions(t *testing.T, library, typ string, want map[string][]string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		served, err := testAccServedLibraryOptions(t, library)
		if err != nil {
			return err
		}
		var opts struct {
			TypeOptions []map[string]json.RawMessage
		}
		if err := json.Unmarshal(served, &opts); err != nil {
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
		served, err := testAccServedLibraryOptions(t, library)
		if err != nil {
			return err
		}
		var opts map[string]json.RawMessage
		if err := json.Unmarshal(served, &opts); err != nil {
			return fmt.Errorf("parsing library options of %q: %w", library, err)
		}
		if got := string(opts[key]); got != want {
			return fmt.Errorf("library option %s = %s on the server, want %s", key, got, want)
		}
		return nil
	}
}

func testAccServedLibraryOptions(t *testing.T, library string) (json.RawMessage, error) {
	folders, err := testAccClient(t).GetVirtualFolders(t.Context())
	if err != nil {
		return nil, err
	}
	idx := slices.IndexFunc(folders, func(f client.VirtualFolder) bool { return f.Name == library })
	if idx < 0 {
		return nil, fmt.Errorf("library %q not found", library)
	}
	return folders[idx].LibraryOptions, nil
}

// testAccCheckNoLibraryNamed also catches the numbered copies Jellyfin makes
// of a duplicate name.
func testAccCheckNoLibraryNamed(t *testing.T, prefix string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		folders, err := testAccClient(t).GetVirtualFolders(t.Context())
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
