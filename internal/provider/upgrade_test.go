// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

var v038 = map[string]resource.ExternalProvider{
	"jellyfin": {Source: providerSource, VersionConstraint: "0.3.8"},
}

const providerSource = "ThePhaseless/jellyfin"

// testAccUpgradeFromV038 applies config with 0.3.8, expects this provider to
// plan no change, then runs then.
func testAccUpgradeFromV038(t *testing.T, config string, then ...resource.TestStep) {
	t.Helper()

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
	for _, step := range then {
		step.ProtoV6ProviderFactories = testAccProtoV6ProviderFactories
		steps = append(steps, step)
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps:    steps,
	})
}

// testAccPutBackServerConfiguration posts back, once the test ends, the
// configuration documents and the triggers of scheduled task taskID as the
// server serves them now.
func testAccPutBackServerConfiguration(t *testing.T, taskID string) {
	t.Helper()

	c, ctx := testAccClient(t), t.Context()
	system, errSystem := c.GetSystemConfiguration(ctx)
	network, errNetwork := c.GetNetworkConfiguration(ctx)
	encoding, errEncoding := c.GetEncodingOptions(ctx)
	branding, errBranding := c.GetBrandingConfiguration(ctx)
	metadata, errMetadata := c.GetMetadataConfiguration(ctx)
	livetv, errLivetv := c.GetLiveTVConfiguration(ctx)
	task, errTask := c.GetScheduledTask(ctx, taskID)
	if err := errors.Join(errSystem, errNetwork, errEncoding, errBranding, errMetadata, errLivetv, errTask); err != nil {
		t.Fatalf("reading the server configuration: %v", err)
	}
	triggers := []byte("[]")
	if task.Triggers != nil {
		var err error
		if triggers, err = json.Marshal(task.Triggers); err != nil {
			t.Fatalf("encoding the triggers of task %s: %v", taskID, err)
		}
	}

	t.Cleanup(func() {
		// t.Context() is done by now, and the provider's sign-in has signed c
		// out, since both share a device ID.
		c, ctx := testAccClient(t), context.WithoutCancel(t.Context())
		if err := errors.Join(
			c.UpdateSystemConfiguration(ctx, system),
			c.UpdateNetworkConfiguration(ctx, network),
			c.UpdateEncodingOptions(ctx, encoding),
			c.UpdateBrandingConfiguration(ctx, branding),
			c.UpdateMetadataConfiguration(ctx, metadata),
			c.UpdateLiveTVConfiguration(ctx, livetv),
			c.UpdateScheduledTaskTriggers(ctx, taskID, string(triggers)),
		); err != nil {
			t.Errorf("putting back the server configuration: %v", err)
		}
	})
}

func TestAccUpgradeFromV038(t *testing.T) {
	testAccPreCheck(t)
	jellyfin12 := testAccJellyfin12OrLater(t)
	const taskID = "7738148ffcd07979c7ceb148e06b3aed"
	testAccPutBackServerConfiguration(t, taskID)

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
		name, config string
	}{
		{name: "branding", config: `
resource "jellyfin_branding_configuration" "test" {
  login_disclaimer     = "tf-acc upgrade"
  custom_css           = ".skinHeader { opacity: 0.9; }"
  splashscreen_enabled = true
}
`},
		{name: "metadata", config: `
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
  is_remote_ip_filter_blacklist  = true
  enable_upnp                    = false
  ignore_virtual_interfaces      = true
  virtual_interface_names        = ["veth", "docker"]
  published_server_uri_by_subnet = []
}
`},
		{name: "system", config: `
resource "jellyfin_system_configuration" "test" {
  server_name                        = "tf-acc-upgrade"
  enable_normalized_item_by_name_ids = false
  enable_case_sensitive_item_ids     = false
  image_saving_convention            = "Compatible"
  chapter_image_resolution           = "P720"
  sort_remove_words                  = ["the", "a"]

  trickplay_options = {
    scan_behavior     = "Blocking"
    process_priority  = "Idle"
    interval          = 5000
    width_resolutions = [320, 640]
  }

  cast_receiver_applications = [{ id = "F007D354", name = "Stable" }]
  path_substitutions         = [{ from = "/mnt/upgrade", to = "/media/upgrade" }]

  metadata_options = [{
    item_type                  = "Movie"
    disabled_metadata_fetchers = ["The Open Movie Database"]
    metadata_fetcher_order     = ["The Open Movie Database", "TheMovieDb"]
    disabled_image_fetchers    = []
    image_fetcher_order        = ["TheMovieDb"]
  }]
}
`},
		{name: "livetv", config: `
resource "jellyfin_livetv_configuration" "test" {
  guide_days                         = 7
  recording_path                     = "/config/tf-acc-upgrade-recordings"
  pre_padding_seconds                = 60
  recording_post_processor_arguments = "-i \"{path}\""

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
`},
		{name: "scheduled_task", config: `
resource "jellyfin_scheduled_task" "test" {
  task_id = "` + taskID + `"

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
    disabled_subtitle_fetchers           = ["Open Subtitles"]
    subtitle_fetcher_order               = ["Open Subtitles", "Podnapisi"]
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
			testAccUpgradeFromV038(t, c.config)
		})
	}
}

// This provider plans a metadata_options entry inserted ahead of those 0.3.8
// applied from the entry it displaces, which holds another item type.
func TestAccUpgradeFromV038ThenInsertMetadataOptions(t *testing.T) {
	testAccPreCheck(t)
	testAccPutBackSystemConfiguration(t)

	movie := `{ item_type = "Movie", disabled_metadata_fetchers = ["The Open Movie Database"], metadata_fetcher_order = ["The Open Movie Database", "TheMovieDb"] }`
	testAccUpgradeFromV038(t, testAccSystemConfigurationEntriesConfig(movie), resource.TestStep{
		Config: testAccSystemConfigurationEntriesConfig(`{ item_type = "BoxSet", disabled_metadata_fetchers = [] }`, movie),
		Check: testAccCheckMetadataOptions(t, "Movie", map[string][]string{
			"DisabledMetadataFetchers": {"The Open Movie Database"},
			"MetadataFetcherOrder":     {"The Open Movie Database", "TheMovieDb"},
		}),
	})
}

// JellyfinSecurity loads only after a restart, so this runs with the restart
// tests rather than in TestAccUpgradeFromV038.
func TestAccSecurityPluginUpgradeFromV038(t *testing.T) {
	testAccSecurityPluginPreCheck(t)
	c := testAccInstallSecurityPlugin(t)
	original, err := c.GetPluginConfiguration(t.Context(), jellyfinSecurityPluginID)
	if err != nil {
		t.Fatalf("reading the JellyfinSecurity configuration: %v", err)
	}
	t.Cleanup(func() {
		if err := testAccClient(t).UpdatePluginConfiguration(context.WithoutCancel(t.Context()), jellyfinSecurityPluginID, original); err != nil {
			t.Errorf("putting back the JellyfinSecurity configuration: %v", err)
		}
	})

	testAccUpgradeFromV038(t, testAccSecurityPluginConfigurationConfig(securityPluginTestValues{
		pluginID:           jellyfinSecurityPluginID,
		publicBaseURL:      "https://upgrade.example.com",
		pairDevice:         true,
		stepUpWindow:       900,
		enrollmentDeadline: "2032-03-04T05:06:07Z",
		displayName:        "Upgrade IdP",
		linkByUsername:     true,
	}))
}
