// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

var (
	_ resource.Resource                = &SystemConfigurationResource{}
	_ resource.ResourceWithImportState = &SystemConfigurationResource{}
)

// NewSystemConfigurationResource creates a new system configuration resource.
func NewSystemConfigurationResource() resource.Resource {
	return &SystemConfigurationResource{}
}

// SystemConfigurationResource defines the resource implementation.
type SystemConfigurationResource struct {
	client *client.Client
}

// SystemConfigurationResourceModel describes the resource data model.
type SystemConfigurationResourceModel struct {
	ID                                  types.String `tfsdk:"id"`
	EnableMetrics                       types.Bool   `tfsdk:"enable_metrics"`
	EnableNormalizedItemByNameIds       types.Bool   `tfsdk:"enable_normalized_item_by_name_ids"`
	IsPortAuthorized                    types.Bool   `tfsdk:"is_port_authorized"`
	QuickConnectAvailable               types.Bool   `tfsdk:"quick_connect_available"`
	EnableCaseSensitiveItemIds          types.Bool   `tfsdk:"enable_case_sensitive_item_ids"`
	DisableLiveTvChannelUserDataName    types.Bool   `tfsdk:"disable_live_tv_channel_user_data_name"`
	MetadataPath                        types.String `tfsdk:"metadata_path"`
	PreferredMetadataLanguage           types.String `tfsdk:"preferred_metadata_language"`
	MetadataCountryCode                 types.String `tfsdk:"metadata_country_code"`
	SortReplaceCharacters               types.List   `tfsdk:"sort_replace_characters"`
	SortRemoveCharacters                types.List   `tfsdk:"sort_remove_characters"`
	SortRemoveWords                     types.List   `tfsdk:"sort_remove_words"`
	MinResumePct                        types.Int64  `tfsdk:"min_resume_pct"`
	MaxResumePct                        types.Int64  `tfsdk:"max_resume_pct"`
	MinResumeDurationSeconds            types.Int64  `tfsdk:"min_resume_duration_seconds"`
	MinAudiobookResume                  types.Int64  `tfsdk:"min_audiobook_resume"`
	MaxAudiobookResume                  types.Int64  `tfsdk:"max_audiobook_resume"`
	InactiveSessionThreshold            types.Int64  `tfsdk:"inactive_session_threshold"`
	LibraryMonitorDelay                 types.Int64  `tfsdk:"library_monitor_delay"`
	LibraryUpdateDuration               types.Int64  `tfsdk:"library_update_duration"`
	CacheSize                           types.Int64  `tfsdk:"cache_size"`
	ImageSavingConvention               types.String `tfsdk:"image_saving_convention"`
	MetadataOptions                     types.List   `tfsdk:"metadata_options"`
	SkipDeserializationForBasicTypes    types.Bool   `tfsdk:"skip_deserialization_for_basic_types"`
	UICulture                           types.String `tfsdk:"ui_culture"`
	SaveMetadataHidden                  types.Bool   `tfsdk:"save_metadata_hidden"`
	ContentTypes                        types.List   `tfsdk:"content_types"`
	RemoteClientBitrateLimit            types.Int64  `tfsdk:"remote_client_bitrate_limit"`
	EnableFolderView                    types.Bool   `tfsdk:"enable_folder_view"`
	EnableGroupingMoviesIntoCollections types.Bool   `tfsdk:"enable_grouping_movies_into_collections"`
	EnableGroupingShowsIntoCollections  types.Bool   `tfsdk:"enable_grouping_shows_into_collections"`
	DisplaySpecialsWithinSeasons        types.Bool   `tfsdk:"display_specials_within_seasons"`
	CodecsUsed                          types.List   `tfsdk:"codecs_used"`
	EnableExternalContentInSuggestions  types.Bool   `tfsdk:"enable_external_content_in_suggestions"`
	ImageExtractionTimeoutMs            types.Int64  `tfsdk:"image_extraction_timeout_ms"`
	PathSubstitutions                   types.List   `tfsdk:"path_substitutions"`
	EnableSlowResponseWarning           types.Bool   `tfsdk:"enable_slow_response_warning"`
	SlowResponseThresholdMs             types.Int64  `tfsdk:"slow_response_threshold_ms"`
	CorsHosts                           types.List   `tfsdk:"cors_hosts"`
	ActivityLogRetentionDays            types.Int64  `tfsdk:"activity_log_retention_days"`
	LibraryScanFanoutConcurrency        types.Int64  `tfsdk:"library_scan_fanout_concurrency"`
	LibraryMetadataRefreshConcurrency   types.Int64  `tfsdk:"library_metadata_refresh_concurrency"`
	AllowClientLogUpload                types.Bool   `tfsdk:"allow_client_log_upload"`
	DummyChapterDuration                types.Int64  `tfsdk:"dummy_chapter_duration"`
	ChapterImageResolution              types.String `tfsdk:"chapter_image_resolution"`
	ParallelImageEncodingLimit          types.Int64  `tfsdk:"parallel_image_encoding_limit"`
	CastReceiverApplications            types.List   `tfsdk:"cast_receiver_applications"`
	TrickplayOptions                    types.Object `tfsdk:"trickplay_options"`
	EnableLegacyAuthorization           types.Bool   `tfsdk:"enable_legacy_authorization"`
	LogFileRetentionDays                types.Int64  `tfsdk:"log_file_retention_days"`
	CachePath                           types.String `tfsdk:"cache_path"`
	ServerName                          types.String `tfsdk:"server_name"`
}

