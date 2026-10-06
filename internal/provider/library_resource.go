// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

var (
	_ resource.Resource                = &LibraryResource{}
	_ resource.ResourceWithImportState = &LibraryResource{}
	_ resource.ResourceWithModifyPlan  = &LibraryResource{}
	_ wireBound                        = &LibraryResource{}
)

// NewLibraryResource creates a new library resource.
func NewLibraryResource() resource.Resource {
	return &LibraryResource{}
}

// LibraryResource defines the resource implementation.
type LibraryResource struct {
	client *client.Client
}

// LibraryResourceModel describes the resource data model.
type LibraryResourceModel struct {
	ID             types.String         `tfsdk:"id"`
	Name           types.String         `tfsdk:"name"`
	CollectionType types.String         `tfsdk:"collection_type"`
	Paths          types.List           `tfsdk:"paths"`
	LibraryOptions *LibraryOptionsModel `tfsdk:"library_options"`
	ItemID         types.String         `tfsdk:"item_id"`
}

var libraryWire = sync.OnceValues(func() (*wire.Binding, error) {
	opts := []wire.Option{
		wire.Identity("id"),
		wire.Document("LibraryOptions"),
		wire.Key("paths", "Locations"),
		wire.Key("library_options.extract_chapters_during_library_scan", "ExtractChapterImagesDuringLibraryScan"),
		wire.Inverted("library_options.disabled", "Enabled"),
		wire.Legacy("library_options.path_infos.network_path", "NetworkPath", "10.10", networkPathRemovedMessage),
		wire.MergeByKey("library_options.type_options", "type"),
		wire.MergeByKey("library_options.type_options.image_options", "type"),
		wire.Orders("library_options.type_options.metadata_fetchers", "metadata_fetcher_order", "MetadataFetchers", "type"),
		wire.Orders("library_options.type_options.image_fetchers", "image_fetcher_order", "ImageFetchers", "type"),
		wire.Orders("library_options.type_options.similar_item_providers", "similar_item_provider_order", "SimilarItemProviders", "type"),
		wire.Complement("library_options.subtitle_fetchers", "subtitle_fetcher_order", "disabled_subtitle_fetchers", "SubtitleFetchers", ""),
		wire.VersionMessage("library_options.type_options.similar_item_providers", similarItemsVersionMessage),
		wire.VersionMessage("library_options.type_options.similar_item_provider_order", similarItemsVersionMessage),
		wire.VersionMessage("library_options.path_infos.network_path", networkPathVersionMessage),
	}
	for _, p := range []string{
		"library_options.enable_emby_photos", "library_options.enable_photo_subtitle",
		"library_options.chapter_image_interval_seconds",
		"library_options.extract_media_information_during_library_scan",
		"library_options.download_images_in_advance", "library_options.cache_images_in_library",
		"library_options.enable_media_conversion", "library_options.disabled_metadata_savers",
		"library_options.disabled_metadata_fetchers", "library_options.metadata_fetcher_order",
		"library_options.disabled_image_fetchers", "library_options.image_fetcher_order",
		"library_options.save_local_thumbnail_sets", "library_options.import_missing_episodes",
		"library_options.metadata_refresh_mode",
		"library_options.path_infos.username", "library_options.path_infos.password",
	} {
		opts = append(opts, wire.NeverSent(p, unsupportedLibraryOptionMessage))
	}
	return wire.Bind(schemaOf(&LibraryResource{}), "VirtualFolderInfo", opts...)
})

func (r *LibraryResource) Wire() (*wire.Binding, error) { return libraryWire() }

// libraryOptionsWire binds POST /Library/VirtualFolders/LibraryOptions and
// the listing's options; the rest comes from the typed folder.
var libraryOptionsWire = sync.OnceValues(func() (*wire.Binding, error) {
	b, err := libraryWire()
	if err != nil {
		return nil, err
	}
	return b.Document("LibraryOptions")
})

func similarItemsVersionMessage(g wire.VersionGap) (string, string) {
	return "Similar item settings not supported",
		fmt.Sprintf("The server runs Jellyfin %s, and similar item providers need Jellyfin 12 or later. Remove %s for this server.", g.ServerVersion, g.Path)
}

func networkPathVersionMessage(g wire.VersionGap) (string, string) {
	return "Network paths not supported",
		fmt.Sprintf("The server runs Jellyfin %s, and Jellyfin %s removed network paths, so the server would drop the value. Remove %s from the configuration.", g.ServerVersion, g.Until, g.Path)
}

