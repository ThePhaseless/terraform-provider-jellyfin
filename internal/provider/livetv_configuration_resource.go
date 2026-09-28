// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
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
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

var (
	_ resource.Resource                = &LiveTVConfigurationResource{}
	_ resource.ResourceWithImportState = &LiveTVConfigurationResource{}
	_ resource.ResourceWithModifyPlan  = &LiveTVConfigurationResource{}
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

// The nested attributes carry no UseStateForUnknown: it pairs list elements by
// index, so tuner_hosts and listing_providers fill their unknowns by key instead.
func tunerHostAttributes() map[string]schema.Attribute {
	optionalString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Description: desc, MarkdownDescription: desc, Optional: true, Computed: true}
	}
	optionalBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Description: desc, MarkdownDescription: desc, Optional: true, Computed: true}
	}
	optionalInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{Description: desc, MarkdownDescription: desc, Optional: true, Computed: true}
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
		return schema.StringAttribute{Description: desc, MarkdownDescription: desc, Optional: true, Computed: true}
	}
	optionalBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Description: desc, MarkdownDescription: desc, Optional: true, Computed: true}
	}
	optionalStringList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{ElementType: types.StringType, Description: desc, MarkdownDescription: desc, Optional: true, Computed: true}
	}
	return map[string]schema.Attribute{
		"id":       optionalString("Provider ID."),
		"type":     optionalString("Provider type."),
		"username": optionalString("Username."),
		"password": schema.StringAttribute{
			Description: "Password.", MarkdownDescription: "Password.",
			Optional: true, Computed: true, Sensitive: true,
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
		},
		"movie_prefix":       optionalString("Movie prefix."),
		"preferred_language": optionalString("Preferred language."),
		"user_agent":         optionalString("User agent."),
	}
}

func (r *LiveTVConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
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
	// Set only the id: the framework types every other attribute from the
	// schema, and the Read that follows an import fills them. A zero-valued
	// model would leave list attributes without an element type.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue("livetv"))...)
}

// ModifyPlan gates each configured field on the Jellyfin version it needs, so
// a field a later pin adds is checked without a change here.
func (r *LiveTVConfigurationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	if b := wireBinding(&resp.Diagnostics, livetvWire); b != nil {
		resp.Diagnostics.Append(checkServerHasFields(ctx, r.client, b, req.Config)...)
	}
}

func (r *LiveTVConfigurationResource) apply(ctx context.Context, data *LiveTVConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	b := wireBinding(diags, livetvWire)
	if b == nil || !r.document().write(ctx, b, data, diags) {
		return
	}
	data.ID = types.StringValue("livetv")
	diags.Append(state.Set(ctx, data)...)
}

func (r *LiveTVConfigurationResource) read(ctx context.Context, data *LiveTVConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	b := wireBinding(diags, livetvWire)
	if b == nil || !r.document().read(ctx, b, data, diags) {
		return
	}
	data.ID = types.StringValue("livetv")
	diags.Append(state.Set(ctx, data)...)
}

func (r *LiveTVConfigurationResource) document() document {
	return document{what: "Live TV configuration", get: r.client.GetLiveTVConfiguration, put: r.client.UpdateLiveTVConfiguration}
}
