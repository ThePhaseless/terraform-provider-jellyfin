// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

var (
	_ resource.Resource                = &EncodingConfigurationResource{}
	_ resource.ResourceWithImportState = &EncodingConfigurationResource{}
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
	HardwareDecodingCodecs                                    types.List    `tfsdk:"hardware_decoding_codecs"`
	AllowOnDemandMetadataBasedKeyframeExtractionForExtensions types.List    `tfsdk:"allow_on_demand_metadata_based_keyframe_extraction_for_extensions"`
}

func (r *EncodingConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_encoding_configuration"
}

func (r *EncodingConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
			"encoding_thread_count":                   schema.Int64Attribute{Description: "Encoding thread count.", MarkdownDescription: "Encoding thread count.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"transcoding_temp_path":                   schema.StringAttribute{Description: "Transcoding temporary path.", MarkdownDescription: "Transcoding temporary path.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"fallback_font_path":                      schema.StringAttribute{Description: "Fallback font path.", MarkdownDescription: "Fallback font path.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"enable_fallback_font":                    schema.BoolAttribute{Description: "Whether fallback font is enabled.", MarkdownDescription: "Whether fallback font is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_audio_vbr":                        schema.BoolAttribute{Description: "Whether audio VBR is enabled.", MarkdownDescription: "Whether audio VBR is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"down_mix_audio_boost":                    schema.Float64Attribute{Description: "Down-mix audio boost.", MarkdownDescription: "Down-mix audio boost.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Float64{float64planmodifier.UseStateForUnknown()}},
			"down_mix_stereo_algorithm":               schema.StringAttribute{Description: "Down-mix stereo algorithm.", MarkdownDescription: "Down-mix stereo algorithm.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"max_muxing_queue_size":                   schema.Int64Attribute{Description: "Max muxing queue size.", MarkdownDescription: "Max muxing queue size.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"enable_throttling":                       schema.BoolAttribute{Description: "Whether throttling is enabled.", MarkdownDescription: "Whether throttling is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"throttle_delay_seconds":                  schema.Int64Attribute{Description: "Throttle delay in seconds.", MarkdownDescription: "Throttle delay in seconds.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"enable_segment_deletion":                 schema.BoolAttribute{Description: "Whether segment deletion is enabled.", MarkdownDescription: "Whether segment deletion is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"segment_keep_seconds":                    schema.Int64Attribute{Description: "Segment keep time in seconds.", MarkdownDescription: "Segment keep time in seconds.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"hardware_acceleration_type":              schema.StringAttribute{Description: "Hardware acceleration type.", MarkdownDescription: "Hardware acceleration type.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"encoder_app_path":                        schema.StringAttribute{Description: "Encoder application path.", MarkdownDescription: "Encoder application path.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"encoder_app_path_display":                schema.StringAttribute{Description: "Encoder application display path.", MarkdownDescription: "Encoder application display path.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"vaapi_device":                            schema.StringAttribute{Description: "VAAPI device.", MarkdownDescription: "VAAPI device.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"qsv_device":                              schema.StringAttribute{Description: "QSV device.", MarkdownDescription: "QSV device.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"enable_tonemapping":                      schema.BoolAttribute{Description: "Whether tonemapping is enabled.", MarkdownDescription: "Whether tonemapping is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_vpp_tonemapping":                  schema.BoolAttribute{Description: "Whether VPP tonemapping is enabled.", MarkdownDescription: "Whether VPP tonemapping is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_video_toolbox_tonemapping":        schema.BoolAttribute{Description: "Whether VideoToolbox tonemapping is enabled.", MarkdownDescription: "Whether VideoToolbox tonemapping is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"tonemapping_algorithm":                   schema.StringAttribute{Description: "Tonemapping algorithm.", MarkdownDescription: "Tonemapping algorithm.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"tonemapping_mode":                        schema.StringAttribute{Description: "Tonemapping mode.", MarkdownDescription: "Tonemapping mode.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"tonemapping_range":                       schema.StringAttribute{Description: "Tonemapping range.", MarkdownDescription: "Tonemapping range.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"tonemapping_desat":                       schema.Float64Attribute{Description: "Tonemapping desaturation.", MarkdownDescription: "Tonemapping desaturation.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Float64{float64planmodifier.UseStateForUnknown()}},
			"tonemapping_peak":                        schema.Float64Attribute{Description: "Tonemapping peak.", MarkdownDescription: "Tonemapping peak.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Float64{float64planmodifier.UseStateForUnknown()}},
			"tonemapping_param":                       schema.Float64Attribute{Description: "Tonemapping parameter.", MarkdownDescription: "Tonemapping parameter.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Float64{float64planmodifier.UseStateForUnknown()}},
			"vpp_tonemapping_brightness":              schema.Float64Attribute{Description: "VPP tonemapping brightness.", MarkdownDescription: "VPP tonemapping brightness.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Float64{float64planmodifier.UseStateForUnknown()}},
			"vpp_tonemapping_contrast":                schema.Float64Attribute{Description: "VPP tonemapping contrast.", MarkdownDescription: "VPP tonemapping contrast.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Float64{float64planmodifier.UseStateForUnknown()}},
			"h264_crf":                                schema.Int64Attribute{Description: "H264 CRF.", MarkdownDescription: "H264 CRF.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"h265_crf":                                schema.Int64Attribute{Description: "H265 CRF.", MarkdownDescription: "H265 CRF.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"encoder_preset":                          schema.StringAttribute{Description: "Encoder preset.", MarkdownDescription: "Encoder preset.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"deinterlace_double_rate":                 schema.BoolAttribute{Description: "Whether deinterlace double rate is enabled.", MarkdownDescription: "Whether deinterlace double rate is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"deinterlace_method":                      schema.StringAttribute{Description: "Deinterlace method.", MarkdownDescription: "Deinterlace method.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"enable_decoding_color_depth10_hevc":      schema.BoolAttribute{Description: "Whether 10-bit HEVC decoding is enabled.", MarkdownDescription: "Whether 10-bit HEVC decoding is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_decoding_color_depth10_vp9":       schema.BoolAttribute{Description: "Whether 10-bit VP9 decoding is enabled.", MarkdownDescription: "Whether 10-bit VP9 decoding is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_decoding_color_depth10_hevc_rext": schema.BoolAttribute{Description: "Whether 10-bit HEVC RExt decoding is enabled.", MarkdownDescription: "Whether 10-bit HEVC RExt decoding is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_decoding_color_depth12_hevc_rext": schema.BoolAttribute{Description: "Whether 12-bit HEVC RExt decoding is enabled.", MarkdownDescription: "Whether 12-bit HEVC RExt decoding is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_enhanced_nvdec_decoder":           schema.BoolAttribute{Description: "Whether enhanced NVDEC decoder is enabled.", MarkdownDescription: "Whether enhanced NVDEC decoder is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"prefer_system_native_hw_decoder":         schema.BoolAttribute{Description: "Whether to prefer system native hardware decoder.", MarkdownDescription: "Whether to prefer system native hardware decoder.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_intel_low_power_h264_hw_encoder":  schema.BoolAttribute{Description: "Whether Intel low-power H264 hardware encoder is enabled.", MarkdownDescription: "Whether Intel low-power H264 hardware encoder is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_intel_low_power_hevc_hw_encoder":  schema.BoolAttribute{Description: "Whether Intel low-power HEVC hardware encoder is enabled.", MarkdownDescription: "Whether Intel low-power HEVC hardware encoder is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_hardware_encoding":                schema.BoolAttribute{Description: "Whether hardware encoding is enabled.", MarkdownDescription: "Whether hardware encoding is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"allow_hevc_encoding":                     schema.BoolAttribute{Description: "Whether HEVC encoding is allowed.", MarkdownDescription: "Whether HEVC encoding is allowed.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"allow_av1_encoding":                      schema.BoolAttribute{Description: "Whether AV1 encoding is allowed.", MarkdownDescription: "Whether AV1 encoding is allowed.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_subtitle_extraction":              schema.BoolAttribute{Description: "Whether subtitle extraction is enabled.", MarkdownDescription: "Whether subtitle extraction is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"hardware_decoding_codecs":                schema.ListAttribute{ElementType: types.StringType, Description: "Hardware decoding codecs.", MarkdownDescription: "Hardware decoding codecs.", Optional: true, Computed: true, PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}},
			"allow_on_demand_metadata_based_keyframe_extraction_for_extensions": schema.ListAttribute{ElementType: types.StringType, Description: "Extensions allowing on-demand metadata-based keyframe extraction.", MarkdownDescription: "Extensions allowing on-demand metadata-based keyframe extraction.", Optional: true, Computed: true, PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *EncodingConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}

	r.client = c
}

func (r *EncodingConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EncodingConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *EncodingConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EncodingConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *EncodingConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data EncodingConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *EncodingConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Encoding configuration cannot be deleted. We just remove from state.
}

func (r *EncodingConfigurationResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Singleton resource — the import ID is not used. Read will populate all fields.
	data := EncodingConfigurationResourceModel{ID: types.StringValue("encoding")}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EncodingConfigurationResource) apply(ctx context.Context, data *EncodingConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetEncodingOptions(ctx)
	if err != nil {
		diags.AddError("Failed to read current encoding configuration", err.Error())
		return
	}

	base, err := parseJSONObject(current.RawJSON)
	if err != nil {
		diags.AddError("Failed to parse current encoding configuration", err.Error())
		return
	}

	d := overlayEncodingConfiguration(ctx, base, data)
	if d.HasError() {
		diags.Append(d...)
		return
	}

	payload, err := json.Marshal(base)
	if err != nil {
		diags.AddError("Failed to serialize encoding configuration", err.Error())
		return
	}

	if err := r.client.UpdateEncodingOptions(ctx, &client.EncodingOptions{RawJSON: string(payload)}); err != nil {
		diags.AddError("Failed to update encoding configuration", err.Error())
		return
	}

	updated, err := r.client.GetEncodingOptions(ctx)
	if err != nil {
		diags.AddError("Failed to read encoding configuration after update", err.Error())
		return
	}

	flattenEncodingConfiguration(ctx, updated.RawJSON, data, diags)
	data.ID = types.StringValue("encoding")
	diags.Append(state.Set(ctx, data)...)
}

func (r *EncodingConfigurationResource) read(ctx context.Context, data *EncodingConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetEncodingOptions(ctx)
	if err != nil {
		diags.AddError("Failed to read encoding configuration", err.Error())
		return
	}

	flattenEncodingConfiguration(ctx, current.RawJSON, data, diags)
	data.ID = types.StringValue("encoding")
	diags.Append(state.Set(ctx, data)...)
}

func overlayEncodingConfiguration(ctx context.Context, m map[string]json.RawMessage, data *EncodingConfigurationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	putJSONInt64(m, "EncodingThreadCount", data.EncodingThreadCount)
	putJSONString(m, "TranscodingTempPath", data.TranscodingTempPath)
	putJSONString(m, "FallbackFontPath", data.FallbackFontPath)
	putJSONBool(m, "EnableFallbackFont", data.EnableFallbackFont)
	putJSONBool(m, "EnableAudioVbr", data.EnableAudioVbr)
	putJSONFloat64(m, "DownMixAudioBoost", data.DownMixAudioBoost)
	putJSONString(m, "DownMixStereoAlgorithm", data.DownMixStereoAlgorithm)
	putJSONInt64(m, "MaxMuxingQueueSize", data.MaxMuxingQueueSize)
	putJSONBool(m, "EnableThrottling", data.EnableThrottling)
	putJSONInt64(m, "ThrottleDelaySeconds", data.ThrottleDelaySeconds)
	putJSONBool(m, "EnableSegmentDeletion", data.EnableSegmentDeletion)
	putJSONInt64(m, "SegmentKeepSeconds", data.SegmentKeepSeconds)
	putJSONString(m, "HardwareAccelerationType", data.HardwareAccelerationType)
	putJSONString(m, "EncoderAppPath", data.EncoderAppPath)
	putJSONString(m, "EncoderAppPathDisplay", data.EncoderAppPathDisplay)
	putJSONString(m, "VaapiDevice", data.VaapiDevice)
	putJSONString(m, "QsvDevice", data.QsvDevice)
	putJSONBool(m, "EnableTonemapping", data.EnableTonemapping)
	putJSONBool(m, "EnableVppTonemapping", data.EnableVppTonemapping)
	putJSONBool(m, "EnableVideoToolboxTonemapping", data.EnableVideoToolboxTonemapping)
	putJSONString(m, "TonemappingAlgorithm", data.TonemappingAlgorithm)
	putJSONString(m, "TonemappingMode", data.TonemappingMode)
	putJSONString(m, "TonemappingRange", data.TonemappingRange)
	putJSONFloat64(m, "TonemappingDesat", data.TonemappingDesat)
	putJSONFloat64(m, "TonemappingPeak", data.TonemappingPeak)
	putJSONFloat64(m, "TonemappingParam", data.TonemappingParam)
	putJSONFloat64(m, "VppTonemappingBrightness", data.VppTonemappingBrightness)
	putJSONFloat64(m, "VppTonemappingContrast", data.VppTonemappingContrast)
	putJSONInt64(m, "H264Crf", data.H264Crf)
	putJSONInt64(m, "H265Crf", data.H265Crf)
	putJSONString(m, "EncoderPreset", data.EncoderPreset)
	putJSONBool(m, "DeinterlaceDoubleRate", data.DeinterlaceDoubleRate)
	putJSONString(m, "DeinterlaceMethod", data.DeinterlaceMethod)
	putJSONBool(m, "EnableDecodingColorDepth10Hevc", data.EnableDecodingColorDepth10Hevc)
	putJSONBool(m, "EnableDecodingColorDepth10Vp9", data.EnableDecodingColorDepth10Vp9)
	putJSONBool(m, "EnableDecodingColorDepth10HevcRext", data.EnableDecodingColorDepth10HevcRext)
	putJSONBool(m, "EnableDecodingColorDepth12HevcRext", data.EnableDecodingColorDepth12HevcRext)
	putJSONBool(m, "EnableEnhancedNvdecDecoder", data.EnableEnhancedNvdecDecoder)
	putJSONBool(m, "PreferSystemNativeHwDecoder", data.PreferSystemNativeHwDecoder)
	putJSONBool(m, "EnableIntelLowPowerH264HwEncoder", data.EnableIntelLowPowerH264HwEncoder)
	putJSONBool(m, "EnableIntelLowPowerHevcHwEncoder", data.EnableIntelLowPowerHevcHwEncoder)
	putJSONBool(m, "EnableHardwareEncoding", data.EnableHardwareEncoding)
	putJSONBool(m, "AllowHevcEncoding", data.AllowHevcEncoding)
	putJSONBool(m, "AllowAv1Encoding", data.AllowAv1Encoding)
	putJSONBool(m, "EnableSubtitleExtraction", data.EnableSubtitleExtraction)
	if d := putJSONStringList(ctx, m, "HardwareDecodingCodecs", data.HardwareDecodingCodecs); d.HasError() {
		return d
	}
	if d := putJSONStringList(ctx, m, "AllowOnDemandMetadataBasedKeyframeExtractionForExtensions", data.AllowOnDemandMetadataBasedKeyframeExtractionForExtensions); d.HasError() {
		return d
	}
	return diags
}

func flattenEncodingConfiguration(ctx context.Context, raw string, data *EncodingConfigurationResourceModel, diags *diag.Diagnostics) {
	m, err := parseJSONObject(raw)
	if err != nil {
		diags.AddError("Failed to parse encoding configuration", err.Error())
		return
	}
	data.EncodingThreadCount = getJSONInt64(m, "EncodingThreadCount")
	data.TranscodingTempPath = getJSONString(m, "TranscodingTempPath")
	data.FallbackFontPath = getJSONString(m, "FallbackFontPath")
	data.EnableFallbackFont = getJSONBool(m, "EnableFallbackFont")
	data.EnableAudioVbr = getJSONBool(m, "EnableAudioVbr")
	data.DownMixAudioBoost = getJSONFloat64(m, "DownMixAudioBoost")
	data.DownMixStereoAlgorithm = getJSONString(m, "DownMixStereoAlgorithm")
	data.MaxMuxingQueueSize = getJSONInt64(m, "MaxMuxingQueueSize")
	data.EnableThrottling = getJSONBool(m, "EnableThrottling")
	data.ThrottleDelaySeconds = getJSONInt64(m, "ThrottleDelaySeconds")
	data.EnableSegmentDeletion = getJSONBool(m, "EnableSegmentDeletion")
	data.SegmentKeepSeconds = getJSONInt64(m, "SegmentKeepSeconds")
	data.HardwareAccelerationType = getJSONString(m, "HardwareAccelerationType")
	data.EncoderAppPath = getJSONString(m, "EncoderAppPath")
	data.EncoderAppPathDisplay = getJSONString(m, "EncoderAppPathDisplay")
	data.VaapiDevice = getJSONString(m, "VaapiDevice")
	data.QsvDevice = getJSONString(m, "QsvDevice")
	data.EnableTonemapping = getJSONBool(m, "EnableTonemapping")
	data.EnableVppTonemapping = getJSONBool(m, "EnableVppTonemapping")
	data.EnableVideoToolboxTonemapping = getJSONBool(m, "EnableVideoToolboxTonemapping")
	data.TonemappingAlgorithm = getJSONString(m, "TonemappingAlgorithm")
	data.TonemappingMode = getJSONString(m, "TonemappingMode")
	data.TonemappingRange = getJSONString(m, "TonemappingRange")
	data.TonemappingDesat = getJSONFloat64(m, "TonemappingDesat")
	data.TonemappingPeak = getJSONFloat64(m, "TonemappingPeak")
	data.TonemappingParam = getJSONFloat64(m, "TonemappingParam")
	data.VppTonemappingBrightness = getJSONFloat64(m, "VppTonemappingBrightness")
	data.VppTonemappingContrast = getJSONFloat64(m, "VppTonemappingContrast")
	data.H264Crf = getJSONInt64(m, "H264Crf")
	data.H265Crf = getJSONInt64(m, "H265Crf")
	data.EncoderPreset = getJSONString(m, "EncoderPreset")
	data.DeinterlaceDoubleRate = getJSONBool(m, "DeinterlaceDoubleRate")
	data.DeinterlaceMethod = getJSONString(m, "DeinterlaceMethod")
	data.EnableDecodingColorDepth10Hevc = getJSONBool(m, "EnableDecodingColorDepth10Hevc")
	data.EnableDecodingColorDepth10Vp9 = getJSONBool(m, "EnableDecodingColorDepth10Vp9")
	data.EnableDecodingColorDepth10HevcRext = getJSONBool(m, "EnableDecodingColorDepth10HevcRext")
	data.EnableDecodingColorDepth12HevcRext = getJSONBool(m, "EnableDecodingColorDepth12HevcRext")
	data.EnableEnhancedNvdecDecoder = getJSONBool(m, "EnableEnhancedNvdecDecoder")
	data.PreferSystemNativeHwDecoder = getJSONBool(m, "PreferSystemNativeHwDecoder")
	data.EnableIntelLowPowerH264HwEncoder = getJSONBool(m, "EnableIntelLowPowerH264HwEncoder")
	data.EnableIntelLowPowerHevcHwEncoder = getJSONBool(m, "EnableIntelLowPowerHevcHwEncoder")
	data.EnableHardwareEncoding = getJSONBool(m, "EnableHardwareEncoding")
	data.AllowHevcEncoding = getJSONBool(m, "AllowHevcEncoding")
	data.AllowAv1Encoding = getJSONBool(m, "AllowAv1Encoding")
	data.EnableSubtitleExtraction = getJSONBool(m, "EnableSubtitleExtraction")
	data.HardwareDecodingCodecs, _ = getJSONStringList(ctx, m, "HardwareDecodingCodecs")
	data.AllowOnDemandMetadataBasedKeyframeExtractionForExtensions, _ = getJSONStringList(ctx, m, "AllowOnDemandMetadataBasedKeyframeExtractionForExtensions")
}
