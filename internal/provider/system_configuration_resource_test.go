// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccSystemConfigurationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_system_configuration" "test" {
  trickplay_options = {
    process_priority = "idle"
  }
}
`,
				ExpectError: regexp.MustCompile(`trickplay_options\.process_priority\s+value\s+must\s+be\s+one\s+of`),
			},
			{
				Config: `
resource "jellyfin_system_configuration" "test" {
  cast_receiver_applications = [{ id = "F007D354" }]
}
`,
				ExpectError: regexp.MustCompile(`"cast_receiver_applications\[0\]\.name"\s+must\s+be\s+specified`),
			},
			{
				Config: testAccSystemConfigurationResourceConfig("TestServer", systemConfigurationTestValues{
					itemIDFlags:              false,
					imageSavingConvention:    "Compatible",
					chapterImageResolution:   "P720",
					scanBehavior:             "Blocking",
					processPriority:          "Idle",
					castReceiverApplications: `[{ id = "F007D354", name = "Stable" }]`,
					pathSubstitutions:        `[]`,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "server_name", "TestServer"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "enable_normalized_item_by_name_ids", "false"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "enable_case_sensitive_item_ids", "false"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "image_saving_convention", "Compatible"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "chapter_image_resolution", "P720"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "trickplay_options.scan_behavior", "Blocking"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "trickplay_options.process_priority", "Idle"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "cast_receiver_applications.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "cast_receiver_applications.0.id", "F007D354"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "cast_receiver_applications.0.name", "Stable"),
				),
			},
			{
				ResourceName:            "jellyfin_system_configuration.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateId:           "system",
				ImportStateVerifyIgnore: []string{"server_name"},
			},
			// An entry added at update has no prior values; to is left out, and Jellyfin fills in "".
			{
				Config: testAccSystemConfigurationResourceConfig("TestServer", systemConfigurationTestValues{
					itemIDFlags:              false,
					imageSavingConvention:    "Compatible",
					chapterImageResolution:   "P720",
					scanBehavior:             "Blocking",
					processPriority:          "Idle",
					castReceiverApplications: `[{ id = "F007D354", name = "Stable" }]`,
					pathSubstitutions:        `[{ from = "/mnt/media" }]`,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "path_substitutions.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "path_substitutions.0.from", "/mnt/media"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "path_substitutions.0.to", ""),
				),
			},
			// Update back to a fresh server's values so later tests start from the defaults.
			{
				Config: testAccSystemConfigurationResourceConfig("UpdatedServer", systemConfigurationTestValues{
					itemIDFlags:              true,
					imageSavingConvention:    "Legacy",
					chapterImageResolution:   "MatchSource",
					scanBehavior:             "NonBlocking",
					processPriority:          "BelowNormal",
					castReceiverApplications: `[{ id = "F007D354", name = "Stable" }, { id = "6F511C87", name = "Unstable" }]`,
					pathSubstitutions:        `[]`,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "server_name", "UpdatedServer"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "enable_normalized_item_by_name_ids", "true"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "enable_case_sensitive_item_ids", "true"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "image_saving_convention", "Legacy"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "chapter_image_resolution", "MatchSource"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "trickplay_options.scan_behavior", "NonBlocking"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "trickplay_options.process_priority", "BelowNormal"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "cast_receiver_applications.#", "2"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "cast_receiver_applications.1.id", "6F511C87"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "cast_receiver_applications.1.name", "Unstable"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "path_substitutions.#", "0"),
				),
			},
			// Leaves every nested attribute out so the post-apply empty-plan check covers omitted lists and objects.
			{
				Config: `
resource "jellyfin_system_configuration" "test" {
  server_name = "UpdatedServer"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "cast_receiver_applications.#", "2"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "trickplay_options.process_priority", "BelowNormal"),
				),
			},
		},
	})
}

type systemConfigurationTestValues struct {
	itemIDFlags              bool
	imageSavingConvention    string
	chapterImageResolution   string
	scanBehavior             string
	processPriority          string
	castReceiverApplications string
	pathSubstitutions        string
}

