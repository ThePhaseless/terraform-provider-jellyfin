// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
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
	_ resource.Resource                = &LiveTVConfigurationResource{}
	_ resource.ResourceWithImportState = &LiveTVConfigurationResource{}
)

// NewLiveTVConfigurationResource creates a new Live TV configuration resource.
func NewLiveTVConfigurationResource() resource.Resource {
	return &LiveTVConfigurationResource{}
}

// LiveTVConfigurationResource defines the resource implementation.
type LiveTVConfigurationResource struct {
	client *client.Client
}

// LiveTVConfigurationResourceModel describes the resource data model.
type LiveTVConfigurationResourceModel struct {
	ID                                       types.String `tfsdk:"id"`
	GuideDays                                types.Int64  `tfsdk:"guide_days"`
	RecordingPath                            types.String `tfsdk:"recording_path"`
	MovieRecordingPath                       types.String `tfsdk:"movie_recording_path"`
	SeriesRecordingPath                      types.String `tfsdk:"series_recording_path"`
	EnableRecordingSubfolders                types.Bool   `tfsdk:"enable_recording_subfolders"`
	EnableOriginalAudioWithEncodedRecordings types.Bool   `tfsdk:"enable_original_audio_with_encoded_recordings"`
	TunerHosts                               types.List   `tfsdk:"tuner_hosts"`
	ListingProviders                         types.List   `tfsdk:"listing_providers"`
	PrePaddingSeconds                        types.Int64  `tfsdk:"pre_padding_seconds"`
	PostPaddingSeconds                       types.Int64  `tfsdk:"post_padding_seconds"`
	MediaLocationsCreated                    types.List   `tfsdk:"media_locations_created"`
	RecordingPostProcessor                   types.String `tfsdk:"recording_post_processor"`
	RecordingPostProcessorArguments          types.String `tfsdk:"recording_post_processor_arguments"`
	SaveRecordingNFO                         types.Bool   `tfsdk:"save_recording_nfo"`
	SaveRecordingImages                      types.Bool   `tfsdk:"save_recording_images"`
}

// TunerHostModel describes a tuner host entry.
type TunerHostModel struct {
	ID                            types.String `tfsdk:"id"`
	URL                           types.String `tfsdk:"url"`
	Type                          types.String `tfsdk:"type"`
	DeviceID                      types.String `tfsdk:"device_id"`
	FriendlyName                  types.String `tfsdk:"friendly_name"`
	ImportFavoritesOnly           types.Bool   `tfsdk:"import_favorites_only"`
	AllowHWTranscoding            types.Bool   `tfsdk:"allow_hw_transcoding"`
	AllowFmp4TranscodingContainer types.Bool   `tfsdk:"allow_fmp4_transcoding_container"`
	AllowStreamSharing            types.Bool   `tfsdk:"allow_stream_sharing"`
	FallbackMaxStreamingBitrate   types.Int64  `tfsdk:"fallback_max_streaming_bitrate"`
	EnableStreamLooping           types.Bool   `tfsdk:"enable_stream_looping"`
	Source                        types.String `tfsdk:"source"`
	TunerCount                    types.Int64  `tfsdk:"tuner_count"`
	UserAgent                     types.String `tfsdk:"user_agent"`
	IgnoreDts                     types.Bool   `tfsdk:"ignore_dts"`
	ReadAtNativeFramerate         types.Bool   `tfsdk:"read_at_native_framerate"`
}

// ListingProviderModel describes a listings provider entry.
type ListingProviderModel struct {
	ID                types.String `tfsdk:"id"`
	Type              types.String `tfsdk:"type"`
	Username          types.String `tfsdk:"username"`
	Password          types.String `tfsdk:"password"`
	ListingsID        types.String `tfsdk:"listings_id"`
	ZipCode           types.String `tfsdk:"zip_code"`
	Country           types.String `tfsdk:"country"`
	Path              types.String `tfsdk:"path"`
	EnabledTuners     types.List   `tfsdk:"enabled_tuners"`
	EnableAllTuners   types.Bool   `tfsdk:"enable_all_tuners"`
	NewsCategories    types.List   `tfsdk:"news_categories"`
	SportsCategories  types.List   `tfsdk:"sports_categories"`
	KidsCategories    types.List   `tfsdk:"kids_categories"`
	MovieCategories   types.List   `tfsdk:"movie_categories"`
	ChannelMappings   types.List   `tfsdk:"channel_mappings"`
	MoviePrefix       types.String `tfsdk:"movie_prefix"`
	PreferredLanguage types.String `tfsdk:"preferred_language"`
	UserAgent         types.String `tfsdk:"user_agent"`
}