// LibraryOptionsModel describes the typed library options.
type LibraryOptionsModel struct {
	EnablePhotos                             types.Bool   `tfsdk:"enable_photos"`
	EnableRealtimeMonitor                    types.Bool   `tfsdk:"enable_realtime_monitor"`
	EnableEmbiPhotos                         types.Bool   `tfsdk:"enable_emby_photos"`
	EnablePhotoSubtitle                      types.Bool   `tfsdk:"enable_photo_subtitle"`
	ExtractChaptersDuringLibraryScan         types.Bool   `tfsdk:"extract_chapters_during_library_scan"`
	EnableChapterImageExtraction             types.Bool   `tfsdk:"enable_chapter_image_extraction"`
	EnableTrickplayImageExtraction           types.Bool   `tfsdk:"enable_trickplay_image_extraction"`
	ExtractTrickplayImagesDuringLibraryScan  types.Bool   `tfsdk:"extract_trickplay_images_during_library_scan"`
	SaveTrickplayWithMedia                   types.Bool   `tfsdk:"save_trickplay_with_media"`
	ChapterImageIntervalSeconds              types.Int64  `tfsdk:"chapter_image_interval_seconds"`
	ExtractMediaInformationDuringLibraryScan types.Bool   `tfsdk:"extract_media_information_during_library_scan"`
	DownloadImagesInAdvance                  types.Bool   `tfsdk:"download_images_in_advance"`
	CacheImagesInLibrary                     types.Bool   `tfsdk:"cache_images_in_library"`
	EnableMediaConversion                    types.Bool   `tfsdk:"enable_media_conversion"`
	PathInfos                                types.List   `tfsdk:"path_infos"`
	PreferredMetadataLanguage                types.String `tfsdk:"preferred_metadata_language"`
	MetadataCountryCode                      types.String `tfsdk:"metadata_country_code"`
	MetadataSavers                           types.List   `tfsdk:"metadata_savers"`
	DisabledMetadataSavers                   types.List   `tfsdk:"disabled_metadata_savers"`
	LocalMetadataReaderOrder                 types.List   `tfsdk:"local_metadata_reader_order"`
	DisabledMetadataFetchers                 types.List   `tfsdk:"disabled_metadata_fetchers"`
	MetadataFetcherOrder                     types.List   `tfsdk:"metadata_fetcher_order"`
	DisabledImageFetchers                    types.List   `tfsdk:"disabled_image_fetchers"`
	ImageFetcherOrder                        types.List   `tfsdk:"image_fetcher_order"`
	SubtitleFetchers                         types.List   `tfsdk:"subtitle_fetchers"`
	DisabledSubtitleFetchers                 types.List   `tfsdk:"disabled_subtitle_fetchers"`
	SubtitleFetcherOrder                     types.List   `tfsdk:"subtitle_fetcher_order"`
	SaveLocalMetadata                        types.Bool   `tfsdk:"save_local_metadata"`
	SaveLocalThumbnailSets                   types.Bool   `tfsdk:"save_local_thumbnail_sets"`
	ImportMissingEpisodes                    types.Bool   `tfsdk:"import_missing_episodes"`
	EnableAutomaticSeriesGrouping            types.Bool   `tfsdk:"enable_automatic_series_grouping"`
	SeasonZeroDisplayName                    types.String `tfsdk:"season_zero_display_name"`
	MetadataRefreshMode                      types.String `tfsdk:"metadata_refresh_mode"`
	Disabled                                 types.Bool   `tfsdk:"disabled"`
	TypeOptions                              types.List   `tfsdk:"type_options"`
}

// TypeOptionsModel describes one TypeOptions entry.
type TypeOptionsModel struct {
	Type                     types.String `tfsdk:"type"`
	MetadataFetchers         types.List   `tfsdk:"metadata_fetchers"`
	MetadataFetcherOrder     types.List   `tfsdk:"metadata_fetcher_order"`
	ImageFetchers            types.List   `tfsdk:"image_fetchers"`
	ImageOptions             types.List   `tfsdk:"image_options"`
	ImageFetcherOrder        types.List   `tfsdk:"image_fetcher_order"`
	SimilarItemProviders     types.List   `tfsdk:"similar_item_providers"`
	SimilarItemProviderOrder types.List   `tfsdk:"similar_item_provider_order"`
}

// ImageOptionsModel describes one ImageOptions entry.
type ImageOptionsModel struct {
	Type     types.String `tfsdk:"type"`
	Limit    types.Int64  `tfsdk:"limit"`
	MinWidth types.Int64  `tfsdk:"min_width"`
}

func (r *LibraryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_library"
}