// MetadataOptionsModel describes a metadata options entry.
type MetadataOptionsModel struct {
	ItemType                 types.String `tfsdk:"item_type"`
	DisabledMetadataSavers   types.List   `tfsdk:"disabled_metadata_savers"`
	LocalMetadataReaderOrder types.List   `tfsdk:"local_metadata_reader_order"`
	DisabledMetadataFetchers types.List   `tfsdk:"disabled_metadata_fetchers"`
	MetadataFetcherOrder     types.List   `tfsdk:"metadata_fetcher_order"`
	DisabledImageFetchers    types.List   `tfsdk:"disabled_image_fetchers"`
	ImageFetcherOrder        types.List   `tfsdk:"image_fetcher_order"`
}

// NameValuePairModel describes a name/value pair entry.
type NameValuePairModel struct {
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

// PathSubstitutionModel describes a path substitution entry.
type PathSubstitutionModel struct {
	From types.String `tfsdk:"from"`
	To   types.String `tfsdk:"to"`
}

// CastReceiverApplicationModel describes a cast receiver application entry.
type CastReceiverApplicationModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

// TrickplayOptionsModel describes the trickplay options object.
type TrickplayOptionsModel struct {
	EnableHwAcceleration         types.Bool   `tfsdk:"enable_hw_acceleration"`
	EnableHwEncoding             types.Bool   `tfsdk:"enable_hw_encoding"`
	EnableKeyFrameOnlyExtraction types.Bool   `tfsdk:"enable_key_frame_only_extraction"`
	ScanBehavior                 types.String `tfsdk:"scan_behavior"`
	ProcessPriority              types.String `tfsdk:"process_priority"`
	Interval                     types.Int64  `tfsdk:"interval"`
	WidthResolutions             types.List   `tfsdk:"width_resolutions"`
	TileWidth                    types.Int64  `tfsdk:"tile_width"`
	TileHeight                   types.Int64  `tfsdk:"tile_height"`
	Qscale                       types.Int64  `tfsdk:"qscale"`
	JpegQuality                  types.Int64  `tfsdk:"jpeg_quality"`
	ProcessThreads               types.Int64  `tfsdk:"process_threads"`
}

func (r *SystemConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_configuration"
}

func (r *SystemConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optionalString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		}
	}
	optionalBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.UseStateForUnknown(),
			},
		}
	}
	optionalInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		}
	}
	optionalStringList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{
			ElementType:         types.StringType,
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		}
	}
	optionalIntList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{
			ElementType:         types.Int64Type,
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		}
	}

	metadataOptionsAttributes := map[string]schema.Attribute{
		"item_type":                   optionalString("Item type."),
		"disabled_metadata_savers":    optionalStringList("Disabled metadata savers."),
		"local_metadata_reader_order": optionalStringList("Local metadata reader order."),
		"disabled_metadata_fetchers":  optionalStringList("Disabled metadata fetchers."),
		"metadata_fetcher_order":      optionalStringList("Metadata fetcher order."),
		"disabled_image_fetchers":     optionalStringList("Disabled image fetchers."),
		"image_fetcher_order":         optionalStringList("Image fetcher order."),
	}

	nameValuePairAttributes := map[string]schema.Attribute{
		"name":  optionalString("Name."),
		"value": optionalString("Value."),
	}

	pathSubstitutionAttributes := map[string]schema.Attribute{
		"from": optionalString("From path."),
		"to":   optionalString("To path."),
	}

	castReceiverApplicationAttributes := map[string]schema.Attribute{
		"id":   optionalString("Application ID."),
		"name": optionalString("Application name."),
	}

	trickplayOptionsAttributes := map[string]schema.Attribute{
		"enable_hw_acceleration":           optionalBool("Enable hardware acceleration."),
		"enable_hw_encoding":               optionalBool("Enable hardware encoding."),
		"enable_key_frame_only_extraction": optionalBool("Enable key frame only extraction."),
		"scan_behavior":                    optionalString("Scan behavior."),
		"process_priority":                 optionalString("Process priority class."),
		"interval":                         optionalInt("Interval."),
		"width_resolutions":                optionalIntList("Width resolutions."),
		"tile_width":                       optionalInt("Tile width."),
		"tile_height":                      optionalInt("Tile height."),
		"qscale":                           optionalInt("Qscale."),
		"jpeg_quality":                     optionalInt("JPEG quality."),
		"process_threads":                  optionalInt("Process threads."),
	}

	resp.Schema = schema.Schema{
		Description:         "Manages the Jellyfin system configuration.",
		MarkdownDescription: "Manages the Jellyfin system configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "Resource identifier. Always set to `system` for this singleton resource.",
				MarkdownDescription: "Resource identifier. Always set to `system` for this singleton resource.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enable_metrics":                         optionalBool("Enable metrics."),
			"enable_normalized_item_by_name_ids":     optionalBool("Enable normalized item by name IDs."),
			"is_port_authorized":                     optionalBool("Is port authorized."),
			"quick_connect_available":                optionalBool("Quick connect available."),
			"enable_case_sensitive_item_ids":         optionalBool("Enable case sensitive item IDs."),
			"disable_live_tv_channel_user_data_name": optionalBool("Disable live TV channel user data name."),
			"metadata_path":                          optionalString("Metadata path."),
			"preferred_metadata_language":            optionalString("Preferred metadata language."),
			"metadata_country_code":                  optionalString("Metadata country code."),
			"sort_replace_characters":                optionalStringList("Sort replace characters."),
			"sort_remove_characters":                 optionalStringList("Sort remove characters."),
			"sort_remove_words":                      optionalStringList("Sort remove words."),
			"min_resume_pct":                         optionalInt("Minimum resume percentage."),
			"max_resume_pct":                         optionalInt("Maximum resume percentage."),
			"min_resume_duration_seconds":            optionalInt("Minimum resume duration seconds."),
			"min_audiobook_resume":                   optionalInt("Minimum audiobook resume."),
			"max_audiobook_resume":                   optionalInt("Maximum audiobook resume."),
			"inactive_session_threshold":             optionalInt("Inactive session threshold."),
			"library_monitor_delay":                  optionalInt("Library monitor delay."),
			"library_update_duration":                optionalInt("Library update duration."),
			"cache_size":                             optionalInt("Cache size."),
			"image_saving_convention":                optionalString("Image saving convention."),
			"metadata_options": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: metadataOptionsAttributes,
				},
				Description:         "Metadata options.",
				MarkdownDescription: "Metadata options.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"skip_deserialization_for_basic_types": optionalBool("Skip deserialization for basic types."),
			"ui_culture":                           optionalString("UI culture."),
			"save_metadata_hidden":                 optionalBool("Save metadata hidden."),
			"content_types": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: nameValuePairAttributes,
				},
				Description:         "Content types.",
				MarkdownDescription: "Content types.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"remote_client_bitrate_limit":             optionalInt("Remote client bitrate limit."),
			"enable_folder_view":                      optionalBool("Enable folder view."),
			"enable_grouping_movies_into_collections": optionalBool("Enable grouping movies into collections."),
			"enable_grouping_shows_into_collections":  optionalBool("Enable grouping shows into collections."),
			"display_specials_within_seasons":         optionalBool("Display specials within seasons."),
			"codecs_used":                             optionalStringList("Codecs used."),
			"enable_external_content_in_suggestions":  optionalBool("Enable external content in suggestions."),
			"image_extraction_timeout_ms":             optionalInt("Image extraction timeout in milliseconds."),
			"path_substitutions": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: pathSubstitutionAttributes,
				},
				Description:         "Path substitutions.",
				MarkdownDescription: "Path substitutions.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"enable_slow_response_warning":         optionalBool("Enable slow response warning."),
			"slow_response_threshold_ms":           optionalInt("Slow response threshold in milliseconds."),
			"cors_hosts":                           optionalStringList("CORS hosts."),
			"activity_log_retention_days":          optionalInt("Activity log retention days."),
			"library_scan_fanout_concurrency":      optionalInt("Library scan fanout concurrency."),
			"library_metadata_refresh_concurrency": optionalInt("Library metadata refresh concurrency."),
			"allow_client_log_upload":              optionalBool("Allow client log upload."),
			"dummy_chapter_duration":               optionalInt("Dummy chapter duration."),
			"chapter_image_resolution":             optionalString("Chapter image resolution."),
			"parallel_image_encoding_limit":        optionalInt("Parallel image encoding limit."),
			"cast_receiver_applications": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: castReceiverApplicationAttributes,
				},
				Description:         "Cast receiver applications.",
				MarkdownDescription: "Cast receiver applications.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"trickplay_options": schema.SingleNestedAttribute{
				Attributes:          trickplayOptionsAttributes,
				Description:         "Trickplay options.",
				MarkdownDescription: "Trickplay options.",
				Optional:            true,
				Computed:            true,
			},
			"enable_legacy_authorization": optionalBool("Enable legacy authorization."),
			"log_file_retention_days":     optionalInt("Log file retention days."),
			"cache_path":                  optionalString("Cache path."),
			"server_name":                 optionalString("Server display name."),
		},
	}
}