// ChannelMappingModel describes a channel mapping entry.
type ChannelMappingModel struct {
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

func (r *LiveTVConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_livetv_configuration"
}

func (r *LiveTVConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages the Jellyfin Live TV configuration.",
		MarkdownDescription: "Manages the Jellyfin Live TV configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "Resource identifier. Always set to `livetv` for this singleton resource.",
				MarkdownDescription: "Resource identifier. Always set to `livetv` for this singleton resource.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"guide_days": schema.Int64Attribute{
				Description:         "Number of guide days.",
				MarkdownDescription: "Number of guide days.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"recording_path": schema.StringAttribute{
				Description:         "Recording path.",
				MarkdownDescription: "Recording path.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"movie_recording_path": schema.StringAttribute{
				Description:         "Movie recording path.",
				MarkdownDescription: "Movie recording path.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"series_recording_path": schema.StringAttribute{
				Description:         "Series recording path.",
				MarkdownDescription: "Series recording path.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enable_recording_subfolders": schema.BoolAttribute{
				Description:         "Whether recording subfolders are enabled.",
				MarkdownDescription: "Whether recording subfolders are enabled.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"enable_original_audio_with_encoded_recordings": schema.BoolAttribute{
				Description:         "Whether original audio is kept with encoded recordings.",
				MarkdownDescription: "Whether original audio is kept with encoded recordings.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"tuner_hosts": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: tunerHostAttributes(),
				},
				Description:         "Tuner hosts.",
				MarkdownDescription: "Tuner hosts.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"listing_providers": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: listingProviderAttributes(),
				},
				Description:         "Listing providers.",
				MarkdownDescription: "Listing providers.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"pre_padding_seconds": schema.Int64Attribute{
				Description:         "Pre-padding seconds.",
				MarkdownDescription: "Pre-padding seconds.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"post_padding_seconds": schema.Int64Attribute{
				Description:         "Post-padding seconds.",
				MarkdownDescription: "Post-padding seconds.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"media_locations_created": schema.ListAttribute{
				ElementType:         types.StringType,
				Description:         "Media locations created.",
				MarkdownDescription: "Media locations created.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"recording_post_processor": schema.StringAttribute{
				Description:         "Recording post processor.",
				MarkdownDescription: "Recording post processor.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"recording_post_processor_arguments": schema.StringAttribute{
				Description:         "Recording post processor arguments.",
				MarkdownDescription: "Recording post processor arguments.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"save_recording_nfo": schema.BoolAttribute{
				Description:         "Whether to save recording NFO.",
				MarkdownDescription: "Whether to save recording NFO.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"save_recording_images": schema.BoolAttribute{
				Description:         "Whether to save recording images.",
				MarkdownDescription: "Whether to save recording images.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func tunerHostAttributes() map[string]schema.Attribute {
	optionalString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Description: desc, MarkdownDescription: desc, Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}
	}
	optionalBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Description: desc, MarkdownDescription: desc, Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}}
	}
	optionalInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{Description: desc, MarkdownDescription: desc, Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}}
	}
	return map[string]schema.Attribute{
		"id":                               optionalString("Tuner host ID."),
		"url":                              optionalString("Tuner host URL."),
		"type":                             optionalString("Tuner host type."),
		"device_id":                        optionalString("Device ID."),
		"friendly_name":                    optionalString("Friendly name."),
		"import_favorites_only":            optionalBool("Import favorites only."),
		"allow_hw_transcoding":             optionalBool("Allow hardware transcoding."),
		"allow_fmp4_transcoding_container": optionalBool("Allow fmp4 transcoding container."),
		"allow_stream_sharing":             optionalBool("Allow stream sharing."),
		"fallback_max_streaming_bitrate":   optionalInt("Fallback max streaming bitrate."),
		"enable_stream_looping":            optionalBool("Enable stream looping."),
		"source":                           optionalString("Source."),
		"tuner_count":                      optionalInt("Tuner count."),
		"user_agent":                       optionalString("User agent."),
		"ignore_dts":                       optionalBool("Ignore DTS."),
		"read_at_native_framerate":         optionalBool("Read at native framerate."),
	}
}

