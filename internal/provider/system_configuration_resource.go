// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

var (
	_ resource.Resource                = &SystemConfigurationResource{}
	_ resource.ResourceWithImportState = &SystemConfigurationResource{}
	_ resource.ResourceWithModifyPlan  = &SystemConfigurationResource{}
	_ wireBound                        = &SystemConfigurationResource{}
	_ offersProviders                  = &SystemConfigurationResource{}
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
	EnableNormalizedItemByNameIDs       types.Bool   `tfsdk:"enable_normalized_item_by_name_ids"`
	IsPortAuthorized                    types.Bool   `tfsdk:"is_port_authorized"`
	QuickConnectAvailable               types.Bool   `tfsdk:"quick_connect_available"`
	EnableCaseSensitiveItemIDs          types.Bool   `tfsdk:"enable_case_sensitive_item_ids"`
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

var systemWire = sync.OnceValues(func() (*wire.Binding, error) {
	return wire.Bind(schemaOf(&SystemConfigurationResource{}), "ServerConfiguration",
		wire.Identity("id"),
		// Merged into the served options, so that the settings a create
		// leaves unset, and so plans unknown, keep their values.
		wire.Document("TrickplayOptions"),
		wire.Complement("metadata_options.metadata_fetchers", "metadata_fetcher_order", "disabled_metadata_fetchers", "MetadataFetchers", "item_type"),
		wire.Complement("metadata_options.image_fetchers", "image_fetcher_order", "disabled_image_fetchers", "ImageFetchers", "item_type"))
})

func (r *SystemConfigurationResource) Wire() (*wire.Binding, error) { return systemWire() }

func (r *SystemConfigurationResource) offered(c *client.Client) wire.AvailableFunc {
	return newOfferedProviders(c).byItemType
}

func (r *SystemConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_configuration"
}

func (r *SystemConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	// UseStateForUnknown also copies a null prior value, and a planned null
	// fails the apply when the server returns a value.
	nonNullStateString := func(a schema.StringAttribute) schema.StringAttribute {
		a.PlanModifiers = []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()}
		return a
	}
	nonNullStateBool := func(a schema.BoolAttribute) schema.BoolAttribute {
		a.PlanModifiers = []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()}
		return a
	}
	nonNullStateList := func(a schema.ListAttribute) schema.ListAttribute {
		a.PlanModifiers = []planmodifier.List{listplanmodifier.UseNonNullStateForUnknown()}
		return a
	}

	metadataOptionsAttributes := map[string]schema.Attribute{
		"item_type":                   nonNullStateString(optionalString("Item type.")),
		"disabled_metadata_savers":    nonNullStateList(optionalStringList("Disabled metadata savers.")),
		"local_metadata_reader_order": nonNullStateList(optionalStringList("Local metadata reader order.")),
		"metadata_fetchers":           combinedStringList(itemTypeFetchersDescription("metadata", "disabled_metadata_fetchers", "metadata_fetcher_order"), "disabled_metadata_fetchers", "metadata_fetcher_order"),
		"disabled_metadata_fetchers":  replacedBy(nonNullStateList(optionalStringList("Disabled metadata fetchers.")), itemTypeFetchersDeprecation("metadata"), "metadata_fetchers"),
		"metadata_fetcher_order":      replacedBy(nonNullStateList(optionalStringList("Metadata fetcher order.")), itemTypeFetchersDeprecation("metadata"), "metadata_fetchers"),
		"image_fetchers":              combinedStringList(itemTypeFetchersDescription("image", "disabled_image_fetchers", "image_fetcher_order"), "disabled_image_fetchers", "image_fetcher_order"),
		"disabled_image_fetchers":     replacedBy(nonNullStateList(optionalStringList("Disabled image fetchers.")), itemTypeFetchersDeprecation("image"), "image_fetchers"),
		"image_fetcher_order":         replacedBy(nonNullStateList(optionalStringList("Image fetcher order.")), itemTypeFetchersDeprecation("image"), "image_fetchers"),
	}

	nameValuePairAttributes := map[string]schema.Attribute{
		"name":  nonNullStateString(optionalString("Name.")),
		"value": nonNullStateString(optionalString("Value.")),
	}

	pathSubstitutionAttributes := map[string]schema.Attribute{
		"from": nonNullStateString(optionalString("From path.")),
		"to":   nonNullStateString(optionalString("To path.")),
	}

	castReceiverApplicationAttributes := map[string]schema.Attribute{
		"id":   nonNullStateString(optionalString("Application ID. Must be set in every entry.")),
		"name": nonNullStateString(optionalString("Application name. Must be set in every entry.")),
	}

	trickplayOptionsAttributes := map[string]schema.Attribute{
		"enable_hw_acceleration":           optionalBool("Enable hardware acceleration."),
		"enable_hw_encoding":               optionalBool("Enable hardware encoding."),
		"enable_key_frame_only_extraction": optionalBool("Enable key frame only extraction."),
		"scan_behavior":                    optionalEnum("Scan behavior.", "Blocking", "NonBlocking"),
		"process_priority":                 nonNullStateString(optionalEnum("Process priority class.", "Normal", "Idle", "High", "RealTime", "BelowNormal", "AboveNormal")),
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
			"enable_normalized_item_by_name_ids":     nonNullStateBool(optionalBool("Enable normalized item by name IDs.")),
			"is_port_authorized":                     optionalBool("Is port authorized."),
			"quick_connect_available":                optionalBool("Quick connect available."),
			"enable_case_sensitive_item_ids":         nonNullStateBool(optionalBool("Enable case sensitive item IDs.")),
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
			"image_saving_convention":                optionalEnum("Image saving convention.", "Legacy", "Compatible"),
			"metadata_options": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: metadataOptionsAttributes,
				},
				Description:         "Metadata options, one entry per item type. The list replaces the server's list, so an item type it leaves out loses its entry, and Jellyfin then uses its defaults for that type, which enable every fetcher.",
				MarkdownDescription: "Metadata options, one entry per item type. The list replaces the server's list, so an item type it leaves out loses its entry, and Jellyfin then uses its defaults for that type, which enable every fetcher.",
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
			"chapter_image_resolution":             optionalEnum("Chapter image resolution.", "MatchSource", "P144", "P240", "P360", "P480", "P720", "P1080", "P1440", "P2160"),
			"parallel_image_encoding_limit":        optionalInt("Parallel image encoding limit."),
			"cast_receiver_applications": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: castReceiverApplicationAttributes,
					// Not Required on id and name: Terraform would propose null for them when the
					// list is left out, so plans would never be empty.
					Validators: []validator.Object{
						objectvalidator.AlsoRequires(path.MatchRelative().AtName("id"), path.MatchRelative().AtName("name")),
					},
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

