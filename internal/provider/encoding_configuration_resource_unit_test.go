// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnitEncodingConfigurationRoundTrip(t *testing.T) {
	fixture := `{"EncodingThreadCount":-1,"TranscodingTempPath":"/tmp","FallbackFontPath":"/fonts","EnableFallbackFont":false,"EnableAudioVbr":false,"DownMixAudioBoost":2,"DownMixStereoAlgorithm":"None","MaxMuxingQueueSize":2048,"EnableThrottling":false,"ThrottleDelaySeconds":180,"EnableSegmentDeletion":false,"SegmentKeepSeconds":720,"HardwareAccelerationType":"none","EncoderAppPath":"","EncoderAppPathDisplay":"","VaapiDevice":"/dev/dri/renderD128","QsvDevice":"","EnableTonemapping":false,"EnableVppTonemapping":false,"EnableVideoToolboxTonemapping":false,"TonemappingAlgorithm":"bt2390","TonemappingMode":"auto","TonemappingRange":"auto","TonemappingDesat":0,"TonemappingPeak":100,"TonemappingParam":0,"VppTonemappingBrightness":16,"VppTonemappingContrast":1,"H264Crf":23,"H265Crf":28,"EncoderPreset":"auto","DeinterlaceDoubleRate":false,"DeinterlaceMethod":"yadif","EnableDecodingColorDepth10Hevc":true,"EnableDecodingColorDepth10Vp9":true,"EnableDecodingColorDepth10HevcRext":false,"EnableDecodingColorDepth12HevcRext":false,"EnableEnhancedNvdecDecoder":true,"PreferSystemNativeHwDecoder":true,"EnableIntelLowPowerH264HwEncoder":false,"EnableIntelLowPowerHevcHwEncoder":false,"EnableHardwareEncoding":true,"AllowHevcEncoding":false,"AllowAv1Encoding":false,"EnableSubtitleExtraction":true,"SubtitleExtractionTimeoutMinutes":45,"HardwareDecodingCodecs":["h264","vc1"],"AllowOnDemandMetadataBasedKeyframeExtractionForExtensions":["mkv"],"HlsAudioSeekStrategy":"TranscodeAudio"}`
	checkRoundTrip[EncodingConfigurationResourceModel](t, mustWire(t, encodingWire), fixture)
}

func TestUnitEncodingConfigurationJellyfin12FieldsResolveToNullWhenKeysMissing(t *testing.T) {
	ctx := t.Context()
	b := mustWire(t, encodingWire)
	fixture := `{"EnableSubtitleExtraction":true,"HardwareDecodingCodecs":["h264","vc1"]}`

	data := readWire[EncodingConfigurationResourceModel](t, b, fixture)
	if !data.SubtitleExtractionTimeoutMinutes.IsNull() {
		t.Errorf("subtitle_extraction_timeout_minutes = %s, want null", data.SubtitleExtractionTimeoutMinutes)
	}
	if !data.HlsAudioSeekStrategy.IsNull() {
		t.Errorf("hls_audio_seek_strategy = %s, want null", data.HlsAudioSeekStrategy)
	}

	data.SubtitleExtractionTimeoutMinutes = types.Int64Unknown()
	data.HlsAudioSeekStrategy = types.StringUnknown()
	base, err := parseJSONObject(fixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("write: %v", d)
	}
	for _, key := range []string{"SubtitleExtractionTimeoutMinutes", "HlsAudioSeekStrategy"} {
		if raw, ok := base[key]; ok {
			t.Errorf("%s = %s, want key absent", key, raw)
		}
	}
}

func TestUnitEncodingConfigurationEnumValidators(t *testing.T) {
	attrs := schemaOf(&EncodingConfigurationResource{}).Attributes
	for name, valid := range map[string]string{
		"down_mix_stereo_algorithm":  "None",
		"hardware_acceleration_type": "none",
		"tonemapping_algorithm":      "bt2390",
		"tonemapping_mode":           "auto",
		"tonemapping_range":          "auto",
		"encoder_preset":             "auto",
		"deinterlace_method":         "yadif",
		"hls_audio_seek_strategy":    "TrimCopiedAudio",
	} {
		attr, ok := attrs[name].(rschema.StringAttribute)
		if !ok {
			t.Fatalf("%s attribute type = %T, want schema.StringAttribute", name, attrs[name])
		}
		t.Run(name, func(t *testing.T) {
			testUnitAssertStringValidation(t, attr, map[string]bool{valid: false, "": true})
		})
	}
}
