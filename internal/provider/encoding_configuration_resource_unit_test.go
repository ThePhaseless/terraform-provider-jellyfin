// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"
)

func TestUnitEncodingConfigurationOverlay(t *testing.T) {
	ctx := context.Background()
	fixture := `{"EncodingThreadCount":-1,"TranscodingTempPath":"/tmp","FallbackFontPath":"/fonts","EnableFallbackFont":false,"EnableAudioVbr":false,"DownMixAudioBoost":2,"DownMixStereoAlgorithm":"None","MaxMuxingQueueSize":2048,"EnableThrottling":false,"ThrottleDelaySeconds":180,"EnableSegmentDeletion":false,"SegmentKeepSeconds":720,"HardwareAccelerationType":"none","EncoderAppPath":"","EncoderAppPathDisplay":"","VaapiDevice":"/dev/dri/renderD128","QsvDevice":"","EnableTonemapping":false,"EnableVppTonemapping":false,"EnableVideoToolboxTonemapping":false,"TonemappingAlgorithm":"bt2390","TonemappingMode":"auto","TonemappingRange":"auto","TonemappingDesat":0,"TonemappingPeak":100,"TonemappingParam":0,"VppTonemappingBrightness":16,"VppTonemappingContrast":1,"H264Crf":23,"H265Crf":28,"EncoderPreset":"","DeinterlaceDoubleRate":false,"DeinterlaceMethod":"yadif","EnableDecodingColorDepth10Hevc":true,"EnableDecodingColorDepth10Vp9":true,"EnableDecodingColorDepth10HevcRext":false,"EnableDecodingColorDepth12HevcRext":false,"EnableEnhancedNvdecDecoder":true,"PreferSystemNativeHwDecoder":true,"EnableIntelLowPowerH264HwEncoder":false,"EnableIntelLowPowerHevcHwEncoder":false,"EnableHardwareEncoding":true,"AllowHevcEncoding":false,"AllowAv1Encoding":false,"EnableSubtitleExtraction":true,"HardwareDecodingCodecs":["h264","vc1"],"AllowOnDemandMetadataBasedKeyframeExtractionForExtensions":["mkv"]}`

	var data EncodingConfigurationResourceModel
	flattenEncodingConfiguration(ctx, fixture, &data, nil)

	base := map[string]json.RawMessage{}
	overlayEncodingConfiguration(ctx, base, &data)

	result, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	var want map[string]interface{}
	if err := json.Unmarshal([]byte(fixture), &want); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("round-trip mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}