func itemTypeFetchersDescription(kind, disabledAttr, orderAttr string) string {
	return fmt.Sprintf("Enabled %[1]s fetchers for `item_type`, in priority order: Jellyfin asks the first one first and disables every other %[1]s fetcher it offers for the item type. Jellyfin applies them to items whose library has no `type_options` entry for their type. Each name must match one the server offers exactly, so it works only for an item type whose fetchers Jellyfin lists, such as Movie or Series; for any other, such as Person, it reads as null and setting it is an error, and `%[2]s` and `%[3]s` still apply. Jellyfin enables any %[1]s fetcher installed later, which then shows up as a change to this list. Conflicts with `%[2]s` and `%[3]s`, which it replaces.", kind, disabledAttr, orderAttr)
}

func itemTypeFetchersDeprecation(kind string) string {
	return fmt.Sprintf("Deprecated: list the enabled %[1]s fetchers in priority order in `%[1]s_fetchers` instead, which disables the rest. It will be removed in a future release.", kind)
}

func (r *SystemConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *SystemConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.write(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *SystemConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.singleton().read(wire.WithAvailable(ctx, r.offered(r.client)), req.State, &resp.State, &resp.Diagnostics)
}

func (r *SystemConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.write(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *SystemConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// System configuration cannot be deleted. We just remove from state.
}

func (r *SystemConfigurationResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	r.singleton().setID(ctx, &resp.State, &resp.Diagnostics)
}

// ModifyPlan gates each configured field on the Jellyfin version it needs, so
// a field a later pin adds is checked without a change here. It also plans
// each metadata_options attribute left unset from the prior entry with the
// same item type. UseNonNullStateForUnknown takes it from the prior entry at
// the same index instead, so inserting or reordering entries would plan, and
// then write, another item type's lists.
func (r *SystemConfigurationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	checkServerHasFields(ctx, r.client, systemWire, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() || req.State.Raw.IsNull() {
		return
	}

	metadataOptionsPath := path.Root("metadata_options")
	var config, plan, state types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, metadataOptionsPath, &config)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, metadataOptionsPath, &plan)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, metadataOptionsPath, &state)...)
	if resp.Diagnostics.HasError() || config.IsNull() || config.IsUnknown() || plan.IsNull() || plan.IsUnknown() {
		return
	}

	planned, diags := planMetadataOptionsByItemType(ctx, config, plan, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, metadataOptionsPath, planned)...)
}