func (r *SystemConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SystemConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SystemConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *SystemConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SystemConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *SystemConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SystemConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *SystemConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// System configuration cannot be deleted. We just remove from state.
}

func (r *SystemConfigurationResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Singleton resource — the import ID is not used. Read will populate all fields.
	data := SystemConfigurationResourceModel{ID: types.StringValue("system")}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SystemConfigurationResource) apply(ctx context.Context, data *SystemConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetSystemConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read current system configuration", err.Error())
		return
	}

	base, err := parseJSONObject(current.RawJSON)
	if err != nil {
		diags.AddError("Failed to parse current system configuration", err.Error())
		return
	}

	if d := overlaySystemConfiguration(ctx, base, data); d.HasError() {
		diags.Append(d...)
		return
	}

	payload, err := json.Marshal(base)
	if err != nil {
		diags.AddError("Failed to serialize system configuration", err.Error())
		return
	}

	if err := r.client.UpdateSystemConfiguration(ctx, &client.SystemConfiguration{RawJSON: string(payload)}); err != nil {
		diags.AddError("Failed to update system configuration", err.Error())
		return
	}

	updated, err := r.client.GetSystemConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read system configuration after update", err.Error())
		return
	}

	flattenSystemConfiguration(ctx, updated.RawJSON, data, diags)
	data.ID = types.StringValue("system")
	diags.Append(state.Set(ctx, data)...)
}

