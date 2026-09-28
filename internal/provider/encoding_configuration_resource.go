// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

var (
	_ resource.Resource                = &EncodingConfigurationResource{}
	_ resource.ResourceWithImportState = &EncodingConfigurationResource{}
	_ resource.ResourceWithModifyPlan  = &EncodingConfigurationResource{}
	_ wireBound                        = &EncodingConfigurationResource{}
)

// NewEncodingConfigurationResource creates a new encoding configuration resource.
func NewEncodingConfigurationResource() resource.Resource {
	return &EncodingConfigurationResource{}
}

// EncodingConfigurationResource defines the resource implementation.
type EncodingConfigurationResource struct {
	client *client.Client
}

// EncodingConfigurationResourceModel describes the resource data model.
type EncodingConfigurationResourceModel struct {
	ID                                                        types.String  `tfsdk:"id"`
	EncodingThreadCount                                       types.Int64   `tfsdk:"encoding_thread_count"`
	TranscodingTempPath                                       types.String  `tfsdk:"transcoding_temp_path"`
	FallbackFontPath                                          types.String  `tfsdk:"fallback_font_path"`
	EnableFallbackFont                                        types.Bool    `tfsdk:"enable_fallback_font"`
	EnableAudioVbr                                            types.Bool    `tfsdk:"enable_audio_vbr"`
	DownMixAudioBoost                                         types.Float64 `tfsdk:"down_mix_audio_boost"`
	DownMixStereoAlgorithm                                    types.String  `tfsdk:"down_mix_stereo_algorithm"`
	MaxMuxingQueueSize                                        types.Int64   `tfsdk:"max_muxing_queue_size"`
	EnableThrottling                                          types.Bool    `tfsdk:"enable_throttling"`
	ThrottleDelaySeconds                                      types.Int64   `tfsdk:"throttle_delay_seconds"`
	EnableSegmentDeletion                                     types.Bool    `tfsdk:"enable_segment_deletion"`
	SegmentKeepSeconds                                        types.Int64   `tfsdk:"segment_keep_seconds"`
	HardwareAccelerationType                                  types.String  `tfsdk:"hardware_acceleration_type"`
	EncoderAppPath                                            types.String  `tfsdk:"encoder_app_path"`
	EncoderAppPathDisplay                                     types.String  `tfsdk:"encoder_app_path_display"`
	VaapiDevice                                               types.String  `tfsdk:"vaapi_device"`
	QsvDevice                                                 types.String  `tfsdk:"qsv_device"`
	EnableTonemapping                                         types.Bool    `tfsdk:"enable_tonemapping"`
	EnableVppTonemapping                                      types.Bool    `tfsdk:"enable_vpp_tonemapping"`
	EnableVideoToolboxTonemapping                             types.Bool    `tfsdk:"enable_video_toolbox_tonemapping"`
	TonemappingAlgorithm                                      types.String  `tfsdk:"tonemapping_algorithm"`
	TonemappingMode                                           types.String  `tfsdk:"tonemapping_mode"`
	TonemappingRange                                          types.String  `tfsdk:"tonemapping_range"`
	TonemappingDesat                                          types.Float64 `tfsdk:"tonemapping_desat"`
	TonemappingPeak                                           types.Float64 `tfsdk:"tonemapping_peak"`
	TonemappingParam                                          types.Float64 `tfsdk:"tonemapping_param"`
	VppTonemappingBrightness                                  types.Float64 `tfsdk:"vpp_tonemapping_brightness"`
	VppTonemappingContrast                                    types.Float64 `tfsdk:"vpp_tonemapping_contrast"`
	H264Crf                                                   types.Int64   `tfsdk:"h264_crf"`
	H265Crf                                                   types.Int64   `tfsdk:"h265_crf"`
	EncoderPreset                                             types.String  `tfsdk:"encoder_preset"`
	DeinterlaceDoubleRate                                     types.Bool    `tfsdk:"deinterlace_double_rate"`
	DeinterlaceMethod                                         types.String  `tfsdk:"deinterlace_method"`
	EnableDecodingColorDepth10Hevc                            types.Bool    `tfsdk:"enable_decoding_color_depth10_hevc"`
	EnableDecodingColorDepth10Vp9                             types.Bool    `tfsdk:"enable_decoding_color_depth10_vp9"`
	EnableDecodingColorDepth10HevcRext                        types.Bool    `tfsdk:"enable_decoding_color_depth10_hevc_rext"`
	EnableDecodingColorDepth12HevcRext                        types.Bool    `tfsdk:"enable_decoding_color_depth12_hevc_rext"`
	EnableEnhancedNvdecDecoder                                types.Bool    `tfsdk:"enable_enhanced_nvdec_decoder"`
	PreferSystemNativeHwDecoder                               types.Bool    `tfsdk:"prefer_system_native_hw_decoder"`
	EnableIntelLowPowerH264HwEncoder                          types.Bool    `tfsdk:"enable_intel_low_power_h264_hw_encoder"`
	EnableIntelLowPowerHevcHwEncoder                          types.Bool    `tfsdk:"enable_intel_low_power_hevc_hw_encoder"`
	EnableHardwareEncoding                                    types.Bool    `tfsdk:"enable_hardware_encoding"`
	AllowHevcEncoding                                         types.Bool    `tfsdk:"allow_hevc_encoding"`
	AllowAv1Encoding                                          types.Bool    `tfsdk:"allow_av1_encoding"`
	EnableSubtitleExtraction                                  types.Bool    `tfsdk:"enable_subtitle_extraction"`
	SubtitleExtractionTimeoutMinutes                          types.Int64   `tfsdk:"subtitle_extraction_timeout_minutes"`
	HardwareDecodingCodecs                                    types.List    `tfsdk:"hardware_decoding_codecs"`
	AllowOnDemandMetadataBasedKeyframeExtractionForExtensions types.List    `tfsdk:"allow_on_demand_metadata_based_keyframe_extraction_for_extensions"`
	HlsAudioSeekStrategy                                      types.String  `tfsdk:"hls_audio_seek_strategy"`
}

