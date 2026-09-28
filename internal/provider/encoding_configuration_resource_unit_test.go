// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func TestUnitEncodingConfigurationRoundTrip(t *testing.T) {
	fixture := `{"EncodingThreadCount":-1,"TranscodingTempPath":"/tmp","FallbackFontPath":"/fonts","EnableFallbackFont":false,"EnableAudioVbr":false,"DownMixAudioBoost":2,"DownMixStereoAlgorithm":"None","MaxMuxingQueueSize":2048,"EnableThrottling":false,"ThrottleDelaySeconds":180,"EnableSegmentDeletion":false,"SegmentKeepSeconds":720,"HardwareAccelerationType":"none","EncoderAppPath":"","EncoderAppPathDisplay":"","VaapiDevice":"/dev/dri/renderD128","QsvDevice":"","EnableTonemapping":false,"EnableVppTonemapping":false,"EnableVideoToolboxTonemapping":false,"TonemappingAlgorithm":"bt2390","TonemappingMode":"auto","TonemappingRange":"auto","TonemappingDesat":0,"TonemappingPeak":100,"TonemappingParam":0,"VppTonemappingBrightness":16,"VppTonemappingContrast":1,"H264Crf":23,"H265Crf":28,"EncoderPreset":"auto","DeinterlaceDoubleRate":false,"DeinterlaceMethod":"yadif","EnableDecodingColorDepth10Hevc":true,"EnableDecodingColorDepth10Vp9":true,"EnableDecodingColorDepth10HevcRext":false,"EnableDecodingColorDepth12HevcRext":false,"EnableEnhancedNvdecDecoder":true,"PreferSystemNativeHwDecoder":true,"EnableIntelLowPowerH264HwEncoder":false,"EnableIntelLowPowerHevcHwEncoder":false,"EnableHardwareEncoding":true,"AllowHevcEncoding":false,"AllowAv1Encoding":false,"EnableSubtitleExtraction":true,"SubtitleExtractionTimeoutMinutes":45,"HardwareDecodingCodecs":["h264","vc1"],"AllowOnDemandMetadataBasedKeyframeExtractionForExtensions":["mkv"],"HlsAudioSeekStrategy":"TranscodeAudio"}`
	checkRoundTrip[EncodingConfigurationResourceModel](t, mustWire(t, encodingWire), fixture)
}

func TestUnitEncodingConfigurationJellyfin12FieldsResolveToNullWhenKeysMissing(t *testing.T) {
	ctx := context.Background()
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

func TestUnitEncodingConfigurationPlanRejectsJellyfin12FieldsOnOlderServers(t *testing.T) {
	t.Parallel()

	configured := EncodingConfigurationResourceModel{
		SubtitleExtractionTimeoutMinutes:                          types.Int64Value(45),
		HlsAudioSeekStrategy:                                      types.StringValue("TranscodeAudio"),
		HardwareDecodingCodecs:                                    types.ListNull(types.StringType),
		AllowOnDemandMetadataBasedKeyframeExtractionForExtensions: types.ListNull(types.StringType),
	}
	unset := configured
	unset.SubtitleExtractionTimeoutMinutes = types.Int64Null()
	unset.HlsAudioSeekStrategy = types.StringNull()
	wantDetails := map[string]string{
		"hls_audio_seek_strategy":             "hls_audio_seek_strategy requires Jellyfin 12.0 or later: the server's encoding configuration has no HlsAudioSeekStrategy field, so it would discard the value. Remove hls_audio_seek_strategy from the configuration or upgrade the server.",
		"subtitle_extraction_timeout_minutes": "subtitle_extraction_timeout_minutes requires Jellyfin 12.0 or later: the server's encoding configuration has no SubtitleExtractionTimeoutMinutes field, so it would discard the value. Remove subtitle_extraction_timeout_minutes from the configuration or upgrade the server.",
	}

	for name, tc := range map[string]struct {
		version       string
		config        EncodingConfigurationResourceModel
		wantPaths     []string
		wantInfoCalls int32
	}{
		"configured on 10.11.11": {
			version:       "10.11.11",
			config:        configured,
			wantPaths:     []string{"hls_audio_seek_strategy", "subtitle_extraction_timeout_minutes"},
			wantInfoCalls: 1,
		},
		"configured on 12.1.0": {version: "12.1.0", config: configured, wantInfoCalls: 1},
		"unset on 10.11.11":    {version: "10.11.11", config: unset},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var infoCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/System/Info/Public" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				infoCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"Version":"`+tc.version+`"}`)
			}))
			defer server.Close()

			ctx := context.Background()
			r := &EncodingConfigurationResource{client: client.NewClient(server.URL, "k")}
			plan := tfsdk.Plan{Schema: schemaOf(r)}
			if d := plan.Set(ctx, &tc.config); d.HasError() {
				t.Fatalf("plan: %v", d)
			}
			resp := resource.ModifyPlanResponse{Plan: plan}
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{Config: tfsdk.Config(plan), Plan: plan}, &resp)

			if got := infoCalls.Load(); got != tc.wantInfoCalls {
				t.Errorf("GET /System/Info/Public sent %d times, want %d", got, tc.wantInfoCalls)
			}
			var gotPaths []string
			for _, d := range resp.Diagnostics.Errors() {
				withPath, ok := d.(diag.DiagnosticWithPath)
				if !ok {
					t.Fatalf("error has no attribute path: %v", d)
				}
				p := withPath.Path().String()
				gotPaths = append(gotPaths, p)
				if d.Summary() != "Unsupported Jellyfin server version" || d.Detail() != wantDetails[p] {
					t.Errorf("error at %s:\n got %q: %q\nwant %q: %q", p, d.Summary(), d.Detail(), "Unsupported Jellyfin server version", wantDetails[p])
				}
			}
			slices.Sort(gotPaths)
			if !slices.Equal(gotPaths, tc.wantPaths) {
				t.Errorf("errors at %v, want %v", gotPaths, tc.wantPaths)
			}
		})
	}
}