func (r *SystemConfigurationResource) read(ctx context.Context, data *SystemConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetSystemConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read system configuration", err.Error())
		return
	}

	flattenSystemConfiguration(ctx, current.RawJSON, data, diags)
	data.ID = types.StringValue("system")
	diags.Append(state.Set(ctx, data)...)
}

func overlaySystemConfiguration(ctx context.Context, m map[string]json.RawMessage, data *SystemConfigurationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	putJSONBool(m, "EnableMetrics", data.EnableMetrics)
	putJSONBool(m, "EnableNormalizedItemByNameIds", data.EnableNormalizedItemByNameIds)
	putJSONBool(m, "IsPortAuthorized", data.IsPortAuthorized)
	putJSONBool(m, "QuickConnectAvailable", data.QuickConnectAvailable)
	putJSONBool(m, "EnableCaseSensitiveItemIds", data.EnableCaseSensitiveItemIds)
	putJSONBool(m, "DisableLiveTvChannelUserDataName", data.DisableLiveTvChannelUserDataName)
	putJSONString(m, "MetadataPath", data.MetadataPath)
	putJSONString(m, "PreferredMetadataLanguage", data.PreferredMetadataLanguage)
	putJSONString(m, "MetadataCountryCode", data.MetadataCountryCode)
	putJSONStringList(ctx, m, "SortReplaceCharacters", data.SortReplaceCharacters)
	putJSONStringList(ctx, m, "SortRemoveCharacters", data.SortRemoveCharacters)
	putJSONStringList(ctx, m, "SortRemoveWords", data.SortRemoveWords)
	putJSONInt64(m, "MinResumePct", data.MinResumePct)
	putJSONInt64(m, "MaxResumePct", data.MaxResumePct)
	putJSONInt64(m, "MinResumeDurationSeconds", data.MinResumeDurationSeconds)
	putJSONInt64(m, "MinAudiobookResume", data.MinAudiobookResume)
	putJSONInt64(m, "MaxAudiobookResume", data.MaxAudiobookResume)
	putJSONInt64(m, "InactiveSessionThreshold", data.InactiveSessionThreshold)
	putJSONInt64(m, "LibraryMonitorDelay", data.LibraryMonitorDelay)
	putJSONInt64(m, "LibraryUpdateDuration", data.LibraryUpdateDuration)
	putJSONInt64(m, "CacheSize", data.CacheSize)
	putJSONString(m, "ImageSavingConvention", data.ImageSavingConvention)
	if d := overlayMetadataOptions(ctx, m, data.MetadataOptions); d.HasError() {
		return append(diags, d...)
	}
	putJSONBool(m, "SkipDeserializationForBasicTypes", data.SkipDeserializationForBasicTypes)
	putJSONString(m, "UICulture", data.UICulture)
	putJSONBool(m, "SaveMetadataHidden", data.SaveMetadataHidden)
	if d := overlayContentTypes(ctx, m, data.ContentTypes); d.HasError() {
		return append(diags, d...)
	}
	putJSONInt64(m, "RemoteClientBitrateLimit", data.RemoteClientBitrateLimit)
	putJSONBool(m, "EnableFolderView", data.EnableFolderView)
	putJSONBool(m, "EnableGroupingMoviesIntoCollections", data.EnableGroupingMoviesIntoCollections)
	putJSONBool(m, "EnableGroupingShowsIntoCollections", data.EnableGroupingShowsIntoCollections)
	putJSONBool(m, "DisplaySpecialsWithinSeasons", data.DisplaySpecialsWithinSeasons)
	putJSONStringList(ctx, m, "CodecsUsed", data.CodecsUsed)
	putJSONBool(m, "EnableExternalContentInSuggestions", data.EnableExternalContentInSuggestions)
	putJSONInt64(m, "ImageExtractionTimeoutMs", data.ImageExtractionTimeoutMs)
	if d := overlayPathSubstitutions(ctx, m, data.PathSubstitutions); d.HasError() {
		return append(diags, d...)
	}
	putJSONBool(m, "EnableSlowResponseWarning", data.EnableSlowResponseWarning)
	putJSONInt64(m, "SlowResponseThresholdMs", data.SlowResponseThresholdMs)
	putJSONStringList(ctx, m, "CorsHosts", data.CorsHosts)
	putJSONInt64(m, "ActivityLogRetentionDays", data.ActivityLogRetentionDays)
	putJSONInt64(m, "LibraryScanFanoutConcurrency", data.LibraryScanFanoutConcurrency)
	putJSONInt64(m, "LibraryMetadataRefreshConcurrency", data.LibraryMetadataRefreshConcurrency)
	putJSONBool(m, "AllowClientLogUpload", data.AllowClientLogUpload)
	putJSONInt64(m, "DummyChapterDuration", data.DummyChapterDuration)
	putJSONString(m, "ChapterImageResolution", data.ChapterImageResolution)
	putJSONInt64(m, "ParallelImageEncodingLimit", data.ParallelImageEncodingLimit)
	if d := overlayCastReceiverApplications(ctx, m, data.CastReceiverApplications); d.HasError() {
		return append(diags, d...)
	}
	if d := overlayTrickplayOptions(ctx, m, data.TrickplayOptions); d.HasError() {
		return append(diags, d...)
	}
	putJSONBool(m, "EnableLegacyAuthorization", data.EnableLegacyAuthorization)
	putJSONInt64(m, "LogFileRetentionDays", data.LogFileRetentionDays)
	putJSONString(m, "CachePath", data.CachePath)
	putJSONString(m, "ServerName", data.ServerName)

	return diags
}

