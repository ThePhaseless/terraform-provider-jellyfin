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
			// Create and Read.
			{
				Config: `
resource "jellyfin_encoding_configuration" "test" {
  encoding_thread_count = -1
  enable_fallback_font = false
  enable_audio_vbr = false
  down_mix_audio_boost = 2
  down_mix_stereo_algorithm = "None"
  max_muxing_queue_size = 2048
  enable_throttling = false
  throttle_delay_seconds = 180
  enable_segment_deletion = false
  segment_keep_seconds = 720
  hardware_acceleration_type = "none"
  vaapi_device = "/dev/dri/renderD128"
  qsv_device = ""
  enable_tonemapping = false
  enable_vpp_tonemapping = false
  enable_video_toolbox_tonemapping = false
  tonemapping_algorithm = "bt2390"
  tonemapping_mode = "auto"
  tonemapping_range = "auto"
  tonemapping_desat = 0
  tonemapping_peak = 100
  tonemapping_param = 0
  vpp_tonemapping_brightness = 16
  vpp_tonemapping_contrast = 1
  h264_crf = 23
  h265_crf = 28
  encoder_preset = ""
  deinterlace_double_rate = false
  deinterlace_method = "yadif"
  enable_decoding_color_depth10_hevc = true
  enable_decoding_color_depth10_vp9 = true
  enable_decoding_color_depth10_hevc_rext = false
  enable_decoding_color_depth12_hevc_rext = false
  enable_enhanced_nvdec_decoder = true
  prefer_system_native_hw_decoder = true
  enable_intel_low_power_h264_hw_encoder = false
  enable_intel_low_power_hevc_hw_encoder = false
  enable_hardware_encoding = true
  allow_hevc_encoding = false
  allow_av1_encoding = false
  enable_subtitle_extraction = true
  hardware_decoding_codecs = ["h264", "vc1"]
  allow_on_demand_metadata_based_keyframe_extraction_for_extensions = ["mkv"]
  transcoding_temp_path = ""
  fallback_font_path = ""
  encoder_app_path = ""
  encoder_app_path_display = ""
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "encoding_thread_count", "-1"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_fallback_font", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_audio_vbr", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "down_mix_audio_boost", "2"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "down_mix_stereo_algorithm", "None"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "max_muxing_queue_size", "2048"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_throttling", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "throttle_delay_seconds", "180"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_segment_deletion", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "segment_keep_seconds", "720"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "hardware_acceleration_type", "none"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "vaapi_device", "/dev/dri/renderD128"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "qsv_device", ""),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_tonemapping", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_vpp_tonemapping", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_video_toolbox_tonemapping", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "tonemapping_algorithm", "bt2390"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "tonemapping_mode", "auto"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "tonemapping_range", "auto"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "tonemapping_desat", "0"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "tonemapping_peak", "100"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "tonemapping_param", "0"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "vpp_tonemapping_brightness", "16"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "vpp_tonemapping_contrast", "1"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "h264_crf", "23"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "h265_crf", "28"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "encoder_preset", ""),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "deinterlace_double_rate", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "deinterlace_method", "yadif"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_decoding_color_depth10_hevc", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_decoding_color_depth10_vp9", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_decoding_color_depth10_hevc_rext", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_decoding_color_depth12_hevc_rext", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_enhanced_nvdec_decoder", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "prefer_system_native_hw_decoder", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_intel_low_power_h264_hw_encoder", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_intel_low_power_hevc_hw_encoder", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_hardware_encoding", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "allow_hevc_encoding", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "allow_av1_encoding", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_subtitle_extraction", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "hardware_decoding_codecs.#", "2"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "allow_on_demand_metadata_based_keyframe_extraction_for_extensions.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "transcoding_temp_path", ""),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "fallback_font_path", ""),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "encoder_app_path", ""),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "encoder_app_path_display", ""),
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_encoding_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "encoding",
			},
			// Update.
			{
				Config: `
resource "jellyfin_encoding_configuration" "test" {
  encoding_thread_count = -1
  enable_fallback_font = true
  enable_audio_vbr = false
  down_mix_audio_boost = 2
  down_mix_stereo_algorithm = "None"
  max_muxing_queue_size = 2048
  enable_throttling = false
  throttle_delay_seconds = 180
  enable_segment_deletion = false
  segment_keep_seconds = 720
  hardware_acceleration_type = "none"
  vaapi_device = "/dev/dri/renderD128"
  qsv_device = ""
  enable_tonemapping = false
  enable_vpp_tonemapping = false
  enable_video_toolbox_tonemapping = false
  tonemapping_algorithm = "bt2390"
  tonemapping_mode = "auto"
  tonemapping_range = "auto"
  tonemapping_desat = 0
  tonemapping_peak = 100
  tonemapping_param = 0
  vpp_tonemapping_brightness = 16
  vpp_tonemapping_contrast = 1
  h264_crf = 23
  h265_crf = 28
  encoder_preset = ""
  deinterlace_double_rate = false
  deinterlace_method = "yadif"
  enable_decoding_color_depth10_hevc = true
  enable_decoding_color_depth10_vp9 = true
  enable_decoding_color_depth10_hevc_rext = false
  enable_decoding_color_depth12_hevc_rext = false
  enable_enhanced_nvdec_decoder = true
  prefer_system_native_hw_decoder = true
  enable_intel_low_power_h264_hw_encoder = false
  enable_intel_low_power_hevc_hw_encoder = false
  enable_hardware_encoding = true
  allow_hevc_encoding = false
  allow_av1_encoding = false
  enable_subtitle_extraction = true
  hardware_decoding_codecs = ["h264", "vc1"]
  allow_on_demand_metadata_based_keyframe_extraction_for_extensions = ["mkv"]
  transcoding_temp_path = ""
  fallback_font_path = ""
  encoder_app_path = ""
  encoder_app_path_display = ""
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "encoding_thread_count", "-1"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_fallback_font", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_audio_vbr", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "down_mix_audio_boost", "2"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "down_mix_stereo_algorithm", "None"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "max_muxing_queue_size", "2048"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_throttling", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "throttle_delay_seconds", "180"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_segment_deletion", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "segment_keep_seconds", "720"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "hardware_acceleration_type", "none"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "vaapi_device", "/dev/dri/renderD128"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "qsv_device", ""),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_tonemapping", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_vpp_tonemapping", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_video_toolbox_tonemapping", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "tonemapping_algorithm", "bt2390"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "tonemapping_mode", "auto"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "tonemapping_range", "auto"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "tonemapping_desat", "0"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "tonemapping_peak", "100"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "tonemapping_param", "0"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "vpp_tonemapping_brightness", "16"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "vpp_tonemapping_contrast", "1"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "h264_crf", "23"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "h265_crf", "28"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "encoder_preset", ""),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "deinterlace_double_rate", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "deinterlace_method", "yadif"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_decoding_color_depth10_hevc", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_decoding_color_depth10_vp9", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_decoding_color_depth10_hevc_rext", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_decoding_color_depth12_hevc_rext", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_enhanced_nvdec_decoder", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "prefer_system_native_hw_decoder", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_intel_low_power_h264_hw_encoder", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_intel_low_power_hevc_hw_encoder", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_hardware_encoding", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "allow_hevc_encoding", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "allow_av1_encoding", "false"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "enable_subtitle_extraction", "true"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "hardware_decoding_codecs.#", "2"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "allow_on_demand_metadata_based_keyframe_extraction_for_extensions.#", "1"),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "transcoding_temp_path", ""),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "fallback_font_path", ""),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "encoder_app_path", ""),
					resource.TestCheckResourceAttr("jellyfin_encoding_configuration.test", "encoder_app_path_display", ""),
				),
			},
		},
	})
}
