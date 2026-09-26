// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnitEncodingConfigurationOverlay(t *testing.T) {
	ctx := context.Background()
	fixture := `{"EncodingThreadCount":-1,"TranscodingTempPath":"/tmp","FallbackFontPath":"/fonts","EnableFallbackFont":false,"EnableAudioVbr":false,"DownMixAudioBoost":2,"DownMixStereoAlgorithm":"None","MaxMuxingQueueSize":2048,"EnableThrottling":false,"ThrottleDelaySeconds":180,"EnableSegmentDeletion":false,"SegmentKeepSeconds":720,"HardwareAccelerationType":"none","EncoderAppPath":"","EncoderAppPathDisplay":"","VaapiDevice":"/dev/dri/renderD128","QsvDevice":"","EnableTonemapping":false,"EnableVppTonemapping":false,"EnableVideoToolboxTonemapping":false,"TonemappingAlgorithm":"bt2390","TonemappingMode":"auto","TonemappingRange":"auto","TonemappingDesat":0,"TonemappingPeak":100,"TonemappingParam":0,"VppTonemappingBrightness":16,"VppTonemappingContrast":1,"H264Crf":23,"H265Crf":28,"EncoderPreset":"auto","DeinterlaceDoubleRate":false,"DeinterlaceMethod":"yadif","EnableDecodingColorDepth10Hevc":true,"EnableDecodingColorDepth10Vp9":true,"EnableDecodingColorDepth10HevcRext":false,"EnableDecodingColorDepth12HevcRext":false,"EnableEnhancedNvdecDecoder":true,"PreferSystemNativeHwDecoder":true,"EnableIntelLowPowerH264HwEncoder":false,"EnableIntelLowPowerHevcHwEncoder":false,"EnableHardwareEncoding":true,"AllowHevcEncoding":false,"AllowAv1Encoding":false,"EnableSubtitleExtraction":true,"SubtitleExtractionTimeoutMinutes":45,"HardwareDecodingCodecs":["h264","vc1"],"AllowOnDemandMetadataBasedKeyframeExtractionForExtensions":["mkv"],"HlsAudioSeekStrategy":"TranscodeAudio"}`

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

func TestUnitEncodingConfigurationJellyfin12FieldsResolveToNullWhenKeysMissing(t *testing.T) {
	ctx := context.Background()
	fixture := `{"EnableSubtitleExtraction":true,"HardwareDecodingCodecs":["h264","vc1"]}`

	data := EncodingConfigurationResourceModel{
		SubtitleExtractionTimeoutMinutes: types.Int64Unknown(),
		HlsAudioSeekStrategy:             types.StringUnknown(),
	}

	base, err := parseJSONObject(fixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d := overlayEncodingConfiguration(ctx, base, &data); d.HasError() {
		t.Fatalf("overlay: %v", d.Errors())
	}
	for _, key := range []string{"SubtitleExtractionTimeoutMinutes", "HlsAudioSeekStrategy"} {
		if raw, ok := base[key]; ok {
			t.Errorf("%s = %s, want key absent", key, raw)
		}
	}

	var diags diag.Diagnostics
	flattenEncodingConfiguration(ctx, fixture, &data, &diags)
	if diags.HasError() {
		t.Fatalf("flatten: %v", diags.Errors())
	}
	if !data.SubtitleExtractionTimeoutMinutes.IsNull() {
		t.Errorf("subtitle_extraction_timeout_minutes = %s, want null", data.SubtitleExtractionTimeoutMinutes)
	}
	if !data.HlsAudioSeekStrategy.IsNull() {
		t.Errorf("hls_audio_seek_strategy = %s, want null", data.HlsAudioSeekStrategy)
	}
}

func TestUnitEncodingConfigurationEnumValidators(t *testing.T) {
	ctx := context.Background()

	var resp resource.SchemaResponse
	(&EncodingConfigurationResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)

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
		attr, ok := resp.Schema.Attributes[name].(rschema.StringAttribute)
		if !ok {
			t.Fatalf("%s attribute type = %T, want schema.StringAttribute", name, resp.Schema.Attributes[name])
		}

		for value, wantError := range map[string]bool{valid: false, "": true} {
			var diags diag.Diagnostics
			for _, v := range attr.Validators {
				vresp := validator.StringResponse{}
				v.ValidateString(ctx, validator.StringRequest{
					Path:        path.Root(name),
					ConfigValue: types.StringValue(value),
				}, &vresp)
				diags.Append(vresp.Diagnostics...)
			}

			if diags.HasError() != wantError {
				t.Errorf("%s = %q: got error %t, want %t: %v", name, value, diags.HasError(), wantError, diags)
			}
		}
	}
}

func TestUnitEncodingConfigurationJellyfin12FieldsCheckedAgainstServerKeys(t *testing.T) {
	withoutKeys := `{"EnableSubtitleExtraction":true}`
	withKeys := `{"EnableSubtitleExtraction":true,"SubtitleExtractionTimeoutMinutes":30,"HlsAudioSeekStrategy":"TrimCopiedAudio"}`
	configured := EncodingConfigurationResourceModel{
		SubtitleExtractionTimeoutMinutes: types.Int64Value(45),
		HlsAudioSeekStrategy:             types.StringValue("TranscodeAudio"),
	}

	for name, tc := range map[string]struct {
		server    string
		data      EncodingConfigurationResourceModel
		wantPaths []path.Path
	}{
		"configured, server lacks keys": {
			server:    withoutKeys,
			data:      configured,
			wantPaths: []path.Path{path.Root("subtitle_extraction_timeout_minutes"), path.Root("hls_audio_seek_strategy")},
		},
		"configured, server has keys": {
			server: withKeys,
			data:   configured,
		},
		"unset, server lacks keys": {
			server: withoutKeys,
			data: EncodingConfigurationResourceModel{
				SubtitleExtractionTimeoutMinutes: types.Int64Null(),
				HlsAudioSeekStrategy:             types.StringUnknown(),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			base, err := parseJSONObject(tc.server)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			diags := checkJellyfin12EncodingKeys(base, &tc.data)

			if got := diags.ErrorsCount(); got != len(tc.wantPaths) {
				t.Fatalf("got %d errors, want %d: %v", got, len(tc.wantPaths), diags)
			}
			for i, want := range tc.wantPaths {
				withPath, ok := diags.Errors()[i].(diag.DiagnosticWithPath)
				if !ok {
					t.Fatalf("error %d has no attribute path: %v", i, diags.Errors()[i])
				}
				if !withPath.Path().Equal(want) {
					t.Errorf("error %d path = %s, want %s", i, withPath.Path(), want)
				}
			}
		})
	}
}