func (r *LibraryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages a Jellyfin media library (virtual folder).",
		MarkdownDescription: "Manages a Jellyfin media library (virtual folder).",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description:         "The library name.",
				MarkdownDescription: "The library name.",
				Required:            true,
				Validators:          libraryNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"collection_type": schema.StringAttribute{
				Description:         "The collection type: one of " + strings.Join(collectionTypes, ", ") + ". A library without a collection type, which is how Jellyfin's web UI creates a Mixed Movies and Shows library, reads as mixed: Jellyfin treats the two the same.",
				MarkdownDescription: "The collection type: one of `" + strings.Join(collectionTypes, "`, `") + "`. A library without a collection type, which is how Jellyfin's web UI creates a Mixed Movies and Shows library, reads as `mixed`: Jellyfin treats the two the same.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(collectionTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(collectionTypeRequiresReplace,
						"Changing the collection type replaces the library, except from an empty string to mixed.",
						"Changing the collection type replaces the library, except from an empty string to `mixed`."),
				},
			},
			"paths": schema.ListAttribute{
				Description:         "Paths of the library's media folders. Jellyfin looks them up on the server, so when it runs in a container they must be paths inside the container. Changing the paths replaces the library; changing only their order does not.",
				MarkdownDescription: "Paths of the library's media folders. Jellyfin looks them up on the server, so when it runs in a container they must be paths inside the container. Changing the paths replaces the library; changing only their order does not.",
				Required:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplaceIf(pathsRequireReplace,
						"Changing the paths replaces the library; changing only their order does not.",
						"Changing the paths replaces the library; changing only their order does not."),
				},
			},
			"library_options": schema.SingleNestedAttribute{
				Description:         "Typed library options.",
				MarkdownDescription: "Typed library options.",
				Optional:            true,
				Computed:            true,
				Attributes:          libraryOptionsAttributes(),
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
			},
			"item_id": schema.StringAttribute{
				Description:         "The internal item ID assigned by Jellyfin.",
				MarkdownDescription: "The internal item ID assigned by Jellyfin.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"id": schema.StringAttribute{
				Description:         "The library resource identifier.",
				MarkdownDescription: "The library resource identifier.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Jellyfin parses a collection type case-insensitively but returns it in this
// spelling, so any other spelling would read back as a different value.
var collectionTypes = []string{"movies", "tvshows", "music", "musicvideos", "homevideos", "boxsets", "books", "mixed"}

// LibraryCollectionType returns the collection_type jellyfin_library reads for
// a library Jellyfin lists with served, and whether the resource accepts it.
func LibraryCollectionType(served string) (string, bool) {
	collectionType := flattenCollectionType(served).ValueString()
	return collectionType, slices.Contains(collectionTypes, collectionType)
}

func flattenCollectionType(collectionType string) types.String {
	return types.StringValue(cmp.Or(collectionType, "mixed"))
}

func collectionTypeRequiresReplace(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = !req.StateValue.Equal(types.StringValue("")) || !req.PlanValue.Equal(types.StringValue("mixed"))
}

func pathsRequireReplace(ctx context.Context, req planmodifier.ListRequest, resp *listplanmodifier.RequiresReplaceIfFuncResponse) {
	var planned, prior []string
	if req.PlanValue.ElementsAs(ctx, &planned, false).HasError() || req.StateValue.ElementsAs(ctx, &prior, false).HasError() {
		resp.RequiresReplace = true
		return
	}
	resp.RequiresReplace = !samePaths(planned, prior)
}

// pathsInOrder returns the served locations in the order of want when both
// hold the same paths.
func pathsInOrder(ctx context.Context, want types.List, served []string) (types.List, diag.Diagnostics) {
	var wanted []string
	if !want.IsNull() && !want.IsUnknown() && !want.ElementsAs(ctx, &wanted, false).HasError() && samePaths(wanted, served) {
		served = wanted
	}
	return types.ListValueFrom(ctx, types.StringType, served)
}

func samePaths(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

func libraryOptionsAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"enable_photos":                                 optionalBool("Whether photos are enabled."),
		"enable_realtime_monitor":                       optionalBool("Whether realtime monitoring is enabled."),
		"enable_emby_photos":                            unsupportedLibraryOption.boolAttribute("Whether Emby photos are enabled."),
		"enable_photo_subtitle":                         unsupportedLibraryOption.boolAttribute("Whether photo subtitles are enabled."),
		"extract_chapters_during_library_scan":          optionalBool("Whether chapter images are extracted during the library scan."),
		"enable_chapter_image_extraction":               optionalBool("Whether chapter image extraction is enabled."),
		"chapter_image_interval_seconds":                unsupportedLibraryOption.intAttribute("Chapter image interval in seconds."),
		"enable_trickplay_image_extraction":             optionalBool("Whether trickplay images, the previews shown while seeking, are extracted."),
		"extract_trickplay_images_during_library_scan":  optionalBool("Whether trickplay images are extracted during the library scan, instead of only by the trickplay scheduled task."),
		"save_trickplay_with_media":                     optionalBool("Whether trickplay images are saved in the media folders, next to the media, instead of in Jellyfin's data folder."),
		"extract_media_information_during_library_scan": unsupportedLibraryOption.boolAttribute("Whether media information is extracted during library scan."),
		"download_images_in_advance":                    unsupportedLibraryOption.boolAttribute("Whether images are downloaded in advance."),
		"cache_images_in_library":                       unsupportedLibraryOption.boolAttribute("Whether images are cached in the library."),
		"enable_media_conversion":                       unsupportedLibraryOption.boolAttribute("Whether media conversion is enabled."),
		"path_infos": schema.ListNestedAttribute{
			NestedObject: schema.NestedAttributeObject{
				Attributes: pathInfoAttributes(),
			},
			Description:         "Path information entries.",
			MarkdownDescription: "Path information entries.",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		},
		"preferred_metadata_language":      optionalString("Preferred metadata language."),
		"metadata_country_code":            optionalString("Metadata country code."),
		"metadata_savers":                  optionalStringList("Enabled metadata savers, such as `Nfo`, which save metadata into the media folders."),
		"disabled_metadata_savers":         unsupportedLibraryOption.stringListAttribute("Disabled metadata savers; Jellyfin only has the enabled ones, which `metadata_savers` sets."),
		"local_metadata_reader_order":      optionalStringList("Local metadata reader order."),
		"disabled_metadata_fetchers":       unsupportedLibraryOption.stringListAttribute("Disabled metadata fetchers for the whole library; Jellyfin only has them per item type, where `metadata_fetchers` in `type_options` enables the fetchers it lists."),
		"metadata_fetcher_order":           unsupportedLibraryOption.stringListAttribute("Metadata fetcher order for the whole library; Jellyfin only has it per item type, which `metadata_fetchers` in `type_options` sets."),
		"disabled_image_fetchers":          unsupportedLibraryOption.stringListAttribute("Disabled image fetchers for the whole library; Jellyfin only has them per item type, where `image_fetchers` in `type_options` enables the fetchers it lists."),
		"image_fetcher_order":              unsupportedLibraryOption.stringListAttribute("Image fetcher order for the whole library; Jellyfin only has it per item type, which `image_fetchers` in `type_options` sets."),
		"subtitle_fetchers":                combinedStringList(subtitleFetchersDescription, "disabled_subtitle_fetchers", "subtitle_fetcher_order"),
		"disabled_subtitle_fetchers":       replacedBy(optionalStringList("Disabled subtitle fetchers."), subtitleFetchersDeprecation, "subtitle_fetchers"),
		"subtitle_fetcher_order":           replacedBy(optionalStringList("Subtitle fetcher order."), subtitleFetchersDeprecation, "subtitle_fetchers"),
		"save_local_metadata":              optionalBool("Whether local metadata is saved."),
		"save_local_thumbnail_sets":        unsupportedLibraryOption.boolAttribute("Whether local thumbnail sets are saved."),
		"import_missing_episodes":          unsupportedLibraryOption.boolAttribute("Whether missing episodes are imported."),
		"enable_automatic_series_grouping": optionalBool("Whether automatic series grouping is enabled."),
		"season_zero_display_name":         optionalString("Season zero display name."),
		"metadata_refresh_mode":            unsupportedLibraryOption.stringAttribute("Metadata refresh mode."),
		"disabled":                         optionalBool("Whether the library is disabled, the inverse of Jellyfin's `Enabled` option."),
		"type_options": schema.ListNestedAttribute{
			NestedObject: schema.NestedAttributeObject{
				Attributes: typeOptionsAttributes(),
			},
			Description:         "Type-specific options. The list replaces the server's list; each entry is applied over the server's entry with the same type, so attributes left unset keep the server's values.",
			MarkdownDescription: "Type-specific options. The list replaces the server's list; each entry is applied over the server's entry with the same `type`, so attributes left unset keep the server's values.",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		},
	}
}