func testAccSystemConfigurationResourceConfig(serverName string, v systemConfigurationTestValues) string {
	itemIDFlags := "false"
	if v.itemIDFlags {
		itemIDFlags = "true"
	}
	return `
resource "jellyfin_system_configuration" "test" {
  server_name                        = "` + serverName + `"
  enable_normalized_item_by_name_ids = ` + itemIDFlags + `
  enable_case_sensitive_item_ids     = ` + itemIDFlags + `
  image_saving_convention            = "` + v.imageSavingConvention + `"
  chapter_image_resolution           = "` + v.chapterImageResolution + `"

  trickplay_options = {
    scan_behavior    = "` + v.scanBehavior + `"
    process_priority = "` + v.processPriority + `"
  }

  cast_receiver_applications = ` + v.castReceiverApplications + `
  path_substitutions         = ` + v.pathSubstitutions + `
}
`
}

// testAccPutBackSystemConfiguration posts back, once the test ends, the
// system configuration as the server serves it now.
func testAccPutBackSystemConfiguration(t *testing.T) {
	t.Helper()
	original, err := testAccClient(t).GetSystemConfiguration(t.Context())
	if err != nil {
		t.Fatalf("reading the system configuration: %v", err)
	}
	t.Cleanup(func() {
		if err := testAccClient(t).UpdateSystemConfiguration(context.Background(), original); err != nil {
			t.Errorf("putting back the system configuration: %v", err)
		}
	})
}