func listingProviderAttributes() map[string]schema.Attribute {
	optionalString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Description: desc, MarkdownDescription: desc, Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}
	}
	optionalBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Description: desc, MarkdownDescription: desc, Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}}
	}
	optionalStringList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{ElementType: types.StringType, Description: desc, MarkdownDescription: desc, Optional: true, Computed: true, PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}}
	}
	return map[string]schema.Attribute{
		"id":       optionalString("Provider ID."),
		"type":     optionalString("Provider type."),
		"username": optionalString("Username."),
		"password": schema.StringAttribute{
			Description: "Password.", MarkdownDescription: "Password.",
			Optional: true, Computed: true, Sensitive: true,
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"listings_id":       optionalString("Listings ID."),
		"zip_code":          optionalString("ZIP code."),
		"country":           optionalString("Country."),
		"path":              optionalString("Path."),
		"enabled_tuners":    optionalStringList("Enabled tuners."),
		"enable_all_tuners": optionalBool("Enable all tuners."),
		"news_categories":   optionalStringList("News categories."),
		"sports_categories": optionalStringList("Sports categories."),
		"kids_categories":   optionalStringList("Kids categories."),
		"movie_categories":  optionalStringList("Movie categories."),
		"channel_mappings": schema.ListNestedAttribute{
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"name":  optionalString("Channel name."),
					"value": optionalString("Mapped value."),
				},
			},
			Description: "Channel mappings.", MarkdownDescription: "Channel mappings.",
			Optional: true, Computed: true,
			PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
		},
		"movie_prefix":       optionalString("Movie prefix."),
		"preferred_language": optionalString("Preferred language."),
		"user_agent":         optionalString("User agent."),
	}
}

func (r *LiveTVConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *LiveTVConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data LiveTVConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *LiveTVConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data LiveTVConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *LiveTVConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data LiveTVConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *LiveTVConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Live TV configuration cannot be deleted. We just remove from state.
}

func (r *LiveTVConfigurationResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	data := LiveTVConfigurationResourceModel{ID: types.StringValue("livetv")}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LiveTVConfigurationResource) apply(ctx context.Context, data *LiveTVConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetLiveTVConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read current Live TV configuration", err.Error())
		return
	}

	base, err := parseJSONObject(current.RawJSON)
	if err != nil {
		diags.AddError("Failed to parse current Live TV configuration", err.Error())
		return
	}

	d := overlayLiveTVConfiguration(ctx, base, data)
	if d.HasError() {
		diags.Append(d...)
		return
	}

	payload, err := json.Marshal(base)
	if err != nil {
		diags.AddError("Failed to serialize Live TV configuration", err.Error())
		return
	}

	if err := r.client.UpdateLiveTVConfiguration(ctx, &client.LiveTVConfiguration{RawJSON: string(payload)}); err != nil {
		diags.AddError("Failed to update Live TV configuration", err.Error())
		return
	}

	updated, err := r.client.GetLiveTVConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read Live TV configuration after update", err.Error())
		return
	}

	flattenLiveTVConfiguration(ctx, updated.RawJSON, data, diags)
	data.ID = types.StringValue("livetv")
	diags.Append(state.Set(ctx, data)...)
}

func (r *LiveTVConfigurationResource) read(ctx context.Context, data *LiveTVConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetLiveTVConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read Live TV configuration", err.Error())
		return
	}

	flattenLiveTVConfiguration(ctx, current.RawJSON, data, diags)
	data.ID = types.StringValue("livetv")
	diags.Append(state.Set(ctx, data)...)
}