func pathInfoAttributes() map[string]schema.Attribute {
	password := unsupportedLibraryOption.stringAttribute("Password.")
	password.Sensitive = true
	return map[string]schema.Attribute{
		"path": optionalString("Local path."),
		"network_path": schema.StringAttribute{
			Description:         "Network path. " + networkPathRemovedMessage,
			MarkdownDescription: "Network path. " + networkPathRemovedMessage,
			Optional:            true,
			Computed:            true,
			DeprecationMessage:  networkPathRemovedMessage,
			PlanModifiers: []planmodifier.String{
				priorValueEvenIfNull{},
			},
		},
		"username": unsupportedLibraryOption.stringAttribute("Username."),
		"password": password,
	}
}

const networkPathRemovedMessage = "Jellyfin 10.10 removed network paths, so setting it is an error on Jellyfin 10.10 and later."

type priorValueEvenIfNull struct{}

func (priorValueEvenIfNull) Description(context.Context) string {
	return "An unset value is planned as its prior value, null included."
}

func (m priorValueEvenIfNull) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (priorValueEvenIfNull) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.ConfigValue.IsNull() && req.PlanValue.IsUnknown() {
		resp.PlanValue = req.StateValue
	}
}

const unsupportedLibraryOptionMessage = "Jellyfin has no such library option, so setting it is an error. The attribute will be removed in a future release."

