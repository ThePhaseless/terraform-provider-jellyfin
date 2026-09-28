// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
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
	optionalEnum := func(desc string, values ...string) schema.StringAttribute {
		a := optionalString(desc + " One of `" + strings.Join(values, "`, `") + "`.")
		a.Validators = []validator.String{stringvalidator.OneOf(values...)}
		return a
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
	// UseStateForUnknown also copies a null prior value, and a planned null
	// fails the apply when the server returns a value. That happens for an
	// entry a list gains in the plan, whose prior values are all null while
	// the server fills in what the entry leaves out, and for the three
	// attributes whose keys 0.3.7 and earlier misspelt, which state from
	// those versions holds as null.
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
					// Required on id and name would make Terraform propose null for
					// them whenever the list is left out of the configuration, so
					// every plan would differ from state; the element validator
					// enforces them only for entries that are configured.
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
	// Singleton resource, so the import ID is not used. Set only the id: the
	// framework types every other attribute from the schema, and the Read that
	// follows an import fills them. A zero-valued model would leave list
	// attributes without an element type.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue("system"))...)
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
	if b := wireBinding(&resp.Diagnostics, systemWire); b != nil {
		resp.Diagnostics.Append(checkServerHasFields(ctx, r.client, b, req.Config)...)
	}
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

// planMetadataOptionsByItemType plans the attributes of each entry as the
// plan modifiers of its schema do, but against the prior entry with its item
// type.
func planMetadataOptionsByItemType(ctx context.Context, config, plan, state types.List) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	var c, p, s []metadataOptionsModel
	if config.ElementsAs(ctx, &c, false).HasError() || plan.ElementsAs(ctx, &p, false).HasError() || len(c) != len(p) {
		return plan, diags
	}
	if !state.IsNull() && !state.IsUnknown() && state.ElementsAs(ctx, &s, false).HasError() {
		return plan, diags
	}

	for i := range p {
		// Without an item type, an entry takes the one of the prior entry at
		// its index, so its plan from that entry stands.
		if c[i].ItemType.IsNull() {
			continue
		}
		prior, found := priorMetadataOptions(s, i, c[i].ItemType)
		fromPrior := func(configured, planned, prior types.List, sharedKeysChange bool) types.List {
			switch {
			case !configured.IsNull():
				return planned
			case found && !sharedKeysChange && !prior.IsNull():
				return prior
			case found && !sharedKeysChange && planned.IsNull():
				// Only a plan that changes nothing holds a null: once
				// anything changes, the framework plans every unset
				// attribute unknown.
				return planned
			}
			return types.ListUnknown(types.StringType)
		}
		e := c[i]
		p[i].DisabledMetadataSavers = fromPrior(e.DisabledMetadataSavers, p[i].DisabledMetadataSavers, prior.DisabledMetadataSavers, false)
		p[i].LocalMetadataReaderOrder = fromPrior(e.LocalMetadataReaderOrder, p[i].LocalMetadataReaderOrder, prior.LocalMetadataReaderOrder, false)
		p[i].MetadataFetchers = fromPrior(e.MetadataFetchers, p[i].MetadataFetchers, prior.MetadataFetchers,
			changes(e.DisabledMetadataFetchers, prior.DisabledMetadataFetchers) || changes(e.MetadataFetcherOrder, prior.MetadataFetcherOrder))
		p[i].DisabledMetadataFetchers = fromPrior(e.DisabledMetadataFetchers, p[i].DisabledMetadataFetchers, prior.DisabledMetadataFetchers, changes(e.MetadataFetchers, prior.MetadataFetchers))
		p[i].MetadataFetcherOrder = fromPrior(e.MetadataFetcherOrder, p[i].MetadataFetcherOrder, prior.MetadataFetcherOrder, changes(e.MetadataFetchers, prior.MetadataFetchers))
		p[i].ImageFetchers = fromPrior(e.ImageFetchers, p[i].ImageFetchers, prior.ImageFetchers,
			changes(e.DisabledImageFetchers, prior.DisabledImageFetchers) || changes(e.ImageFetcherOrder, prior.ImageFetcherOrder))
		p[i].DisabledImageFetchers = fromPrior(e.DisabledImageFetchers, p[i].DisabledImageFetchers, prior.DisabledImageFetchers, changes(e.ImageFetchers, prior.ImageFetchers))
		p[i].ImageFetcherOrder = fromPrior(e.ImageFetcherOrder, p[i].ImageFetcherOrder, prior.ImageFetcherOrder, changes(e.ImageFetchers, prior.ImageFetchers))
	}

	out, d := types.ListValueFrom(ctx, plan.ElementType(ctx), p)
	diags.Append(d...)
	return out, diags
}

// priorMetadataOptions prefers the prior entry at index when it has the item
// type, so that a configuration holding an item type twice plans each entry
// from its own prior values.
func priorMetadataOptions(state []metadataOptionsModel, index int, itemType types.String) (metadataOptionsModel, bool) {
	if index < len(state) && !state[index].ItemType.IsNull() && !itemType.IsUnknown() && strings.EqualFold(state[index].ItemType.ValueString(), itemType.ValueString()) {
		return state[index], true
	}
	return entryWithType(state, itemType, func(e metadataOptionsModel) types.String { return e.ItemType })
}

func (r *SystemConfigurationResource) apply(ctx context.Context, data *SystemConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	b := wireBinding(diags, systemWire)
	if b == nil {
		return
	}
	ctx = wire.WithAvailable(ctx, r.offered(r.client))

	// The document holds the plugin repositories, which the write posts back
	// as read.
	serverConfigurationMu.Lock()
	defer serverConfigurationMu.Unlock()

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

	if d := b.OverlayModel(ctx, base, data); d.HasError() {
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

	diags.Append(b.FlattenAfterApply(ctx, updated.RawJSON, data)...)
	data.ID = types.StringValue("system")
	diags.Append(state.Set(ctx, data)...)
}

func (r *SystemConfigurationResource) read(ctx context.Context, data *SystemConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	b := wireBinding(diags, systemWire)
	if b == nil {
		return
	}
	ctx = wire.WithAvailable(ctx, r.offered(r.client))

	current, err := r.client.GetSystemConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read system configuration", err.Error())
		return
	}

	diags.Append(b.FlattenInto(ctx, current.RawJSON, data)...)
	data.ID = types.StringValue("system")
	diags.Append(state.Set(ctx, data)...)
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
