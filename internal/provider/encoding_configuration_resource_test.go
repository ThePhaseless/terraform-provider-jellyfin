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
				Config: testAccEncodingConfigurationResourceConfig(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_encoding_configuration.test", "configuration_json"),
				),
			},
			// Update.
			{
				Config: testAccEncodingConfigurationResourceConfig(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_encoding_configuration.test", "configuration_json"),
				),
			},
		},
	})
}

func testAccEncodingConfigurationResourceConfig(enableFallbackFont bool) string {
	if enableFallbackFont {
		return `
resource "jellyfin_encoding_configuration" "test" {
  configuration_json = jsonencode({
    EncodingThreadCount                                        = -1
    EnableFallbackFont                                         = true
    EnableAudioVbr                                             = false
    DownMixAudioBoost                                          = 2
    DownMixStereoAlgorithm                                     = "None"
    MaxMuxingQueueSize                                         = 2048
    EnableThrottling                                           = false
    ThrottleDelaySeconds                                       = 180
    EnableSegmentDeletion                                      = false
    SegmentKeepSeconds                                         = 720
    HardwareAccelerationType                                   = "none"
    VaapiDevice                                                = "/dev/dri/renderD128"
    QsvDevice                                                  = ""
    EnableTonemapping                                          = false
    EnableVppTonemapping                                       = false
    EnableVideoToolboxTonemapping                              = false
    TonemappingAlgorithm                                       = "bt2390"
    TonemappingMode                                            = "auto"
    TonemappingRange                                           = "auto"
    TonemappingDesat                                           = 0
    TonemappingPeak                                            = 100
    TonemappingParam                                           = 0
    VppTonemappingBrightness                                   = 16
    VppTonemappingContrast                                     = 1
    H264Crf                                                    = 23
    H265Crf                                                    = 28
    DeinterlaceDoubleRate                                      = false
    DeinterlaceMethod                                          = "yadif"
    EnableDecodingColorDepth10Hevc                             = true
    EnableDecodingColorDepth10Vp9                              = true
    EnableDecodingColorDepth10HevcRext                         = false
    EnableDecodingColorDepth12HevcRext                         = false
    EnableEnhancedNvdecDecoder                                 = true
    PreferSystemNativeHwDecoder                                = true
    EnableIntelLowPowerH264HwEncoder                           = false
    EnableIntelLowPowerHevcHwEncoder                           = false
    EnableHardwareEncoding                                     = true
    AllowHevcEncoding                                          = false
    AllowAv1Encoding                                           = false
    EnableSubtitleExtraction                                   = true
    HardwareDecodingCodecs                                     = ["h264", "vc1"]
    AllowOnDemandMetadataBasedKeyframeExtractionForExtensions  = ["mkv"]
  })
}
`
	}
	return `
resource "jellyfin_encoding_configuration" "test" {
  configuration_json = jsonencode({
    EncodingThreadCount                                        = -1
    EnableFallbackFont                                         = false
    EnableAudioVbr                                             = false
    DownMixAudioBoost                                          = 2
    DownMixStereoAlgorithm                                     = "None"
    MaxMuxingQueueSize                                         = 2048
    EnableThrottling                                           = false
    ThrottleDelaySeconds                                       = 180
    EnableSegmentDeletion                                      = false
    SegmentKeepSeconds                                         = 720
    HardwareAccelerationType                                   = "none"
    VaapiDevice                                                = "/dev/dri/renderD128"
    QsvDevice                                                  = ""
    EnableTonemapping                                          = false
    EnableVppTonemapping                                       = false
    EnableVideoToolboxTonemapping                              = false
    TonemappingAlgorithm                                       = "bt2390"
    TonemappingMode                                            = "auto"
    TonemappingRange                                           = "auto"
    TonemappingDesat                                           = 0
    TonemappingPeak                                            = 100
    TonemappingParam                                           = 0
    VppTonemappingBrightness                                   = 16
    VppTonemappingContrast                                     = 1
    H264Crf                                                    = 23
    H265Crf                                                    = 28
    DeinterlaceDoubleRate                                      = false
    DeinterlaceMethod                                          = "yadif"
    EnableDecodingColorDepth10Hevc                             = true
    EnableDecodingColorDepth10Vp9                              = true
    EnableDecodingColorDepth10HevcRext                         = false
    EnableDecodingColorDepth12HevcRext                         = false
    EnableEnhancedNvdecDecoder                                 = true
    PreferSystemNativeHwDecoder                                = true
    EnableIntelLowPowerH264HwEncoder                           = false
    EnableIntelLowPowerHevcHwEncoder                           = false
    EnableHardwareEncoding                                     = true
    AllowHevcEncoding                                          = false
    AllowAv1Encoding                                           = false
    EnableSubtitleExtraction                                   = true
    HardwareDecodingCodecs                                     = ["h264", "vc1"]
    AllowOnDemandMetadataBasedKeyframeExtractionForExtensions  = ["mkv"]
  })
}
`
}