var unsupportedLibraryOption = unsupported{
	message: unsupportedLibraryOptionMessage,
	reject: unsetValidator{
		summary: "Unsupported library option",
		reason:  "Jellyfin has no such library option, so the server would ignore this value",
	},
}

func typeOptionsAttributes() map[string]schema.Attribute {
	deprecatedOrder := func(desc, deprecation string) schema.ListAttribute {
		a := optionalStringList(desc + " " + deprecation)
		a.DeprecationMessage = deprecation
		return a
	}
	return map[string]schema.Attribute{
		"type":                   optionalString("Item type."),
		"metadata_fetchers":      optionalStringList("Enabled metadata fetchers for this type, in priority order: Jellyfin asks the first one first. " + ordersNote("metadata fetcher", "metadata_fetcher_order")),
		"metadata_fetcher_order": deprecatedOrder("Metadata fetcher order for this type.", orderDeprecation("metadata_fetchers", "metadata fetchers")),
		"image_fetchers":         optionalStringList("Enabled image fetchers for this type, in priority order: Jellyfin asks the first one first. " + ordersNote("image fetcher", "image_fetcher_order")),
		"image_options": schema.ListNestedAttribute{
			NestedObject: schema.NestedAttributeObject{
				Attributes: imageOptionsAttributes(),
			},
			Description:         "Image options for this type. Each entry is applied over the server's entry with the same image type.",
			MarkdownDescription: "Image options for this type. Each entry is applied over the server's entry with the same image `type`.",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		},
		"image_fetcher_order":         deprecatedOrder("Image fetcher order for this type.", orderDeprecation("image_fetchers", "image fetchers")),
		"similar_item_providers":      optionalStringList("Enabled similar item providers for this type, in priority order; Jellyfin always uses its local ones, such as Local Genre/Tag, which the list only ranks. " + ordersNote("similar item provider", "similar_item_provider_order") + " " + similarItemsNote),
		"similar_item_provider_order": deprecatedOrder("Similar item provider order for this type. "+similarItemsNote, orderDeprecation("similar_item_providers", "similar item providers")),
	}
}

const similarItemsNote = "Needs Jellyfin 12 or later: on Jellyfin 10.x it reads as null and setting it is an error."

func ordersNote(kind, orderAttr string) string {
	return fmt.Sprintf("Unless `%[1]s` is set, changing the list also sets Jellyfin's %[2]s order: these names, then the other names the server's order held. The list does not read that order back, so while it stays as it is, Jellyfin keeps the order it has, including one set by `%[1]s` or outside Terraform.", orderAttr, kind)
}

func orderDeprecation(replacement, kinds string) string {
	return fmt.Sprintf("Deprecated: list the enabled %[1]s in priority order in `%[2]s` instead, which then sets the order. Set, it still overrides that order; removed, the order it set stays until `%[2]s` changes. It will be removed in a future release.", kinds, replacement)
}

const subtitleFetchersDescription = "Enabled subtitle fetchers, in priority order: Jellyfin asks the first one first and disables every other subtitle fetcher it offers. Subtitle fetchers come from plugins, such as Open Subtitles, and each name must match one the server offers exactly. Jellyfin enables a subtitle fetcher installed later, which then shows up as a change to this list. Conflicts with `disabled_subtitle_fetchers` and `subtitle_fetcher_order`, which it replaces."

const subtitleFetchersDeprecation = "Deprecated: list the enabled subtitle fetchers in priority order in `subtitle_fetchers` instead, which disables the rest. It will be removed in a future release."

// Jellyfin parses an image option's type case-insensitively but returns it in
// this spelling, so any other spelling would read back as a different value.
var imageTypes = []string{"Primary", "Art", "Backdrop", "Banner", "Logo", "Thumb", "Disc", "Box", "Screenshot", "Menu", "Chapter", "BoxRear", "Profile"}