func overlayLiveTVConfiguration(ctx context.Context, m map[string]json.RawMessage, data *LiveTVConfigurationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	putJSONInt64(m, "GuideDays", data.GuideDays)
	putJSONString(m, "RecordingPath", data.RecordingPath)
	putJSONString(m, "MovieRecordingPath", data.MovieRecordingPath)
	putJSONString(m, "SeriesRecordingPath", data.SeriesRecordingPath)
	putJSONBool(m, "EnableRecordingSubfolders", data.EnableRecordingSubfolders)
	putJSONBool(m, "EnableOriginalAudioWithEncodedRecordings", data.EnableOriginalAudioWithEncodedRecordings)
	if d := overlayTunerHosts(ctx, m, data.TunerHosts); d.HasError() {
		return append(diags, d...)
	}
	if d := overlayListingProviders(ctx, m, data.ListingProviders); d.HasError() {
		return append(diags, d...)
	}
	putJSONInt64(m, "PrePaddingSeconds", data.PrePaddingSeconds)
	putJSONInt64(m, "PostPaddingSeconds", data.PostPaddingSeconds)
	putJSONStringList(ctx, m, "MediaLocationsCreated", data.MediaLocationsCreated)
	putJSONString(m, "RecordingPostProcessor", data.RecordingPostProcessor)
	putJSONString(m, "RecordingPostProcessorArguments", data.RecordingPostProcessorArguments)
	putJSONBool(m, "SaveRecordingNFO", data.SaveRecordingNFO)
	putJSONBool(m, "SaveRecordingImages", data.SaveRecordingImages)

	return diags
}

func overlayTunerHosts(ctx context.Context, m map[string]json.RawMessage, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}
	var hosts []TunerHostModel
	if d := v.ElementsAs(ctx, &hosts, false); d.HasError() {
		return append(diags, d...)
	}
	entries := make([]map[string]json.RawMessage, len(hosts))
	for i, h := range hosts {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "Id", h.ID)
		putJSONString(entry, "Url", h.URL)
		putJSONString(entry, "Type", h.Type)
		putJSONString(entry, "DeviceId", h.DeviceID)
		putJSONString(entry, "FriendlyName", h.FriendlyName)
		putJSONBool(entry, "ImportFavoritesOnly", h.ImportFavoritesOnly)
		putJSONBool(entry, "AllowHWTranscoding", h.AllowHWTranscoding)
		putJSONBool(entry, "AllowFmp4TranscodingContainer", h.AllowFmp4TranscodingContainer)
		putJSONBool(entry, "AllowStreamSharing", h.AllowStreamSharing)
		putJSONInt64(entry, "FallbackMaxStreamingBitrate", h.FallbackMaxStreamingBitrate)
		putJSONBool(entry, "EnableStreamLooping", h.EnableStreamLooping)
		putJSONString(entry, "Source", h.Source)
		putJSONInt64(entry, "TunerCount", h.TunerCount)
		putJSONString(entry, "UserAgent", h.UserAgent)
		putJSONBool(entry, "IgnoreDts", h.IgnoreDts)
		putJSONBool(entry, "ReadAtNativeFramerate", h.ReadAtNativeFramerate)
		entries[i] = entry
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal tuner hosts", err.Error()))
	}
	m["TunerHosts"] = b
	return diags
}

func overlayListingProviders(ctx context.Context, m map[string]json.RawMessage, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}
	var providers []ListingProviderModel
	if d := v.ElementsAs(ctx, &providers, false); d.HasError() {
		return append(diags, d...)
	}
	entries := make([]map[string]json.RawMessage, len(providers))
	for i, p := range providers {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "Id", p.ID)
		putJSONString(entry, "Type", p.Type)
		putJSONString(entry, "Username", p.Username)
		putJSONString(entry, "Password", p.Password)
		putJSONString(entry, "ListingsId", p.ListingsID)
		putJSONString(entry, "ZipCode", p.ZipCode)
		putJSONString(entry, "Country", p.Country)
		putJSONString(entry, "Path", p.Path)
		putJSONStringList(ctx, entry, "EnabledTuners", p.EnabledTuners)
		putJSONBool(entry, "EnableAllTuners", p.EnableAllTuners)
		putJSONStringList(ctx, entry, "NewsCategories", p.NewsCategories)
		putJSONStringList(ctx, entry, "SportsCategories", p.SportsCategories)
		putJSONStringList(ctx, entry, "KidsCategories", p.KidsCategories)
		putJSONStringList(ctx, entry, "MovieCategories", p.MovieCategories)
		if d := overlayChannelMappings(ctx, entry, p.ChannelMappings); d.HasError() {
			return append(diags, d...)
		}
		putJSONString(entry, "MoviePrefix", p.MoviePrefix)
		putJSONString(entry, "PreferredLanguage", p.PreferredLanguage)
		putJSONString(entry, "UserAgent", p.UserAgent)
		entries[i] = entry
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal listing providers", err.Error()))
	}
	m["ListingProviders"] = b
	return diags
}

