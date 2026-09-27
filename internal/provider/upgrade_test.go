// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// v038 is the last release that spelled every Jellyfin key by hand, before
// the resources read and wrote through their wire bindings.
var v038 = map[string]resource.ExternalProvider{
	"jellyfin": {Source: providerSource, VersionConstraint: "0.3.8"},
}

const providerSource = "ThePhaseless/jellyfin"

// testAccUpgradeFromV038 applies config with provider 0.3.8 and then plans it
// with this provider, which must find nothing to change in the state 0.3.8
// saved or in what 0.3.8 sent Jellyfin. cleanup, when set, is applied last to
// put back what later tests expect.
func testAccUpgradeFromV038(t *testing.T, config, cleanup string) {
	t.Helper()

	// The state 0.3.8 saves names the registry address; the in-process
	// provider must answer to the same one for Terraform to plan it.
	namespace, _, _ := strings.Cut(providerSource, "/")
	t.Setenv(resource.EnvTfAccProviderNamespace, namespace)
	steps := []resource.TestStep{
		{
			ExternalProviders: v038,
			Config:            config,
		},
		{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Config:                   config,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
			},
		},
	}
	if cleanup != "" {
		steps = append(steps, resource.TestStep{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Config:                   cleanup,
		})
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps:    steps,
	})
}