func imageOptionsAttributes() map[string]schema.Attribute {
	typeDescription := "Image type: one of " + strings.Join(imageTypes, ", ") + "."
	return map[string]schema.Attribute{
		"type": schema.StringAttribute{
			Description:         typeDescription,
			MarkdownDescription: "Image type: one of `" + strings.Join(imageTypes, "`, `") + "`.",
			Optional:            true,
			Computed:            true,
			Validators: []validator.String{
				stringvalidator.OneOf(imageTypes...),
			},
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"limit": schema.Int64Attribute{
			Description:         "Image limit.",
			MarkdownDescription: "Image limit.",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
		"min_width": schema.Int64Attribute{
			Description:         "Minimum image width in pixels.",
			MarkdownDescription: "Minimum image width in pixels.",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
	}
}

func (r *LibraryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *LibraryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	b := wireBinding(&resp.Diagnostics, libraryOptionsWire)
	if b == nil {
		return
	}

	// library_options may be unknown, which the model's pointer field cannot
	// hold, so read the attributes one by one.
	var data LibraryResourceModel
	var opts types.Object
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("name"), &data.Name)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("collection_type"), &data.CollectionType)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("paths"), &data.Paths)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("library_options"), &opts)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !opts.IsNull() && !opts.IsUnknown() {
		var lo LibraryOptionsModel
		resp.Diagnostics.Append(opts.As(ctx, &lo, basetypes.ObjectAsOptions{})...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.LibraryOptions = &lo
	}

	var paths []string
	resp.Diagnostics.Append(data.Paths.ElementsAs(ctx, &paths, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = wire.WithAvailable(ctx, newOfferedProviders(r.client).forLibrary(data.CollectionType.ValueString()))
	if d := b.OverlayModel(ctx, map[string]json.RawMessage{}, &data); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}

	switch _, err := r.findFolder(ctx, data.Name.ValueString()); {
	case err == nil:
		resp.Diagnostics.AddError(
			"Library already exists",
			fmt.Sprintf("A library named %q already exists on the Jellyfin server. Import it into this resource with its name as the import ID, or choose another name.", data.Name.ValueString()),
		)
		return
	case !errors.Is(err, errLibraryNotFound):
		resp.Diagnostics.AddError("Failed to read libraries", err.Error())
		return
	}

	if err := r.client.AddVirtualFolder(ctx, data.Name.ValueString(), data.CollectionType.ValueString(), paths, nil); err != nil {
		resp.Diagnostics.AddError("Failed to create library", err.Error()+r.missingPathsHint(ctx, err, paths))
		return
	}

	folder, err := r.findFolder(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read library after creation", err.Error())
		return
	}

	// Track the library before applying its options: a failure then taints it
	// in state instead of leaving it unmanaged.
	tracked := LibraryResourceModel{
		ID:             types.StringValue(folder.Name),
		Name:           data.Name,
		CollectionType: flattenCollectionType(folder.CollectionType),
		Paths:          data.Paths,
		ItemID:         types.StringValue(folder.ItemID),
	}
	resp.Diagnostics.Append(b.FlattenInto(ctx, folder.GetLibraryOptions().RawJSON, &tracked)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &tracked)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.writeOptions(ctx, b, folder, &data, &resp.Diagnostics, &resp.State)
}

func (r *LibraryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	b := wireBinding(&resp.Diagnostics, libraryOptionsWire)
	if b == nil {
		return
	}

	var data LibraryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	folder, err := r.findFolder(ctx, data.Name.ValueString())
	if err != nil {
		if errors.Is(err, errLibraryNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read library", err.Error())
		return
	}
	ctx = wire.WithAvailable(ctx, newOfferedProviders(r.client).forLibrary(folder.CollectionType))

	resp.Diagnostics.Append(readFolder(ctx, folder, &data)...)
	resp.Diagnostics.Append(b.FlattenInto(ctx, folder.GetLibraryOptions().RawJSON, &data)...)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LibraryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	b := wireBinding(&resp.Diagnostics, libraryOptionsWire)
	if b == nil {
		return
	}

	var data LibraryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state LibraryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var plannedOptions, priorOptions types.Object
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("library_options"), &plannedOptions)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("library_options"), &priorOptions)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if unchangedOptions(plannedOptions, priorOptions) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("library_options"), priorOptions)...)
		return
	}

	folder, err := r.findFolder(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read library for update", err.Error())
		return
	}
	ctx = wire.WithAvailable(ctx, newOfferedProviders(r.client).forLibrary(folder.CollectionType))

	r.writeOptions(ctx, b, folder, &data, &resp.Diagnostics, &resp.State)
}