func TestAccSystemConfigurationFetchers(t *testing.T) {
	testAccPreCheck(t)
	testAccPutBackSystemConfiguration(t)

	movie := tfjsonpath.New("metadata_options").AtSliceIndex(0)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSystemConfigurationFetchersConfig(`
    item_type         = "Movie"
    metadata_fetchers = ["TheMovieDb", "The Open Movie Database"]
    image_fetchers    = ["TheMovieDb", "The Open Movie Database", "Embedded Image Extractor", "Screen Grabber"]`),
				Check: testAccCheckMetadataOptions(t, "Movie", map[string][]string{
					"DisabledMetadataFetchers": {},
					"MetadataFetcherOrder":     {"TheMovieDb", "The Open Movie Database"},
					"DisabledImageFetchers":    {},
					"ImageFetcherOrder":        {"TheMovieDb", "The Open Movie Database", "Embedded Image Extractor", "Screen Grabber"},
				}),
			},
			// Reordering a list reorders the server's order, and leaving a
			// fetcher out disables it.
			{
				Config: testAccSystemConfigurationFetchersConfig(`
    item_type         = "Movie"
    metadata_fetchers = ["The Open Movie Database"]
    image_fetchers    = ["Screen Grabber", "TheMovieDb"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue("jellyfin_system_configuration.test", movie.AtMapKey("disabled_metadata_fetchers")),
						plancheck.ExpectUnknownValue("jellyfin_system_configuration.test", movie.AtMapKey("metadata_fetcher_order")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.0.disabled_metadata_fetchers.0", "TheMovieDb"),
					testAccCheckMetadataOptions(t, "Movie", map[string][]string{
						"DisabledMetadataFetchers": {"TheMovieDb"},
						"MetadataFetcherOrder":     {"The Open Movie Database", "TheMovieDb"},
						"DisabledImageFetchers":    {"The Open Movie Database", "Embedded Image Extractor"},
						"ImageFetcherOrder":        {"Screen Grabber", "TheMovieDb", "The Open Movie Database", "Embedded Image Extractor"},
					}),
				),
			},
			{
				Config: testAccSystemConfigurationFetchersConfig(`
    item_type         = "Movie"
    metadata_fetchers = ["TheMovieDb", "The Open Movie Database"]
    image_fetchers    = ["Screen Grabber", "TheMovieDb"]`),
				Check: testAccCheckMetadataOptions(t, "Movie", map[string][]string{
					"DisabledMetadataFetchers": {},
					"MetadataFetcherOrder":     {"TheMovieDb", "The Open Movie Database"},
				}),
			},
			{
				ResourceName:            "jellyfin_system_configuration.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateId:           "system",
				ImportStateVerifyIgnore: []string{"server_name"},
			},
			{
				Config: testAccSystemConfigurationFetchersConfig(`
    item_type                  = "Movie"
    metadata_fetchers          = ["TheMovieDb"]
    disabled_metadata_fetchers = []`),
				ExpectError: regexp.MustCompile(`Attribute\s+"metadata_options\[0\].disabled_metadata_fetchers"\s+cannot\s+be\s+specified\s+when\s+"metadata_options\[0\].metadata_fetchers"\s+is\s+specified`),
			},
			{
				Config: testAccSystemConfigurationFetchersConfig(`
    item_type         = "Movie"
    metadata_fetchers = ["TheTVDB"]`),
				ExpectError: regexp.MustCompile(`metadata_options\[0\].metadata_fetchers\s+lists\s+"TheTVDB",\s+which\s+is\s+not\s+one\s+of\s+the\s+MetadataFetchers\s+the\s+Jellyfin\s+server\s+offers\s+for\s+Movie:\s+"TheMovieDb",\s+"The\s+Open\s+Movie\s+Database"`),
			},
			{
				Config: testAccSystemConfigurationFetchersConfig(`
    item_type         = "Person"
    metadata_fetchers = []`),
				ExpectError: regexp.MustCompile(`does\s+not\s+list\s+the\s+MetadataFetchers\s+it\s+offers\s+for\s+Person`),
			},
			// The attributes metadata_fetchers replaces still work alone.
			{
				Config: testAccSystemConfigurationFetchersConfig(`
    item_type                  = "Movie"
    disabled_metadata_fetchers = ["The Open Movie Database"]
    metadata_fetcher_order     = ["The Open Movie Database", "TheMovieDb"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue("jellyfin_system_configuration.test", movie.AtMapKey("metadata_fetchers")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.0.metadata_fetchers.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.0.metadata_fetchers.0", "TheMovieDb"),
					testAccCheckMetadataOptions(t, "Movie", map[string][]string{
						"DisabledMetadataFetchers": {"The Open Movie Database"},
						"MetadataFetcherOrder":     {"The Open Movie Database", "TheMovieDb"},
					}),
				),
			},
			// Jellyfin lists no fetchers for Person, so metadata_fetchers
			// reads as null for it.
			{
				Config: testAccSystemConfigurationFetchersConfig(`
    item_type                  = "Person"
    disabled_metadata_fetchers = ["TheMovieDb"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("jellyfin_system_configuration.test", "metadata_options.0.metadata_fetchers.#"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.0.disabled_metadata_fetchers.0", "TheMovieDb"),
				),
			},
		},
	})
}

// Terraform plans each metadata_options entry from the prior entry at the
// same index, which holds another item type once entries are added, removed
// or reordered.
func TestAccSystemConfigurationFetchersOfMovedEntries(t *testing.T) {
	testAccPreCheck(t)
	testAccPutBackSystemConfiguration(t)

	const (
		movedMovie   = `{ item_type = "Movie", metadata_fetchers = ["The Open Movie Database"], image_fetchers = ["Screen Grabber", "TheMovieDb"] }`
		movedEpisode = `{ item_type = "Episode", metadata_fetchers = ["TheMovieDb"] }`
	)
	series := tfjsonpath.New("metadata_options").AtSliceIndex(0)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// The state holds the server's default entries, Book first.
			{Config: testAccSystemConfigurationEntriesConfig()},
			{
				Config: testAccSystemConfigurationEntriesConfig(`{ item_type = "Movie", disabled_metadata_fetchers = ["The Open Movie Database"] }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.0.metadata_fetchers.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.0.metadata_fetchers.0", "TheMovieDb"),
					testAccCheckMetadataOptions(t, "Movie", map[string][]string{"DisabledMetadataFetchers": {"The Open Movie Database"}}),
				),
			},
			{
				Config: testAccSystemConfigurationEntriesConfig(
					`{ item_type = "BoxSet", metadata_fetchers = ["TheMovieDb"] }`,
					`{ item_type = "Movie", image_fetchers = ["Screen Grabber", "TheMovieDb"] }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.0.disabled_metadata_fetchers.#", "0"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.0.image_fetchers.0", "TheMovieDb"),
					testAccCheckMetadataOptions(t, "Movie", map[string][]string{
						"DisabledImageFetchers": {"The Open Movie Database", "Embedded Image Extractor"},
						"ImageFetcherOrder":     {"Screen Grabber", "TheMovieDb", "The Open Movie Database", "Embedded Image Extractor"},
					}),
				),
			},
			{
				Config: testAccSystemConfigurationEntriesConfig(
					`{ item_type = "Movie", disabled_metadata_fetchers = [], disabled_image_fetchers = [] }`,
					`{ item_type = "BoxSet", disabled_metadata_fetchers = [], disabled_image_fetchers = [] }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.0.item_type", "Movie"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.0.image_fetchers.#", "4"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.1.item_type", "BoxSet"),
					resource.TestCheckResourceAttr("jellyfin_system_configuration.test", "metadata_options.1.metadata_fetchers.0", "TheMovieDb"),
					testAccCheckMetadataOptions(t, "Movie", map[string][]string{"DisabledMetadataFetchers": {}, "DisabledImageFetchers": {}}),
				),
			},
			{
				Config: testAccSystemConfigurationEntriesConfig(movedMovie, `{ item_type = "Series" }`, movedEpisode),
				Check: testAccCheckMetadataOptions(t, "Series", map[string][]string{
					"DisabledMetadataFetchers": {}, "MetadataFetcherOrder": {}, "DisabledImageFetchers": {}, "ImageFetcherOrder": {},
				}),
			},
			// Series, which sets no lists, moves to Movie's index and keeps its
			// own lists, which the plan shows.
			{
				Config: testAccSystemConfigurationEntriesConfig(`{ item_type = "Series" }`, movedMovie, movedEpisode),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectKnownValue("jellyfin_system_configuration.test", series.AtMapKey("disabled_metadata_fetchers"), knownvalue.ListExact([]knownvalue.Check{})),
						plancheck.ExpectKnownValue("jellyfin_system_configuration.test", series.AtMapKey("image_fetcher_order"), knownvalue.ListExact([]knownvalue.Check{})),
						plancheck.ExpectKnownValue("jellyfin_system_configuration.test", series.AtMapKey("metadata_fetchers"), knownvalue.ListExact([]knownvalue.Check{
							knownvalue.StringExact("TheMovieDb"), knownvalue.StringExact("The Open Movie Database"),
						})),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckMetadataOptions(t, "Series", map[string][]string{
						"DisabledMetadataFetchers": {}, "MetadataFetcherOrder": {}, "DisabledImageFetchers": {}, "ImageFetcherOrder": {},
					}),
					testAccCheckMetadataOptions(t, "Movie", map[string][]string{
						"DisabledMetadataFetchers": {"TheMovieDb"},
						"MetadataFetcherOrder":     {"The Open Movie Database", "TheMovieDb"},
						"DisabledImageFetchers":    {"The Open Movie Database", "Embedded Image Extractor"},
						"ImageFetcherOrder":        {"Screen Grabber", "TheMovieDb", "The Open Movie Database", "Embedded Image Extractor"},
					}),
				),
			},
		},
	})
}

