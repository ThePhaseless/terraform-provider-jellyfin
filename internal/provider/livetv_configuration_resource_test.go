// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccLiveTVConfigurationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read.
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
    type          = "m3u"
    url           = "http://127.0.0.1:9/tf-acc.m3u"
    friendly_name = "tf-acc tuner"
  }]

  listing_providers = [{
    type              = "xmltv"
    path              = "/config/tf-acc-guide.xml"
    enable_all_tuners = true
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
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.type", "xmltv"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.path", "/config/tf-acc-guide.xml"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.enable_all_tuners", "true"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.channel_mappings.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.channel_mappings.0.name", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "listing_providers.0.channel_mappings.0.value", "one"),
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_livetv_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "livetv",
			},
			// Update.
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

  tuner_hosts = [{
    type          = "m3u"
    url           = "http://127.0.0.1:9/tf-acc.m3u"
    friendly_name = "tf-acc tuner renamed"
  }]

  listing_providers = [{
    type              = "xmltv"
    path              = "/config/tf-acc-guide.xml"
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
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "tuner_hosts.0.friendly_name", "tf-acc tuner renamed"),
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
				Check: resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "recording_path", "/tmp"),
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
				Check: resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "recording_path", "/config/tf-acc-missing-recordings"),
			},
			{
				RefreshState: true,
				Check:        resource.TestCheckResourceAttr("jellyfin_livetv_configuration.test", "media_locations_created.#", "0"),
			},
		},
	})
}