type metadataOptionsModel struct {
	ItemType                 types.String `tfsdk:"item_type"`
	DisabledMetadataSavers   types.List   `tfsdk:"disabled_metadata_savers"`
	LocalMetadataReaderOrder types.List   `tfsdk:"local_metadata_reader_order"`
	MetadataFetchers         types.List   `tfsdk:"metadata_fetchers"`
	DisabledMetadataFetchers types.List   `tfsdk:"disabled_metadata_fetchers"`
	MetadataFetcherOrder     types.List   `tfsdk:"metadata_fetcher_order"`
	ImageFetchers            types.List   `tfsdk:"image_fetchers"`
	DisabledImageFetchers    types.List   `tfsdk:"disabled_image_fetchers"`
	ImageFetcherOrder        types.List   `tfsdk:"image_fetcher_order"`
}

// planMetadataOptionsByItemType plans each entry's attributes as its schema's
// plan modifiers do, but against the prior entry with its item type.
func planMetadataOptionsByItemType(ctx context.Context, config, plan, state types.List) (types.List, diag.Diagnostics) {
	return replanEntries(ctx, config, plan, state, func(i int, e metadataOptionsModel, p *metadataOptionsModel, s []metadataOptionsModel) diag.Diagnostics {
		// Without an item type, an entry takes the one of the prior entry at
		// its index, so its plan from that entry stands.
		if e.ItemType.IsNull() {
			return nil
		}
		prior, found := priorMetadataOptions(s, i, e.ItemType)
		fromPrior := func(configured, planned, prior types.List, sharedKeysChange bool) types.List {
			switch {
			case !configured.IsNull():
				return planned
			case found && !sharedKeysChange && !prior.IsNull():
				return prior
			case found && !sharedKeysChange && planned.IsNull():
				// Only a plan that changes nothing holds a null; otherwise the
				// framework plans every unset attribute unknown.
				return planned
			}
			return types.ListUnknown(types.StringType)
		}
		p.DisabledMetadataSavers = fromPrior(e.DisabledMetadataSavers, p.DisabledMetadataSavers, prior.DisabledMetadataSavers, false)
		p.LocalMetadataReaderOrder = fromPrior(e.LocalMetadataReaderOrder, p.LocalMetadataReaderOrder, prior.LocalMetadataReaderOrder, false)
		p.MetadataFetchers = fromPrior(e.MetadataFetchers, p.MetadataFetchers, prior.MetadataFetchers,
			changes(e.DisabledMetadataFetchers, prior.DisabledMetadataFetchers) || changes(e.MetadataFetcherOrder, prior.MetadataFetcherOrder))
		p.DisabledMetadataFetchers = fromPrior(e.DisabledMetadataFetchers, p.DisabledMetadataFetchers, prior.DisabledMetadataFetchers, changes(e.MetadataFetchers, prior.MetadataFetchers))
		p.MetadataFetcherOrder = fromPrior(e.MetadataFetcherOrder, p.MetadataFetcherOrder, prior.MetadataFetcherOrder, changes(e.MetadataFetchers, prior.MetadataFetchers))
		p.ImageFetchers = fromPrior(e.ImageFetchers, p.ImageFetchers, prior.ImageFetchers,
			changes(e.DisabledImageFetchers, prior.DisabledImageFetchers) || changes(e.ImageFetcherOrder, prior.ImageFetcherOrder))
		p.DisabledImageFetchers = fromPrior(e.DisabledImageFetchers, p.DisabledImageFetchers, prior.DisabledImageFetchers, changes(e.ImageFetchers, prior.ImageFetchers))
		p.ImageFetcherOrder = fromPrior(e.ImageFetcherOrder, p.ImageFetcherOrder, prior.ImageFetcherOrder, changes(e.ImageFetchers, prior.ImageFetchers))
		return nil
	})
}

// priorMetadataOptions prefers the prior entry at index when it has the item type,
// so an item type held twice plans each entry from its own prior values.
func priorMetadataOptions(state []metadataOptionsModel, index int, itemType types.String) (metadataOptionsModel, bool) {
	if index < len(state) && !state[index].ItemType.IsNull() && !itemType.IsUnknown() && strings.EqualFold(state[index].ItemType.ValueString(), itemType.ValueString()) {
		return state[index], true
	}
	return entryWithType(state, itemType, func(e metadataOptionsModel) types.String { return e.ItemType })
}

func (r *SystemConfigurationResource) write(ctx context.Context, plan tfsdk.Plan, state *tfsdk.State, diags *diag.Diagnostics) {
	// The document holds the plugin repositories, which the write posts back
	// as read.
	serverConfigurationMu.Lock()
	defer serverConfigurationMu.Unlock()

	r.singleton().write(wire.WithAvailable(ctx, r.offered(r.client)), plan, state, diags)
}

func (r *SystemConfigurationResource) singleton() singleton[SystemConfigurationResourceModel] {
	return singleton[SystemConfigurationResourceModel]{
		id:   "system",
		bind: systemWire,
		doc:  document{what: "system configuration", get: r.client.GetSystemConfiguration, put: r.client.UpdateSystemConfiguration},
	}
}
