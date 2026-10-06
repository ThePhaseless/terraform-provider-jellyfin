// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

var (
	_ resource.Resource                = &LiveTVConfigurationResource{}
	_ resource.ResourceWithImportState = &LiveTVConfigurationResource{}
	_ wireBound                        = &LiveTVConfigurationResource{}
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

var livetvWire = sync.OnceValues(func() (*wire.Binding, error) {
	return wire.Bind(schemaOf(&LiveTVConfigurationResource{}), "LiveTvOptions",
		wire.Identity("id"),
		wire.Key("save_recording_nfo", "SaveRecordingNFO"),
		wire.Key("tuner_hosts.allow_hw_transcoding", "AllowHWTranscoding"),
		// Planning fills an entry's unset settings from the prior entry it
		// matches, which a first apply has none of; the write then keeps
		// those of the served entry with the same id.
		wire.MergeByKey("tuner_hosts", "id"),
		wire.MergeByKey("listing_providers", "id"))
})

func (r *LiveTVConfigurationResource) Wire() (*wire.Binding, error) { return livetvWire() }

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
			"guide_days":                                    optionalInt("Number of guide days."),
			"recording_path":                                optionalString("Recording path."),
			"movie_recording_path":                          optionalString("Movie recording path."),
			"series_recording_path":                         optionalString("Series recording path."),
			"enable_recording_subfolders":                   optionalBool("Whether recording subfolders are enabled."),
			"enable_original_audio_with_encoded_recordings": optionalBool("Whether original audio is kept with encoded recordings."),
			"tuner_hosts": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: tunerHostAttributes(),
				},
				Description:         "Tuner hosts. An entry keeps the settings it does not configure from the existing entry with the same `id`, else the same `url`, else the same `type`. Before the resource is in state, as on its first apply, only an entry with the `id` of an existing one keeps that one's settings; import the configuration first to keep them by `url` or `type`.",
				MarkdownDescription: "Tuner hosts. An entry keeps the settings it does not configure from the existing entry with the same `id`, else the same `url`, else the same `type`. Before the resource is in state, as on its first apply, only an entry with the `id` of an existing one keeps that one's settings; import the configuration first to keep them by `url` or `type`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
					useStateForUnknownByKey([]string{"id"}, []string{"url"}, []string{"type"}),
				},
			},
			"listing_providers": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: listingProviderAttributes(),
				},
				Description:         "Listing providers. An entry keeps the settings it does not configure from the existing entry with the same `id`, else the same `type` and `listings_id`, else the same `type` and `path`, else the same `type`. Before the resource is in state, as on its first apply, only an entry with the `id` of an existing one keeps that one's settings; import the configuration first to keep them by the other keys.",
				MarkdownDescription: "Listing providers. An entry keeps the settings it does not configure from the existing entry with the same `id`, else the same `type` and `listings_id`, else the same `type` and `path`, else the same `type`. Before the resource is in state, as on its first apply, only an entry with the `id` of an existing one keeps that one's settings; import the configuration first to keep them by the other keys.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
					useStateForUnknownByKey([]string{"id"}, []string{"type", "listings_id"}, []string{"type", "path"}, []string{"type"}),
				},
			},
			"pre_padding_seconds":  optionalInt("Pre-padding seconds."),
			"post_padding_seconds": optionalInt("Post-padding seconds."),
			// No UseStateForUnknown: Jellyfin rewrites this list in the background
			// whenever a save points a recording path at an existing directory, so
			// a value carried over from state would not match the one read back.
			"media_locations_created": schema.ListAttribute{
				ElementType:         types.StringType,
				Description:         "Recording folders Jellyfin has added as libraries. Jellyfin maintains this list itself when a recording path points at an existing directory, so it is usually left unset.",
				MarkdownDescription: "Recording folders Jellyfin has added as libraries. Jellyfin maintains this list itself when a recording path points at an existing directory, so it is usually left unset.",
				Optional:            true,
				Computed:            true,
			},
			"recording_post_processor":           optionalString("Recording post processor."),
			"recording_post_processor_arguments": optionalString("Recording post processor arguments."),
			"save_recording_nfo":                 optionalBool("Whether to save recording NFO."),
			"save_recording_images":              optionalBool("Whether to save recording images."),
		},
	}
}

// The nested attributes carry no UseStateForUnknown: it pairs list elements by
// index, so tuner_hosts and listing_providers fill their unknowns by key instead.
func tunerHostAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":                               elementString("Tuner host ID."),
		"url":                              elementString("Tuner host URL."),
		"type":                             elementString("Tuner host type."),
		"device_id":                        elementString("Device ID."),
		"friendly_name":                    elementString("Friendly name."),
		"import_favorites_only":            elementBool("Import favorites only."),
		"allow_hw_transcoding":             elementBool("Allow hardware transcoding."),
		"allow_fmp4_transcoding_container": elementBool("Allow fmp4 transcoding container."),
		"allow_stream_sharing":             elementBool("Allow stream sharing."),
		"fallback_max_streaming_bitrate":   elementInt("Fallback max streaming bitrate."),
		"enable_stream_looping":            elementBool("Enable stream looping."),
		"source":                           elementString("Source."),
		"tuner_count":                      elementInt("Tuner count."),
		"user_agent":                       elementString("User agent."),
		"ignore_dts":                       elementBool("Ignore DTS."),
		"read_at_native_framerate":         elementBool("Read at native framerate."),
	}
}

func listingProviderAttributes() map[string]schema.Attribute {
	password := elementString("Password.")
	password.Sensitive = true
	return map[string]schema.Attribute{
		"id":                elementString("Provider ID."),
		"type":              elementString("Provider type."),
		"username":          elementString("Username."),
		"password":          password,
		"listings_id":       elementString("Listings ID."),
		"zip_code":          elementString("ZIP code."),
		"country":           elementString("Country."),
		"path":              elementString("Path."),
		"enabled_tuners":    elementStringList("Enabled tuners."),
		"enable_all_tuners": elementBool("Enable all tuners."),
		"news_categories":   elementStringList("News categories."),
		"sports_categories": elementStringList("Sports categories."),
		"kids_categories":   elementStringList("Kids categories."),
		"movie_categories":  elementStringList("Movie categories."),
		"channel_mappings": schema.ListNestedAttribute{
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"name":  elementString("Channel name."),
					"value": elementString("Mapped value."),
				},
			},
			Description: "Channel mappings.", MarkdownDescription: "Channel mappings.",
			Optional: true, Computed: true,
		},
		"movie_prefix":       elementString("Movie prefix."),
		"preferred_language": elementString("Preferred language."),
		"user_agent":         elementString("User agent."),
	}
}

func (r *LiveTVConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *LiveTVConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.singleton().write(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *LiveTVConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.singleton().read(ctx, req.State, &resp.State, &resp.Diagnostics)
}

func (r *LiveTVConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.singleton().write(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *LiveTVConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Live TV configuration cannot be deleted. We just remove from state.
}

func (r *LiveTVConfigurationResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	r.singleton().setID(ctx, &resp.State, &resp.Diagnostics)
}

func (r *LiveTVConfigurationResource) singleton() singleton[LiveTVConfigurationResourceModel] {
	return singleton[LiveTVConfigurationResourceModel]{
		id:   "livetv",
		bind: livetvWire,
		doc:  document{what: "Live TV configuration", get: r.client.GetLiveTVConfiguration, put: r.client.UpdateLiveTVConfiguration},
	}
}
