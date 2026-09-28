// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccEncodingConfigurationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			testAccAttrsStep("jellyfin_encoding_configuration", testAccEncodingAttrs(false, 45, "TranscodeAudio")),
			{
				ResourceName:      "jellyfin_encoding_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "encoding",
			},
			testAccAttrsStep("jellyfin_encoding_configuration", testAccEncodingAttrs(true, 30, "TrimCopiedAudio")),
		},
	})
}

func testAccEncodingAttrs(enableFallbackFont bool, subtitleExtractionTimeoutMinutes int, hlsAudioSeekStrategy string) []testAccAttr {
	return []testAccAttr{
		{"encoding_thread_count", -1},
		{"enable_fallback_font", enableFallbackFont},
		{"enable_audio_vbr", false},
		{"down_mix_audio_boost", 2},
		{"down_mix_stereo_algorithm", "None"},
		{"max_muxing_queue_size", 2048},
		{"enable_throttling", false},
		{"throttle_delay_seconds", 180},
		{"enable_segment_deletion", false},
		{"segment_keep_seconds", 720},
		{"hardware_acceleration_type", "none"},
		{"vaapi_device", "/dev/dri/renderD128"},
		{"qsv_device", ""},
		{"enable_tonemapping", false},
		{"enable_vpp_tonemapping", false},
		{"enable_video_toolbox_tonemapping", false},
		{"tonemapping_algorithm", "bt2390"},
		{"tonemapping_mode", "auto"},
		{"tonemapping_range", "auto"},
		{"tonemapping_desat", 0},
		{"tonemapping_peak", 100},
		{"tonemapping_param", 0},
		{"vpp_tonemapping_brightness", 16},
		{"vpp_tonemapping_contrast", 1},
		{"h264_crf", 23},
		{"h265_crf", 28},
		{"encoder_preset", "auto"},
		{"deinterlace_double_rate", false},
		{"deinterlace_method", "yadif"},
		{"enable_decoding_color_depth10_hevc", true},
		{"enable_decoding_color_depth10_vp9", true},
		{"enable_decoding_color_depth10_hevc_rext", false},
		{"enable_decoding_color_depth12_hevc_rext", false},
		{"enable_enhanced_nvdec_decoder", true},
		{"prefer_system_native_hw_decoder", true},
		{"enable_intel_low_power_h264_hw_encoder", false},
		{"enable_intel_low_power_hevc_hw_encoder", false},
		{"enable_hardware_encoding", true},
		{"allow_hevc_encoding", false},
		{"allow_av1_encoding", false},
		{"enable_subtitle_extraction", true},
		{"subtitle_extraction_timeout_minutes", subtitleExtractionTimeoutMinutes},
		{"hardware_decoding_codecs", []string{"h264", "vc1"}},
		{"allow_on_demand_metadata_based_keyframe_extraction_for_extensions", []string{"mkv"}},
		{"hls_audio_seek_strategy", hlsAudioSeekStrategy},
		{"transcoding_temp_path", ""},
		{"fallback_font_path", ""},
		{"encoder_app_path", ""},
		{"encoder_app_path_display", ""},
	}
}