// writeOptions writes the planned library options over those folder serves,
// and stores the library as the server then lists it.
func (r *LibraryResource) writeOptions(ctx context.Context, b *wire.Binding, folder *client.VirtualFolder, data *LibraryResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	base, err := parseJSONObject(folder.GetLibraryOptions().RawJSON)
	if err != nil {
		diags.AddError("Failed to parse library options", err.Error())
		return
	}

	if d := b.OverlayModel(ctx, base, data); d.HasError() {
		diags.Append(d...)
		return
	}

	payload, err := json.Marshal(base)
	if err != nil {
		diags.AddError("Failed to serialize library options", err.Error())
		return
	}

	if err := r.client.UpdateVirtualFolder(ctx, folder.ItemID, &client.LibraryOptions{RawJSON: string(payload)}); err != nil {
		diags.AddError("Failed to update library options", err.Error())
		return
	}

	updated, err := r.findFolder(ctx, folder.Name)
	if err != nil {
		diags.AddError("Failed to read library after options update", err.Error())
		return
	}

	diags.Append(readFolder(ctx, updated, data)...)
	diags.Append(b.FlattenAfterApply(ctx, updated.GetLibraryOptions().RawJSON, data)...)
	diags.Append(state.Set(ctx, data)...)
}

// readFolder sets the attributes the library listing holds outside the
// library options.
func readFolder(ctx context.Context, folder *client.VirtualFolder, data *LibraryResourceModel) diag.Diagnostics {
	data.ID = types.StringValue(folder.Name)
	data.ItemID = types.StringValue(folder.ItemID)
	data.CollectionType = flattenCollectionType(folder.CollectionType)
	paths, diags := pathsInOrder(ctx, data.Paths, folder.Locations)
	data.Paths = paths
	return diags
}

func (r *LibraryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data LibraryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RemoveVirtualFolder(ctx, data.Name.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete library", err.Error())
	}
}

// unchangedOptions reports whether planned holds the prior options, counting
// an attribute planned unknown over a null prior as unchanged.
func unchangedOptions(planned, prior types.Object) bool {
	if planned.IsNull() || planned.IsUnknown() || prior.IsNull() || prior.IsUnknown() {
		return planned.Equal(prior)
	}
	priorAttrs := prior.Attributes()
	for name, v := range planned.Attributes() {
		p, ok := priorAttrs[name]
		unchanged := ok && (v.Equal(p) || v.IsUnknown() && p.IsNull())
		if !unchanged {
			return false
		}
	}
	return true
}

var errLibraryNotFound = errors.New("library not found")