func overlayChannelMappings(ctx context.Context, m map[string]json.RawMessage, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}
	var mappings []ChannelMappingModel
	if d := v.ElementsAs(ctx, &mappings, false); d.HasError() {
		return append(diags, d...)
	}
	entries := make([]map[string]json.RawMessage, len(mappings))
	for i, mapping := range mappings {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "Name", mapping.Name)
		putJSONString(entry, "Value", mapping.Value)
		entries[i] = entry
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal channel mappings", err.Error()))
	}
	m["ChannelMappings"] = b
	return diags
}

func flattenLiveTVConfiguration(ctx context.Context, raw string, data *LiveTVConfigurationResourceModel, diags *diag.Diagnostics) {
	m, err := parseJSONObject(raw)
	if err != nil {
		diags.AddError("Failed to parse Live TV configuration", err.Error())
		return
	}

	data.GuideDays = getJSONInt64(m, "GuideDays")
	data.RecordingPath = getJSONString(m, "RecordingPath")
	data.MovieRecordingPath = getJSONString(m, "MovieRecordingPath")
	data.SeriesRecordingPath = getJSONString(m, "SeriesRecordingPath")
	data.EnableRecordingSubfolders = getJSONBool(m, "EnableRecordingSubfolders")
	data.EnableOriginalAudioWithEncodedRecordings = getJSONBool(m, "EnableOriginalAudioWithEncodedRecordings")
	data.TunerHosts = flattenTunerHosts(ctx, m, diags)
	data.ListingProviders = flattenListingProviders(ctx, m, diags)
	data.PrePaddingSeconds = getJSONInt64(m, "PrePaddingSeconds")
	data.PostPaddingSeconds = getJSONInt64(m, "PostPaddingSeconds")
	data.MediaLocationsCreated, _ = getJSONStringList(ctx, m, "MediaLocationsCreated")
	data.RecordingPostProcessor = getJSONString(m, "RecordingPostProcessor")
	data.RecordingPostProcessorArguments = getJSONString(m, "RecordingPostProcessorArguments")
	data.SaveRecordingNFO = getJSONBool(m, "SaveRecordingNFO")
	data.SaveRecordingImages = getJSONBool(m, "SaveRecordingImages")
}

func flattenTunerHosts(_ context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["TunerHosts"]
	if !ok {
		return types.ListNull(tunerHostObjectType())
	}
	if isJSONNull(raw) {
		return types.ListNull(tunerHostObjectType())
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse tuner hosts", err.Error())
		return types.ListNull(tunerHostObjectType())
	}
	objType := tunerHostObjectType()
	objects := make([]attr.Value, len(entries))
	for i, entry := range entries {
		attrs := map[string]attr.Value{
			"id":                               getJSONString(entry, "Id"),
			"url":                              getJSONString(entry, "Url"),
			"type":                             getJSONString(entry, "Type"),
			"device_id":                        getJSONString(entry, "DeviceId"),
			"friendly_name":                    getJSONString(entry, "FriendlyName"),
			"import_favorites_only":            getJSONBool(entry, "ImportFavoritesOnly"),
			"allow_hw_transcoding":             getJSONBool(entry, "AllowHWTranscoding"),
			"allow_fmp4_transcoding_container": getJSONBool(entry, "AllowFmp4TranscodingContainer"),
			"allow_stream_sharing":             getJSONBool(entry, "AllowStreamSharing"),
			"fallback_max_streaming_bitrate":   getJSONInt64(entry, "FallbackMaxStreamingBitrate"),
			"enable_stream_looping":            getJSONBool(entry, "EnableStreamLooping"),
			"source":                           getJSONString(entry, "Source"),
			"tuner_count":                      getJSONInt64(entry, "TunerCount"),
			"user_agent":                       getJSONString(entry, "UserAgent"),
			"ignore_dts":                       getJSONBool(entry, "IgnoreDts"),
			"read_at_native_framerate":         getJSONBool(entry, "ReadAtNativeFramerate"),
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

func tunerHostObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":                               types.StringType,
		"url":                              types.StringType,
		"type":                             types.StringType,
		"device_id":                        types.StringType,
		"friendly_name":                    types.StringType,
		"import_favorites_only":            types.BoolType,
		"allow_hw_transcoding":             types.BoolType,
		"allow_fmp4_transcoding_container": types.BoolType,
		"allow_stream_sharing":             types.BoolType,
		"fallback_max_streaming_bitrate":   types.Int64Type,
		"enable_stream_looping":            types.BoolType,
		"source":                           types.StringType,
		"tuner_count":                      types.Int64Type,
		"user_agent":                       types.StringType,
		"ignore_dts":                       types.BoolType,
		"read_at_native_framerate":         types.BoolType,
	}}
}