var encodingWire = sync.OnceValues(func() (*wire.Binding, error) {
	return wire.Bind(schemaOf(&EncodingConfigurationResource{}), "EncodingOptions",
		wire.Identity("id"),
		wire.VersionMessage("hls_audio_seek_strategy", encodingVersionMessage),
		wire.VersionMessage("subtitle_extraction_timeout_minutes", encodingVersionMessage))
})

func (r *EncodingConfigurationResource) Wire() (*wire.Binding, error) { return encodingWire() }

func encodingVersionMessage(g wire.VersionGap) (string, string) {
	return "Unsupported Jellyfin server version",
		fmt.Sprintf("%s requires Jellyfin %s or later: the server's encoding configuration has no %s field, so it would discard the value. Remove %s from the configuration or upgrade the server.", g.Path, g.Since, g.Key, g.Path)
}

func (r *EncodingConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_encoding_configuration"
}

func (r *EncodingConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	hlsAudioSeekStrategy := optionalString("Method used to seek the audio stream when transcoding HLS segments. One of `TrimCopiedAudio`, `TranscodeAudio`. Requires Jellyfin 12.0 or later.")
	hlsAudioSeekStrategy.Validators = []validator.String{stringvalidator.OneOf("TrimCopiedAudio", "TranscodeAudio")}

	resp.Schema = schema.Schema{
		Description:         "Manages the Jellyfin encoding configuration.",
		MarkdownDescription: "Manages the Jellyfin encoding configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "Resource identifier. Always set to `encoding` for this singleton resource.",
				MarkdownDescription: "Resource identifier. Always set to `encoding` for this singleton resource.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"encoding_thread_count":                   optionalInt("Encoding thread count."),
			"transcoding_temp_path":                   optionalString("Transcoding temporary path."),
			"fallback_font_path":                      optionalString("Fallback font path."),
			"enable_fallback_font":                    optionalBool("Whether fallback font is enabled."),
			"enable_audio_vbr":                        optionalBool("Whether audio VBR is enabled."),
			"down_mix_audio_boost":                    optionalFloat("Down-mix audio boost."),
			"down_mix_stereo_algorithm":               optionalEnum("Down-mix stereo algorithm.", "None", "Dave750", "NightmodeDialogue", "Rfc7845", "Ac4"),
			"max_muxing_queue_size":                   optionalInt("Max muxing queue size."),
			"enable_throttling":                       optionalBool("Whether throttling is enabled."),
			"throttle_delay_seconds":                  optionalInt("Throttle delay in seconds."),
			"enable_segment_deletion":                 optionalBool("Whether segment deletion is enabled."),
			"segment_keep_seconds":                    optionalInt("Segment keep time in seconds."),
			"hardware_acceleration_type":              optionalEnum("Hardware acceleration type.", "none", "amf", "qsv", "nvenc", "v4l2m2m", "vaapi", "videotoolbox", "rkmpp"),
			"encoder_app_path":                        optionalString("Encoder application path."),
			"encoder_app_path_display":                optionalString("Encoder application display path."),
			"vaapi_device":                            optionalString("VAAPI device."),
			"qsv_device":                              optionalString("QSV device."),
			"enable_tonemapping":                      optionalBool("Whether tonemapping is enabled."),
			"enable_vpp_tonemapping":                  optionalBool("Whether VPP tonemapping is enabled."),
			"enable_video_toolbox_tonemapping":        optionalBool("Whether VideoToolbox tonemapping is enabled."),
			"tonemapping_algorithm":                   optionalEnum("Tonemapping algorithm.", "none", "clip", "linear", "gamma", "reinhard", "hable", "mobius", "bt2390"),
			"tonemapping_mode":                        optionalEnum("Tonemapping mode.", "auto", "max", "rgb", "lum", "itp"),
			"tonemapping_range":                       optionalEnum("Tonemapping range.", "auto", "tv", "pc"),
			"tonemapping_desat":                       optionalFloat("Tonemapping desaturation."),
			"tonemapping_peak":                        optionalFloat("Tonemapping peak."),
			"tonemapping_param":                       optionalFloat("Tonemapping parameter."),
			"vpp_tonemapping_brightness":              optionalFloat("VPP tonemapping brightness."),
			"vpp_tonemapping_contrast":                optionalFloat("VPP tonemapping contrast."),
			"h264_crf":                                optionalInt("H264 CRF."),
			"h265_crf":                                optionalInt("H265 CRF."),
			"encoder_preset":                          optionalEnum("Encoder preset.", "auto", "placebo", "veryslow", "slower", "slow", "medium", "fast", "faster", "veryfast", "superfast", "ultrafast"),
			"deinterlace_double_rate":                 optionalBool("Whether deinterlace double rate is enabled."),
			"deinterlace_method":                      optionalEnum("Deinterlace method.", "yadif", "bwdif"),
			"enable_decoding_color_depth10_hevc":      optionalBool("Whether 10-bit HEVC decoding is enabled."),
			"enable_decoding_color_depth10_vp9":       optionalBool("Whether 10-bit VP9 decoding is enabled."),
			"enable_decoding_color_depth10_hevc_rext": optionalBool("Whether 10-bit HEVC RExt decoding is enabled."),
			"enable_decoding_color_depth12_hevc_rext": optionalBool("Whether 12-bit HEVC RExt decoding is enabled."),
			"enable_enhanced_nvdec_decoder":           optionalBool("Whether enhanced NVDEC decoder is enabled."),
			"prefer_system_native_hw_decoder":         optionalBool("Whether to prefer system native hardware decoder."),
			"enable_intel_low_power_h264_hw_encoder":  optionalBool("Whether Intel low-power H264 hardware encoder is enabled."),
			"enable_intel_low_power_hevc_hw_encoder":  optionalBool("Whether Intel low-power HEVC hardware encoder is enabled."),
			"enable_hardware_encoding":                optionalBool("Whether hardware encoding is enabled."),
			"allow_hevc_encoding":                     optionalBool("Whether HEVC encoding is allowed."),
			"allow_av1_encoding":                      optionalBool("Whether AV1 encoding is allowed."),
			"enable_subtitle_extraction":              optionalBool("Whether subtitle extraction is enabled."),
			"subtitle_extraction_timeout_minutes":     optionalInt("Subtitle extraction timeout in minutes. Requires Jellyfin 12.0 or later."),
			"hardware_decoding_codecs":                optionalStringList("Hardware decoding codecs."),
			"allow_on_demand_metadata_based_keyframe_extraction_for_extensions": optionalStringList("Extensions allowing on-demand metadata-based keyframe extraction."),
			"hls_audio_seek_strategy": hlsAudioSeekStrategy,
		},
	}
}

func (r *EncodingConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *EncodingConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.singleton().write(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *EncodingConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.singleton().read(ctx, req.State, &resp.State, &resp.Diagnostics)
}

func (r *EncodingConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.singleton().write(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *EncodingConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Encoding configuration cannot be deleted. We just remove from state.
}

func (r *EncodingConfigurationResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	r.singleton().setID(ctx, &resp.State, &resp.Diagnostics)
}

// ModifyPlan rejects configured fields the server's Jellyfin version lacks,
// such as the 12.0 ones on 10.11, which the server would accept and drop.
func (r *EncodingConfigurationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if !req.Plan.Raw.IsNull() {
		checkServerHasFields(ctx, r.client, encodingWire, req.Config, &resp.Diagnostics)
	}
}

func (r *EncodingConfigurationResource) singleton() singleton[EncodingConfigurationResourceModel] {
	return singleton[EncodingConfigurationResourceModel]{
		id:   "encoding",
		bind: encodingWire,
		doc:  document{what: "encoding configuration", get: r.client.GetEncodingOptions, put: r.client.UpdateEncodingOptions},
	}
}