func testAccSystemConfigurationEntriesConfig(entries ...string) string {
	if len(entries) == 0 {
		return `
resource "jellyfin_system_configuration" "test" {}
`
	}
	return `
resource "jellyfin_system_configuration" "test" {
  metadata_options = [
    ` + strings.Join(entries, ",\n    ") + `,
  ]
}
`
}

func testAccSystemConfigurationFetchersConfig(entry string) string {
	return `
resource "jellyfin_system_configuration" "test" {
  metadata_options = [{
    ` + entry + `
  }]
}
`
}

// testAccCheckMetadataOptions checks string lists of the server's metadata
// options for itemType.
func testAccCheckMetadataOptions(t *testing.T, itemType string, want map[string][]string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		cfg, err := testAccClient(t).GetSystemConfiguration(context.Background())
		if err != nil {
			return err
		}
		var doc struct {
			MetadataOptions []map[string]json.RawMessage
		}
		if err := json.Unmarshal([]byte(cfg), &doc); err != nil {
			return fmt.Errorf("parsing the system configuration: %w", err)
		}
		entry := slices.IndexFunc(doc.MetadataOptions, func(e map[string]json.RawMessage) bool {
			var got string
			return json.Unmarshal(e["ItemType"], &got) == nil && got == itemType
		})
		if entry < 0 {
			return fmt.Errorf("the server has no %s metadata options", itemType)
		}
		for key, values := range want {
			var got []string
			if err := json.Unmarshal(doc.MetadataOptions[entry][key], &got); err != nil {
				return fmt.Errorf("parsing %s of %s: %w", key, itemType, err)
			}
			if !slices.Equal(got, values) {
				return fmt.Errorf("%s metadata options %s = %q on the server, want %q", itemType, key, got, values)
			}
		}
		return nil
	}
}