func (r *LibraryResource) findFolder(ctx context.Context, name string) (*client.VirtualFolder, error) {
	folders, err := r.client.GetVirtualFolders(ctx)
	if err != nil {
		return nil, err
	}
	for i := range folders {
		if folders[i].Name == name {
			return &folders[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %q", errLibraryNotFound, name)
}

// missingPathsHint names the paths the server cannot find when it rejected a
// create with a 400.
func (r *LibraryResource) missingPathsHint(ctx context.Context, createErr error, paths []string) string {
	var httpErr *client.HTTPError
	if !errors.As(createErr, &httpErr) || httpErr.StatusCode != http.StatusBadRequest {
		return ""
	}
	var missing []string
	for _, p := range paths {
		if exists, err := r.client.DirectoryExists(ctx, p); err == nil && !exists {
			missing = append(missing, strconv.Quote(p))
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("\n\nThese paths do not exist on the Jellyfin server: %s. Paths are looked up by the server, so when Jellyfin runs in a container they must be the paths where the media is mounted inside the container.", strings.Join(missing, ", "))
}

func (r *LibraryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

// ModifyPlan rejects attributes the server's Jellyfin version does not have,
// and plans each type_options attribute left unset from the prior entry with
// the same type, which is the server entry apply writes over.
// UseStateForUnknown takes it from the prior entry at the same index instead,
// so inserting or reordering entries would plan, and then write, another
// type's values.
func (r *LibraryResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	checkServerHasFields(ctx, r.client, libraryWire, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() || req.State.Raw.IsNull() {
		return
	}

	typeOptionsPath := path.Root("library_options").AtName("type_options")
	var config, plan, state types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, typeOptionsPath, &config)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, typeOptionsPath, &plan)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, typeOptionsPath, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.IsNull() || config.IsUnknown() || plan.IsNull() || plan.IsUnknown() {
		return
	}

	planned, diags := planTypeOptionsByType(ctx, config, plan, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, typeOptionsPath, planned)...)
}

func planTypeOptionsByType(ctx context.Context, config, plan, state types.List) (types.List, diag.Diagnostics) {
	unknownList := types.ListUnknown(types.StringType)
	return replanEntries(ctx, config, plan, state, func(_ int, c TypeOptionsModel, p *TypeOptionsModel, s []TypeOptionsModel) diag.Diagnostics {
		prior, found := entryWithType(s, c.Type, func(e TypeOptionsModel) types.String { return e.Type })
		p.MetadataFetchers = unsetFromPrior(c.MetadataFetchers, p.MetadataFetchers, prior.MetadataFetchers, found, unknownList)
		p.MetadataFetcherOrder = orderFromPrior(c.MetadataFetchers, prior.MetadataFetchers, c.MetadataFetcherOrder, p.MetadataFetcherOrder, prior.MetadataFetcherOrder, found)
		p.ImageFetchers = unsetFromPrior(c.ImageFetchers, p.ImageFetchers, prior.ImageFetchers, found, unknownList)
		p.ImageFetcherOrder = orderFromPrior(c.ImageFetchers, prior.ImageFetchers, c.ImageFetcherOrder, p.ImageFetcherOrder, prior.ImageFetcherOrder, found)
		p.SimilarItemProviders = unsetFromPrior(c.SimilarItemProviders, p.SimilarItemProviders, prior.SimilarItemProviders, found, unknownList)
		p.SimilarItemProviderOrder = orderFromPrior(c.SimilarItemProviders, prior.SimilarItemProviders, c.SimilarItemProviderOrder, p.SimilarItemProviderOrder, prior.SimilarItemProviderOrder, found)

		if c.ImageOptions.IsNull() {
			p.ImageOptions = unsetFromPrior(c.ImageOptions, p.ImageOptions, prior.ImageOptions, found, types.ListUnknown(p.ImageOptions.ElementType(ctx)))
			return nil
		}
		imageOptions, diags := planImageOptionsByType(ctx, c.ImageOptions, p.ImageOptions, prior.ImageOptions)
		p.ImageOptions = imageOptions
		return diags
	})
}

func planImageOptionsByType(ctx context.Context, config, plan, state types.List) (types.List, diag.Diagnostics) {
	if config.IsUnknown() || plan.IsNull() || plan.IsUnknown() {
		return plan, nil
	}
	return replanEntries(ctx, config, plan, state, func(_ int, c ImageOptionsModel, p *ImageOptionsModel, s []ImageOptionsModel) diag.Diagnostics {
		prior, found := entryWithType(s, c.Type, func(e ImageOptionsModel) types.String { return e.Type })
		p.Limit = unsetFromPrior(c.Limit, p.Limit, prior.Limit, found, types.Int64Unknown())
		p.MinWidth = unsetFromPrior(c.MinWidth, p.MinWidth, prior.MinWidth, found, types.Int64Unknown())
		return nil
	})
}

// replanEntries returns plan as it is when config, plan or a known state does
// not decode, or config and plan hold different numbers of entries.
func replanEntries[T any](ctx context.Context, config, plan, state types.List, replan func(i int, config T, planned *T, prior []T) diag.Diagnostics) (types.List, diag.Diagnostics) {
	var c, p, s []T
	if config.ElementsAs(ctx, &c, false).HasError() || plan.ElementsAs(ctx, &p, false).HasError() || len(c) != len(p) {
		return plan, nil
	}
	if !state.IsNull() && !state.IsUnknown() && state.ElementsAs(ctx, &s, false).HasError() {
		return plan, nil
	}

	var diags diag.Diagnostics
	for i := range p {
		diags.Append(replan(i, c[i], &p[i], s)...)
		if diags.HasError() {
			return plan, diags
		}
	}
	out, d := types.ListValueFrom(ctx, plan.ElementType(ctx), p)
	diags.Append(d...)
	return out, diags
}

// entryWithType matches the way Jellyfin looks options up: the first entry
// whose type equals typ, ignoring case.
func entryWithType[T any](entries []T, typ types.String, typeOf func(T) types.String) (T, bool) {
	var zero T
	if typ.IsNull() || typ.IsUnknown() {
		return zero, false
	}
	for _, e := range entries {
		if t := typeOf(e); !t.IsNull() && !t.IsUnknown() && strings.EqualFold(t.ValueString(), typ.ValueString()) {
			return e, true
		}
	}
	return zero, false
}

// orderFromPrior plans an unset order unknown while its list is configured
// to other names than the prior entry's, as the list then writes it.
func orderFromPrior(configuredList, priorList, configured, planned, prior types.List, found bool) types.List {
	if configured.IsNull() && !configuredList.IsNull() && (!found || !configuredList.Equal(priorList)) {
		return types.ListUnknown(types.StringType)
	}
	return unsetFromPrior(configured, planned, prior, found, types.ListUnknown(types.StringType))
}

// unsetFromPrior plans an unset attribute unknown when there is no prior entry
// of the same type, because the server fills it in.
func unsetFromPrior[T attr.Value](configured, planned, prior T, found bool, unknown T) T {
	switch {
	case !configured.IsNull():
		return planned
	case found:
		return prior
	default:
		return unknown
	}
}
