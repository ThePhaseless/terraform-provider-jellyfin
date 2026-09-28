// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccLiveTVConfigurationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_livetv_configuration" "test" {
  guide_days                                    = 7
  recording_path                                = "/config/tf-acc-recordings"
  enable_recording_subfolders                   = false
  enable_original_audio_with_encoded_recordings = false
  pre_padding_seconds                           = 0
  post_padding_seconds                          = 0
  recording_post_processor_arguments            = "\"{path}\""
  save_recording_nfo                            = true
  save_recording_images                         = true

  tuner_hosts = [{
    type                 = "m3u"
    url                  = "http://127.0.0.1:9/tf-acc.m3u"
    friendly_name        = "tf-acc tuner"
    tuner_count          = 2
    allow_hw_transcoding = false
  }]

  listing_providers = [{
    type              = "xmltv"
    path              = "/config/tf-acc-guide.xml"
    enable_all_tuners = true
    news_categories   = ["news"]
    channel_mappings = [{
      name  = "1"
      value = "one"
    }]
  }]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "id", "livetv"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "guide_days", "7"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "recording_path", "/config/tf-acc-recordings"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "enable_recording_subfolders", "false"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "enable_original_audio_with_encoded_recordings", "false"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "pre_padding_seconds", "0"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "post_padding_seconds", "0"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "recording_post_processor_arguments", "\"{path}\""),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "save_recording_nfo", "true"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "save_recording_images", "true"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.type", "m3u"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.url", "http://127.0.0.1:9/tf-acc.m3u"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.friendly_name", "tf-acc tuner"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.tuner_count", "2"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.allow_hw_transcoding", "false"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.type", "xmltv"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.path", "/config/tf-acc-guide.xml"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.enable_all_tuners", "true"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.news_categories.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.news_categories.0", "news"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.channel_mappings.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.channel_mappings.0.name", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.channel_mappings.0.value", "one"),
				),
			},
			{
				ResourceName:      "jellyfin_livetv_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "livetv",
			},
			// Update: insert a tuner host ahead of the existing one, which moves to
			// index 1 and keeps the settings its config no longer sets; move the
			// listing provider's guide file, which keeps its news categories.
			{
				Config: `
resource "jellyfin_livetv_configuration" "test" {
  guide_days                                    = 14
  recording_path                                = "/config/tf-acc-recordings"
  enable_recording_subfolders                   = true
  enable_original_audio_with_encoded_recordings = false
  pre_padding_seconds                           = 120
  post_padding_seconds                          = 300
  recording_post_processor_arguments            = "\"{path}\""
  save_recording_nfo                            = false
  save_recording_images                         = true

  tuner_hosts = [
    {
      type = "m3u"
      url  = "http://127.0.0.1:9/tf-acc-second.m3u"
    },
    {
      type          = "m3u"
      url           = "http://127.0.0.1:9/tf-acc.m3u"
      friendly_name = "tf-acc tuner renamed"
    },
  ]

  listing_providers = [{
    type              = "xmltv"
    path              = "/config/tf-acc-guide-moved.xml"
    enable_all_tuners = true
    channel_mappings = [
      {
        name  = "1"
        value = "one"
      },
      {
        name  = "2"
        value = "two"
      },
    ]
  }]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "guide_days", "14"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "enable_recording_subfolders", "true"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "pre_padding_seconds", "120"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "post_padding_seconds", "300"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "save_recording_nfo", "false"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.#", "2"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.url", "http://127.0.0.1:9/tf-acc-second.m3u"),
					resource.TestCheckNoResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.friendly_name"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.tuner_count", "0"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.1.url", "http://127.0.0.1:9/tf-acc.m3u"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.1.friendly_name", "tf-acc tuner renamed"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.1.tuner_count", "2"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.1.allow_hw_transcoding", "false"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.path", "/config/tf-acc-guide-moved.xml"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.news_categories.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.news_categories.0", "news"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.channel_mappings.#", "2"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.channel_mappings.1.name", "2"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.channel_mappings.1.value", "two"),
				),
			},
			// Update: clear the tuner hosts and listing providers again.
			{
				Config: `
resource "jellyfin_livetv_configuration" "test" {
  guide_days        = 14
  tuner_hosts       = []
  listing_providers = []
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.#", "0"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.#", "0"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "pre_padding_seconds", "120"),
				),
			},
			// Omitted attributes keep the server's values, so re-planning an empty block changes nothing.
			{
				Config: `
resource "jellyfin_livetv_configuration" "test" {}
`,
				PlanOnly: true,
			},
		},
	})
}