func overlayMetadataOptions(ctx context.Context, m map[string]json.RawMessage, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}
	var options []MetadataOptionsModel
	if d := v.ElementsAs(ctx, &options, false); d.HasError() {
		return append(diags, d...)
	}
	entries := make([]map[string]json.RawMessage, len(options))
	for i, o := range options {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "ItemType", o.ItemType)
		putJSONStringList(ctx, entry, "DisabledMetadataSavers", o.DisabledMetadataSavers)
		putJSONStringList(ctx, entry, "LocalMetadataReaderOrder", o.LocalMetadataReaderOrder)
		putJSONStringList(ctx, entry, "DisabledMetadataFetchers", o.DisabledMetadataFetchers)
		putJSONStringList(ctx, entry, "MetadataFetcherOrder", o.MetadataFetcherOrder)
		putJSONStringList(ctx, entry, "DisabledImageFetchers", o.DisabledImageFetchers)
		putJSONStringList(ctx, entry, "ImageFetcherOrder", o.ImageFetcherOrder)
		entries[i] = entry
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal metadata options", err.Error()))
	}
	m["MetadataOptions"] = b
	return diags
}

func overlayNameValuePairs(ctx context.Context, m map[string]json.RawMessage, key string, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}
	var pairs []NameValuePairModel
	if d := v.ElementsAs(ctx, &pairs, false); d.HasError() {
		return append(diags, d...)
	}
	entries := make([]map[string]json.RawMessage, len(pairs))
	for i, p := range pairs {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "Name", p.Name)
		putJSONString(entry, "Value", p.Value)
		entries[i] = entry
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal name/value pairs", err.Error()))
	}
	m[key] = b
	return diags
}

func overlayContentTypes(ctx context.Context, m map[string]json.RawMessage, v types.List) diag.Diagnostics {
	return overlayNameValuePairs(ctx, m, "ContentTypes", v)
}

func overlayPathSubstitutions(ctx context.Context, m map[string]json.RawMessage, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}
	var substitutions []PathSubstitutionModel
	if d := v.ElementsAs(ctx, &substitutions, false); d.HasError() {
		return append(diags, d...)
	}
	entries := make([]map[string]json.RawMessage, len(substitutions))
	for i, s := range substitutions {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "From", s.From)
		putJSONString(entry, "To", s.To)
		entries[i] = entry
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal path substitutions", err.Error()))
	}
	m["PathSubstitutions"] = b
	return diags
}

func overlayCastReceiverApplications(ctx context.Context, m map[string]json.RawMessage, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}
	var apps []CastReceiverApplicationModel
	if d := v.ElementsAs(ctx, &apps, false); d.HasError() {
		return append(diags, d...)
	}
	entries := make([]map[string]json.RawMessage, len(apps))
	for i, a := range apps {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "Id", a.ID)
		putJSONString(entry, "Name", a.Name)
		entries[i] = entry
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal cast receiver applications", err.Error()))
	}
	m["CastReceiverApplications"] = b
	return diags
}

func overlayTrickplayOptions(ctx context.Context, m map[string]json.RawMessage, v types.Object) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}
	var opts TrickplayOptionsModel
	if d := tfsdk.ValueAs(ctx, v, &opts); d.HasError() {
		return append(diags, d...)
	}
	entry := map[string]json.RawMessage{}
	putJSONBool(entry, "EnableHwAcceleration", opts.EnableHwAcceleration)
	putJSONBool(entry, "EnableHwEncoding", opts.EnableHwEncoding)
	putJSONBool(entry, "EnableKeyFrameOnlyExtraction", opts.EnableKeyFrameOnlyExtraction)
	putJSONString(entry, "ScanBehavior", opts.ScanBehavior)
	putJSONString(entry, "ProcessPriorityClass", opts.ProcessPriority)
	putJSONInt64(entry, "Interval", opts.Interval)
	putJSONInt64List(ctx, entry, "WidthResolutions", opts.WidthResolutions)
	putJSONInt64(entry, "TileWidth", opts.TileWidth)
	putJSONInt64(entry, "TileHeight", opts.TileHeight)
	putJSONInt64(entry, "Qscale", opts.Qscale)
	putJSONInt64(entry, "JpegQuality", opts.JpegQuality)
	putJSONInt64(entry, "ProcessThreads", opts.ProcessThreads)
	b, err := json.Marshal(entry)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal trickplay options", err.Error()))
	}
	m["TrickplayOptions"] = b
	return diags
}