func TestAccUpgradeFromV038(t *testing.T) {
	testAccPreCheck(t)
	jellyfin12 := testAccJellyfinVersionAtLeast(t, "12")

	encoding12, similarItems := "", ""
	if jellyfin12 {
		encoding12 = `
  subtitle_extraction_timeout_minutes = 45
  hls_audio_seek_strategy             = "TranscodeAudio"`
		similarItems = `
        similar_item_providers      = ["Local Genre/Tag"]
        similar_item_provider_order = ["Local Genre/Tag", "TheMovieDb"]`
	}

	for _, c := range []struct {
		name, config, cleanup string
	}{
		{name: "branding", config: `
resource "jellyfin_branding_configuration" "test" {
  login_disclaimer     = "tf-acc upgrade"
  custom_css           = ".skinHeader { opacity: 0.9; }"
  splashscreen_enabled = false
}
`},
		{name: "metadata", config: `
resource "jellyfin_metadata_configuration" "test" {
  use_file_creation_time_for_date_added = true
}
`, cleanup: `
resource "jellyfin_metadata_configuration" "test" {
  use_file_creation_time_for_date_added = false
}
`},
		{name: "encoding", config: `
resource "jellyfin_encoding_configuration" "test" {
  encoding_thread_count  = 2
  down_mix_audio_boost   = 1.5
  tonemapping_desat      = 0.25
  enable_throttling      = true
  throttle_delay_seconds = 120
  h264_crf               = 21
  hardware_decoding_codecs = ["h264", "hevc"]
  allow_on_demand_metadata_based_keyframe_extraction_for_extensions = ["mkv", "mp4"]` + encoding12 + `
}
`},
		// Only settings that leave the server reachable at its address.
		{name: "networking", config: `
resource "jellyfin_networking_configuration" "test" {
  known_proxies                  = ["10.1.2.3"]
  remote_ip_filter               = []
  is_remote_ip_filter_blacklist  = false
  enable_upnp                    = false
  ignore_virtual_interfaces      = true
  virtual_interface_names        = ["veth", "docker"]
  published_server_uri_by_subnet = []
}
`},
		{name: "system", config: `
resource "jellyfin_system_configuration" "test" {
  server_name                        = "tf-acc-upgrade"
  enable_normalized_item_by_name_ids = true
  enable_case_sensitive_item_ids     = true
  image_saving_convention            = "Legacy"
  chapter_image_resolution           = "MatchSource"
  sort_remove_words                  = ["the", "a", "an"]

  trickplay_options = {
    scan_behavior     = "NonBlocking"
    process_priority  = "BelowNormal"
    interval          = 10000
    width_resolutions = [320]
  }

  cast_receiver_applications = [{ id = "F007D354", name = "Stable" }, { id = "6F511C87", name = "Unstable" }]
  path_substitutions         = [{ from = "/mnt/upgrade", to = "/media/upgrade" }]
}
`, cleanup: `
resource "jellyfin_system_configuration" "test" {
  path_substitutions = []
}
`},
		{name: "livetv", config: `
resource "jellyfin_livetv_configuration" "test" {
  guide_days                         = 7
  recording_path                     = "/config/tf-acc-upgrade-recordings"
  pre_padding_seconds                = 60
  recording_post_processor_arguments = "\"{path}\""

  tuner_hosts = [{
    type                 = "m3u"
    url                  = "http://127.0.0.1:9/tf-acc-upgrade.m3u"
    friendly_name        = "tf-acc upgrade tuner"
    tuner_count          = 2
    allow_hw_transcoding = false
  }]

  listing_providers = [{
    type              = "xmltv"
    path              = "/config/tf-acc-upgrade-guide.xml"
    enable_all_tuners = true
    news_categories   = ["news"]
    channel_mappings  = [{ name = "1", value = "one" }]
  }]
}
`, cleanup: `
resource "jellyfin_livetv_configuration" "test" {
  tuner_hosts       = []
  listing_providers = []
}
`},
		{name: "scheduled_task", config: `
resource "jellyfin_scheduled_task" "test" {
  task_id = "7738148ffcd07979c7ceb148e06b3aed"

  triggers = [
    {
      type              = "WeeklyTrigger"
      day_of_week       = "Tuesday"
      time_of_day_ticks = 36000000000
    },
    {
      type              = "DailyTrigger"
      time_of_day_ticks = 72000000000
      max_runtime_ticks = 144000000000
    },
    {
      type           = "IntervalTrigger"
      interval_ticks = 864000000000
    }
  ]
}
`},
		{name: "user", config: `
resource "jellyfin_user" "test" {
  name               = "tf-acc-upgrade"
  password           = "upgradepass123"
  enable_all_folders = true

  policy = {
    max_active_sessions  = 2
    max_parental_rating  = 13
    blocked_tags         = ["tf-acc"]
    enable_remote_access = false
    sync_play_access     = "JoinGroups"
    access_schedules     = [{ day_of_week = "Monday", start_hour = 8, end_hour = 20.5 }]
  }
}
`},
		{name: "library", config: `
resource "jellyfin_library" "test" {
  name            = "TfAccUpgrade"
  collection_type = "movies"
  paths           = ["/media/movies"]

  library_options = {
    enable_realtime_monitor              = false
    save_local_metadata                  = false
    extract_chapters_during_library_scan = true
    disabled                             = false
    path_infos                           = [{ path = "/media/movies" }]
    type_options = [
      {
        type                   = "Movie"
        metadata_fetchers      = ["TheMovieDb"]
        metadata_fetcher_order = ["TheMovieDb", "The Open Movie Database"]
        image_fetchers         = ["TheMovieDb"]
        image_fetcher_order    = ["TheMovieDb"]
        image_options          = [{ type = "Backdrop", limit = 1, min_width = 1280 }]` + similarItems + `
      }
    ]
  }
}
`},
	} {
		t.Run(c.name, func(t *testing.T) {
			testAccUpgradeFromV038(t, c.config, c.cleanup)
		})
	}
}

// JellyfinSecurity loads only after a restart, so this runs with the restart
// tests rather than in TestAccUpgradeFromV038.
func TestAccSecurityPluginUpgradeFromV038(t *testing.T) {
	testAccSecurityPluginPreCheck(t)
	testAccInstallSecurityPlugin(t)

	testAccUpgradeFromV038(t, testAccSecurityPluginConfigurationConfig(securityPluginTestValues{
		pluginID:           jellyfinSecurityPluginID,
		publicBaseURL:      "https://upgrade.example.com",
		pairDevice:         true,
		stepUpWindow:       900,
		enrollmentDeadline: "2032-03-04T05:06:07Z",
		displayName:        "Upgrade IdP",
		linkByUsername:     true,
	}), "")
}