func TestAccLiveTVConfigurationResourceExistingRecordingPath(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// A missing directory is stored without adding a library.
			{
				Config: `
resource "jellyfin_livetv_configuration" "test" {
  recording_path = "/config/tf-acc-missing-recordings"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "recording_path", "/config/tf-acc-missing-recordings"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "media_locations_created.#", "0"),
				),
			},
			// An existing directory makes Jellyfin add a Recordings library and
			// rewrite media_locations_created while the apply reads the result back.
			{
				Config: `
resource "jellyfin_livetv_configuration" "test" {
  recording_path = "/tmp"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "recording_path", "/tmp"),
					testAccWaitForLiveTVMediaLocations(t, "/tmp"),
				),
			},
			{
				RefreshState: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "media_locations_created.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "media_locations_created.0", "/tmp"),
				),
			},
			// Pointing back at a missing directory makes Jellyfin remove that library again.
			{
				Config: `
resource "jellyfin_livetv_configuration" "test" {
  recording_path = "/config/tf-acc-missing-recordings"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "recording_path", "/config/tf-acc-missing-recordings"),
					testAccWaitForLiveTVMediaLocations(t),
				),
			},
			{
				RefreshState: true,
				Check:        resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "media_locations_created.#", "0"),
			},
		},
	})
}

// Jellyfin rewrites MediaLocationsCreated asynchronously after the POST
// returns, so a refresh right after apply may read the old list.
func testAccWaitForLiveTVMediaLocations(t *testing.T, want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		const timeout = time.Minute
		c := testAccClient(t)
		deadline := time.Now().Add(timeout)
		for {
			current, err := c.GetLiveTVConfiguration(context.Background())
			if err != nil {
				return err
			}
			var cfg struct {
				MediaLocationsCreated []string
			}
			if err := json.Unmarshal([]byte(current), &cfg); err != nil {
				return fmt.Errorf("parsing Live TV configuration: %w", err)
			}
			if slices.Equal(cfg.MediaLocationsCreated, want) {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("MediaLocationsCreated = %q after %s, want %q", cfg.MediaLocationsCreated, timeout, want)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func TestAccLiveTVConfigurationResourceUnknownEntryValues(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "terraform_data" "entry" {
  input = "a"
}

resource "jellyfin_livetv_configuration" "test" {
  tuner_hosts = [
    {
      type        = "m3u"
      url         = "http://127.0.0.1:9/tf-acc-a.m3u"
      tuner_count = 2
    },
    {
      type        = "m3u"
      url         = "http://127.0.0.1:9/tf-acc-b.m3u"
      tuner_count = 3
    },
  ]

  listing_providers = [{
    type        = "SchedulesDirect"
    username    = "tf-acc"
    listings_id = "USA-123"
    password    = terraform_data.entry.output
  }]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.#", "2"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.password", "a"),
				),
			},
			// The remaining tuner's url and the password are unknown until
			// terraform_data applies. The url resolves to the second tuner's, so the
			// entry keeps that tuner's tuner_count rather than the first tuner's,
			// which its type alone would match.
			{
				Config: `
resource "terraform_data" "entry" {
  input = "b"
}

resource "jellyfin_livetv_configuration" "test" {
  tuner_hosts = [{
    type = "m3u"
    url  = "http://127.0.0.1:9/tf-acc-${terraform_data.entry.output}.m3u"
  }]

  listing_providers = [{
    type        = "SchedulesDirect"
    username    = "tf-acc"
    listings_id = "USA-123"
    password    = terraform_data.entry.output
  }]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.url", "http://127.0.0.1:9/tf-acc-b.m3u"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.tuner_count", "3"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.listings_id", "USA-123"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.password", "b"),
				),
			},
			// Clear the lists so the shared test server is left without the tuner
			// and the Schedules Direct credentials.
			{
				Config: `
resource "jellyfin_livetv_configuration" "test" {
  tuner_hosts       = []
  listing_providers = []
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.#", "0"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.#", "0"),
				),
			},
		},
	})
}

func TestAccLiveTVConfigurationResourceDuplicateTunerURLs(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_livetv_configuration" "test" {
  tuner_hosts = [
    {
      type          = "m3u"
      url           = "http://127.0.0.1:9/tf-acc-dup.m3u"
      friendly_name = "first"
      tuner_count   = 2
    },
    {
      type          = "m3u"
      url           = "http://127.0.0.1:9/tf-acc-dup.m3u"
      friendly_name = "second"
      tuner_count   = 3
    },
  ]
}
`,
				Check: resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.#", "2"),
			},
			// The second entry's url matches both tuners and the first entry
			// matches either by type. The name resolves to the first tuner's, so
			// the final plan pairs the entries with the tuners by index; the plan
			// has to pair them the same way or the apply fails as inconsistent.
			{
				Config: `
resource "terraform_data" "name" {
  input = "first"
}

resource "jellyfin_livetv_configuration" "test" {
  tuner_hosts = [
    {
      type          = "m3u"
      friendly_name = terraform_data.name.output
    },
    {
      type = "m3u"
      url  = "http://127.0.0.1:9/tf-acc-dup.m3u"
    },
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.#", "2"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.url", "http://127.0.0.1:9/tf-acc-dup.m3u"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.friendly_name", "first"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.tuner_count", "2"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.1.friendly_name", "second"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.1.tuner_count", "3"),
				),
			},
			// Clear the list so the shared test server is left without the tuners.
			{
				Config: `
resource "jellyfin_livetv_configuration" "test" {
  tuner_hosts = []
}
`,
				Check: resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.#", "0"),
			},
		},
	})
}