func flattenSystemConfiguration(ctx context.Context, raw string, data *SystemConfigurationResourceModel, diags *diag.Diagnostics) {
	m, err := parseJSONObject(raw)
	if err != nil {
		diags.AddError("Failed to parse system configuration", err.Error())
		return
	}

	data.EnableMetrics = getJSONBool(m, "EnableMetrics")
	data.EnableNormalizedItemByNameIds = getJSONBool(m, "EnableNormalizedItemByNameIds")
	data.IsPortAuthorized = getJSONBool(m, "IsPortAuthorized")
	data.QuickConnectAvailable = getJSONBool(m, "QuickConnectAvailable")
	data.EnableCaseSensitiveItemIds = getJSONBool(m, "EnableCaseSensitiveItemIds")
	data.DisableLiveTvChannelUserDataName = getJSONBool(m, "DisableLiveTvChannelUserDataName")
	data.MetadataPath = getJSONString(m, "MetadataPath")
	data.PreferredMetadataLanguage = getJSONString(m, "PreferredMetadataLanguage")
	data.MetadataCountryCode = getJSONString(m, "MetadataCountryCode")
	data.SortReplaceCharacters, _ = getJSONStringList(ctx, m, "SortReplaceCharacters")
	data.SortRemoveCharacters, _ = getJSONStringList(ctx, m, "SortRemoveCharacters")
	data.SortRemoveWords, _ = getJSONStringList(ctx, m, "SortRemoveWords")
	data.MinResumePct = getJSONInt64(m, "MinResumePct")
	data.MaxResumePct = getJSONInt64(m, "MaxResumePct")
	data.MinResumeDurationSeconds = getJSONInt64(m, "MinResumeDurationSeconds")
	data.MinAudiobookResume = getJSONInt64(m, "MinAudiobookResume")
	data.MaxAudiobookResume = getJSONInt64(m, "MaxAudiobookResume")
	data.InactiveSessionThreshold = getJSONInt64(m, "InactiveSessionThreshold")
	data.LibraryMonitorDelay = getJSONInt64(m, "LibraryMonitorDelay")
	data.LibraryUpdateDuration = getJSONInt64(m, "LibraryUpdateDuration")
	data.CacheSize = getJSONInt64(m, "CacheSize")
	data.ImageSavingConvention = getJSONString(m, "ImageSavingConvention")
	data.MetadataOptions = flattenMetadataOptions(ctx, m, diags)
	data.SkipDeserializationForBasicTypes = getJSONBool(m, "SkipDeserializationForBasicTypes")
	data.UICulture = getJSONString(m, "UICulture")
	data.SaveMetadataHidden = getJSONBool(m, "SaveMetadataHidden")
	data.ContentTypes = flattenNameValuePairs(ctx, m, "ContentTypes", diags)
	data.RemoteClientBitrateLimit = getJSONInt64(m, "RemoteClientBitrateLimit")
	data.EnableFolderView = getJSONBool(m, "EnableFolderView")
	data.EnableGroupingMoviesIntoCollections = getJSONBool(m, "EnableGroupingMoviesIntoCollections")
	data.EnableGroupingShowsIntoCollections = getJSONBool(m, "EnableGroupingShowsIntoCollections")
	data.DisplaySpecialsWithinSeasons = getJSONBool(m, "DisplaySpecialsWithinSeasons")
	data.CodecsUsed, _ = getJSONStringList(ctx, m, "CodecsUsed")
	data.EnableExternalContentInSuggestions = getJSONBool(m, "EnableExternalContentInSuggestions")
	data.ImageExtractionTimeoutMs = getJSONInt64(m, "ImageExtractionTimeoutMs")
	data.PathSubstitutions = flattenPathSubstitutions(ctx, m, diags)
	data.EnableSlowResponseWarning = getJSONBool(m, "EnableSlowResponseWarning")
	data.SlowResponseThresholdMs = getJSONInt64(m, "SlowResponseThresholdMs")
	data.CorsHosts, _ = getJSONStringList(ctx, m, "CorsHosts")
	data.ActivityLogRetentionDays = getJSONInt64(m, "ActivityLogRetentionDays")
	data.LibraryScanFanoutConcurrency = getJSONInt64(m, "LibraryScanFanoutConcurrency")
	data.LibraryMetadataRefreshConcurrency = getJSONInt64(m, "LibraryMetadataRefreshConcurrency")
	data.AllowClientLogUpload = getJSONBool(m, "AllowClientLogUpload")
	data.DummyChapterDuration = getJSONInt64(m, "DummyChapterDuration")
	data.ChapterImageResolution = getJSONString(m, "ChapterImageResolution")
	data.ParallelImageEncodingLimit = getJSONInt64(m, "ParallelImageEncodingLimit")
	data.CastReceiverApplications = flattenCastReceiverApplications(ctx, m, diags)
	data.TrickplayOptions = flattenTrickplayOptions(ctx, m, diags)
	data.EnableLegacyAuthorization = getJSONBool(m, "EnableLegacyAuthorization")
	data.LogFileRetentionDays = getJSONInt64(m, "LogFileRetentionDays")
	data.CachePath = getJSONString(m, "CachePath")
	data.ServerName = getJSONString(m, "ServerName")
}