func flattenListingProviders(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["ListingProviders"]
	if !ok {
		return types.ListNull(listingProviderObjectType())
	}
	if isJSONNull(raw) {
		return types.ListNull(listingProviderObjectType())
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse listing providers", err.Error())
		return types.ListNull(listingProviderObjectType())
	}
	objType := listingProviderObjectType()
	objects := make([]attr.Value, len(entries))
	for i, entry := range entries {
		attrs := map[string]attr.Value{
			"id":                 getJSONString(entry, "Id"),
			"type":               getJSONString(entry, "Type"),
			"username":           getJSONString(entry, "Username"),
			"password":           getJSONString(entry, "Password"),
			"listings_id":        getJSONString(entry, "ListingsId"),
			"zip_code":           getJSONString(entry, "ZipCode"),
			"country":            getJSONString(entry, "Country"),
			"path":               getJSONString(entry, "Path"),
			"enabled_tuners":     nullStringList(),
			"enable_all_tuners":  getJSONBool(entry, "EnableAllTuners"),
			"news_categories":    nullStringList(),
			"sports_categories":  nullStringList(),
			"kids_categories":    nullStringList(),
			"movie_categories":   nullStringList(),
			"channel_mappings":   flattenChannelMappings(ctx, entry, diags),
			"movie_prefix":       getJSONString(entry, "MoviePrefix"),
			"preferred_language": getJSONString(entry, "PreferredLanguage"),
			"user_agent":         getJSONString(entry, "UserAgent"),
		}
		if v, d := getJSONStringList(ctx, entry, "EnabledTuners"); !d.HasError() {
			attrs["enabled_tuners"] = v
		}
		if v, d := getJSONStringList(ctx, entry, "NewsCategories"); !d.HasError() {
			attrs["news_categories"] = v
		}
		if v, d := getJSONStringList(ctx, entry, "SportsCategories"); !d.HasError() {
			attrs["sports_categories"] = v
		}
		if v, d := getJSONStringList(ctx, entry, "KidsCategories"); !d.HasError() {
			attrs["kids_categories"] = v
		}
		if v, d := getJSONStringList(ctx, entry, "MovieCategories"); !d.HasError() {
			attrs["movie_categories"] = v
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

func listingProviderObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":                 types.StringType,
		"type":               types.StringType,
		"username":           types.StringType,
		"password":           types.StringType,
		"listings_id":        types.StringType,
		"zip_code":           types.StringType,
		"country":            types.StringType,
		"path":               types.StringType,
		"enabled_tuners":     types.ListType{ElemType: types.StringType},
		"enable_all_tuners":  types.BoolType,
		"news_categories":    types.ListType{ElemType: types.StringType},
		"sports_categories":  types.ListType{ElemType: types.StringType},
		"kids_categories":    types.ListType{ElemType: types.StringType},
		"movie_categories":   types.ListType{ElemType: types.StringType},
		"channel_mappings":   types.ListType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"name": types.StringType, "value": types.StringType}}},
		"movie_prefix":       types.StringType,
		"preferred_language": types.StringType,
		"user_agent":         types.StringType,
	}}
}

func flattenChannelMappings(_ context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["ChannelMappings"]
	if !ok {
		return types.ListNull(types.ObjectType{AttrTypes: map[string]attr.Type{"name": types.StringType, "value": types.StringType}})
	}
	if isJSONNull(raw) {
		return types.ListNull(types.ObjectType{AttrTypes: map[string]attr.Type{"name": types.StringType, "value": types.StringType}})
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse channel mappings", err.Error())
		return types.ListNull(types.ObjectType{AttrTypes: map[string]attr.Type{"name": types.StringType, "value": types.StringType}})
	}
	objType := types.ObjectType{AttrTypes: map[string]attr.Type{"name": types.StringType, "value": types.StringType}}
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

func nullStringList() types.List {
	return types.ListNull(types.StringType)
}