func flattenMetadataOptions(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["MetadataOptions"]
	if !ok {
		return types.ListNull(metadataOptionsObjectType())
	}
	if isJSONNull(raw) {
		return types.ListNull(metadataOptionsObjectType())
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse metadata options", err.Error())
		return types.ListNull(metadataOptionsObjectType())
	}
	objType := metadataOptionsObjectType()
	objects := make([]attr.Value, len(entries))
	for i, entry := range entries {
		attrs := map[string]attr.Value{
			"item_type":                   getJSONString(entry, "ItemType"),
			"disabled_metadata_savers":    nullStringList(),
			"local_metadata_reader_order": nullStringList(),
			"disabled_metadata_fetchers":  nullStringList(),
			"metadata_fetcher_order":      nullStringList(),
			"disabled_image_fetchers":     nullStringList(),
			"image_fetcher_order":         nullStringList(),
		}
		if v, d := getJSONStringList(ctx, entry, "DisabledMetadataSavers"); !d.HasError() {
			attrs["disabled_metadata_savers"] = v
		}
		if v, d := getJSONStringList(ctx, entry, "LocalMetadataReaderOrder"); !d.HasError() {
			attrs["local_metadata_reader_order"] = v
		}
		if v, d := getJSONStringList(ctx, entry, "DisabledMetadataFetchers"); !d.HasError() {
			attrs["disabled_metadata_fetchers"] = v
		}
		if v, d := getJSONStringList(ctx, entry, "MetadataFetcherOrder"); !d.HasError() {
			attrs["metadata_fetcher_order"] = v
		}
		if v, d := getJSONStringList(ctx, entry, "DisabledImageFetchers"); !d.HasError() {
			attrs["disabled_image_fetchers"] = v
		}
		if v, d := getJSONStringList(ctx, entry, "ImageFetcherOrder"); !d.HasError() {
			attrs["image_fetcher_order"] = v
		}
		obj, d := types.ObjectValue(objType.AttrTypes, attrs)
		if d.HasError() {
			diags.Append(d...)
			return types.ListNull(objType)
		}
		objects[i] = obj
	}
	list, d := types.ListValue(objType, objects)
	if d.HasError() {
		diags.Append(d...)
		return types.ListNull(objType)
	}
	return list
}

func metadataOptionsObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"item_type":                   types.StringType,
		"disabled_metadata_savers":    types.ListType{ElemType: types.StringType},
		"local_metadata_reader_order": types.ListType{ElemType: types.StringType},
		"disabled_metadata_fetchers":  types.ListType{ElemType: types.StringType},
		"metadata_fetcher_order":      types.ListType{ElemType: types.StringType},
		"disabled_image_fetchers":     types.ListType{ElemType: types.StringType},
		"image_fetcher_order":         types.ListType{ElemType: types.StringType},
	}}
}

func flattenNameValuePairs(ctx context.Context, m map[string]json.RawMessage, key string, diags *diag.Diagnostics) types.List {
	raw, ok := m[key]
	if !ok {
		return types.ListNull(nameValuePairObjectType())
	}
	if isJSONNull(raw) {
		return types.ListNull(nameValuePairObjectType())
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse name/value pairs", err.Error())
		return types.ListNull(nameValuePairObjectType())
	}
	objType := nameValuePairObjectType()
	objects := make([]attr.Value, len(entries))
	for i, entry := range entries {
		attrs := map[string]attr.Value{
			"name":  getJSONString(entry, "Name"),
			"value": getJSONString(entry, "Value"),
		}
		obj, d := types.ObjectValue(objType.AttrTypes, attrs)
		if d.HasError() {
			diags.Append(d...)
			return types.ListNull(objType)
		}
		objects[i] = obj
	}
	list, d := types.ListValue(objType, objects)
	if d.HasError() {
		diags.Append(d...)
		return types.ListNull(objType)
	}
	return list
}

func nameValuePairObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":  types.StringType,
		"value": types.StringType,
	}}
}

func flattenPathSubstitutions(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["PathSubstitutions"]
	if !ok {
		return types.ListNull(pathSubstitutionObjectType())
	}
	if isJSONNull(raw) {
		return types.ListNull(pathSubstitutionObjectType())
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse path substitutions", err.Error())
		return types.ListNull(pathSubstitutionObjectType())
	}
	objType := pathSubstitutionObjectType()
	objects := make([]attr.Value, len(entries))
	for i, entry := range entries {
		attrs := map[string]attr.Value{
			"from": getJSONString(entry, "From"),
			"to":   getJSONString(entry, "To"),
		}
		obj, d := types.ObjectValue(objType.AttrTypes, attrs)
		if d.HasError() {
			diags.Append(d...)
			return types.ListNull(objType)
		}
		objects[i] = obj
	}
	list, d := types.ListValue(objType, objects)
	if d.HasError() {
		diags.Append(d...)
		return types.ListNull(objType)
	}
	return list
}

func pathSubstitutionObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"from": types.StringType,
		"to":   types.StringType,
	}}
}

func flattenCastReceiverApplications(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["CastReceiverApplications"]
	if !ok {
		return types.ListNull(castReceiverApplicationObjectType())
	}
	if isJSONNull(raw) {
		return types.ListNull(castReceiverApplicationObjectType())
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse cast receiver applications", err.Error())
		return types.ListNull(castReceiverApplicationObjectType())
	}
	objType := castReceiverApplicationObjectType()
	objects := make([]attr.Value, len(entries))
	for i, entry := range entries {
		attrs := map[string]attr.Value{
			"id":   getJSONString(entry, "Id"),
			"name": getJSONString(entry, "Name"),
		}
		obj, d := types.ObjectValue(objType.AttrTypes, attrs)
		if d.HasError() {
			diags.Append(d...)
			return types.ListNull(objType)
		}
		objects[i] = obj
	}
	list, d := types.ListValue(objType, objects)
	if d.HasError() {
		diags.Append(d...)
		return types.ListNull(objType)
	}
	return list
}

func castReceiverApplicationObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":   types.StringType,
		"name": types.StringType,
	}}
}

func flattenTrickplayOptions(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.Object {
	raw, ok := m["TrickplayOptions"]
	if !ok {
		return types.ObjectNull(trickplayOptionsObjectType().AttrTypes)
	}
	if isJSONNull(raw) {
		return types.ObjectNull(trickplayOptionsObjectType().AttrTypes)
	}
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil {
		diags.AddError("Failed to parse trickplay options", err.Error())
		return types.ObjectNull(trickplayOptionsObjectType().AttrTypes)
	}
	objType := trickplayOptionsObjectType()
	widthResolutions, _ := getJSONInt64List(ctx, entry, "WidthResolutions")
	attrs := map[string]attr.Value{
		"enable_hw_acceleration":           getJSONBool(entry, "EnableHwAcceleration"),
		"enable_hw_encoding":               getJSONBool(entry, "EnableHwEncoding"),
		"enable_key_frame_only_extraction": getJSONBool(entry, "EnableKeyFrameOnlyExtraction"),
		"scan_behavior":                    getJSONString(entry, "ScanBehavior"),
		"process_priority":                 getJSONString(entry, "ProcessPriorityClass"),
		"interval":                         getJSONInt64(entry, "Interval"),
		"width_resolutions":                widthResolutions,
		"tile_width":                       getJSONInt64(entry, "TileWidth"),
		"tile_height":                      getJSONInt64(entry, "TileHeight"),
		"qscale":                           getJSONInt64(entry, "Qscale"),
		"jpeg_quality":                     getJSONInt64(entry, "JpegQuality"),
		"process_threads":                  getJSONInt64(entry, "ProcessThreads"),
	}
	obj, d := types.ObjectValue(objType.AttrTypes, attrs)
	if d.HasError() {
		diags.Append(d...)
		return types.ObjectNull(objType.AttrTypes)
	}
	return obj
}

func trickplayOptionsObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"enable_hw_acceleration":           types.BoolType,
		"enable_hw_encoding":               types.BoolType,
		"enable_key_frame_only_extraction": types.BoolType,
		"scan_behavior":                    types.StringType,
		"process_priority":                 types.StringType,
		"interval":                         types.Int64Type,
		"width_resolutions":                types.ListType{ElemType: types.Int64Type},
		"tile_width":                       types.Int64Type,
		"tile_height":                      types.Int64Type,
		"qscale":                           types.Int64Type,
		"jpeg_quality":                     types.Int64Type,
		"process_threads":                  types.Int64Type,
	}}
}

// normalizeJSON re-encodes JSON to remove insignificant formatting and sort object keys.
// Kept for plugin_configuration_resource.go compatibility.
func normalizeJSON(raw string) (string, error) {
	normalized, err := normalizeJSONRecursive(json.RawMessage(raw), 0)
	if err != nil {
		return "", fmt.Errorf("parsing JSON for normalization: %w", err)
	}
	return string(normalized), nil
}

const maxJSONNormalizeDepth = 100

func normalizeJSONRecursive(raw json.RawMessage, depth int) (json.RawMessage, error) {
	if depth > maxJSONNormalizeDepth {
		return nil, fmt.Errorf("JSON nesting exceeds maximum depth of %d", maxJSONNormalizeDepth)
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, err
	}

	trimmed := bytes.TrimSpace(compact.Bytes())
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty JSON value")
	}

	switch trimmed[0] {
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &object); err != nil {
			return nil, err
		}
		for key, value := range object {
			normalized, err := normalizeJSONRecursive(value, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = normalized
		}
		return json.Marshal(object)
	case '[':
		var list []json.RawMessage
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return nil, err
		}
		for i, value := range list {
			normalized, err := normalizeJSONRecursive(value, depth+1)
			if err != nil {
				return nil, err
			}
			list[i] = normalized
		}
		return json.Marshal(list)
	}

	var rawValue json.RawMessage
	if err := json.Unmarshal(trimmed, &rawValue); err != nil {
		return nil, err
	}
	result, err := json.Marshal(rawValue)
	if err != nil {
		return nil, err
	}
	return result, nil
}
