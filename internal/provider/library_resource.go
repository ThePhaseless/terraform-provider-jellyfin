// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
		wire.Orders("library_options.type_options.metadata_fetchers", "metadata_fetcher_order"),
		wire.Orders("library_options.type_options.image_fetchers", "image_fetcher_order"),
		wire.Orders("library_options.type_options.similar_item_providers", "similar_item_provider_order"),
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

// libraryOptionsWire writes POST /Library/VirtualFolders/LibraryOptions and
// reads the options of the library listing; the rest of the library comes
// from the typed folder.
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
	ChapterImageIntervalSeconds              types.Int64  `tfsdk:"chapter_image_interval_seconds"`
	ExtractMediaInformationDuringLibraryScan types.Bool   `tfsdk:"extract_media_information_during_library_scan"`
	DownloadImagesInAdvance                  types.Bool   `tfsdk:"download_images_in_advance"`
	CacheImagesInLibrary                     types.Bool   `tfsdk:"cache_images_in_library"`
	EnableMediaConversion                    types.Bool   `tfsdk:"enable_media_conversion"`
	PathInfos                                types.List   `tfsdk:"path_infos"`
	PreferredMetadataLanguage                types.String `tfsdk:"preferred_metadata_language"`
	MetadataCountryCode                      types.String `tfsdk:"metadata_country_code"`
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

// PathInfoModel describes one PathInfo entry.
type PathInfoModel struct {
	Path        types.String `tfsdk:"path"`
	NetworkPath types.String `tfsdk:"network_path"`
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
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
				Description:         "Paths of the library's media folders. Jellyfin looks them up on the server, so when it runs in a container they must be paths inside the container.",
				MarkdownDescription: "Paths of the library's media folders. Jellyfin looks them up on the server, so when it runs in a container they must be paths inside the container.",
				Required:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
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

// Jellyfin gives a library created without a collection type the same null
// collection type as one created as mixed, and only its library listing tells
// them apart, so reading it as an empty string would force a replacement that
// changes nothing.
func flattenCollectionType(collectionType string) types.String {
	if collectionType == "" {
		return types.StringValue("mixed")
	}
	return types.StringValue(collectionType)
}

// Earlier provider versions stored a library without a collection type as "".
// A refresh reads that as mixed, but a plan that skips the refresh, as
// -refresh=false does, still compares mixed against "", and replacing the
// library there would delete and recreate it for a collection type it already
// has.
func collectionTypeRequiresReplace(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = !req.StateValue.Equal(types.StringValue("")) || !req.PlanValue.Equal(types.StringValue("mixed"))
}

func libraryOptionsAttributes() map[string]schema.Attribute {
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
	unsupportedBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{
			Description:         desc + " " + unsupportedLibraryOptionMessage,
			MarkdownDescription: desc + " " + unsupportedLibraryOptionMessage,
			Optional:            true,
			DeprecationMessage:  unsupportedLibraryOptionMessage,
			Validators:          []validator.Bool{unsupportedLibraryOptionValidator{}},
		}
	}
	unsupportedInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{
			Description:         desc + " " + unsupportedLibraryOptionMessage,
			MarkdownDescription: desc + " " + unsupportedLibraryOptionMessage,
			Optional:            true,
			DeprecationMessage:  unsupportedLibraryOptionMessage,
			Validators:          []validator.Int64{unsupportedLibraryOptionValidator{}},
		}
	}
	unsupportedString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Description:         desc + " " + unsupportedLibraryOptionMessage,
			MarkdownDescription: desc + " " + unsupportedLibraryOptionMessage,
			Optional:            true,
			DeprecationMessage:  unsupportedLibraryOptionMessage,
			Validators:          []validator.String{unsupportedLibraryOptionValidator{}},
		}
	}
	unsupportedStringList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{
			ElementType:         types.StringType,
			Description:         desc + " " + unsupportedLibraryOptionMessage,
			MarkdownDescription: desc + " " + unsupportedLibraryOptionMessage,
			Optional:            true,
			DeprecationMessage:  unsupportedLibraryOptionMessage,
			Validators:          []validator.List{unsupportedLibraryOptionValidator{}},
		}
	}

	return map[string]schema.Attribute{
		"enable_photos":                                 optionalBool("Whether photos are enabled."),
		"enable_realtime_monitor":                       optionalBool("Whether realtime monitoring is enabled."),
		"enable_emby_photos":                            unsupportedBool("Whether Emby photos are enabled."),
		"enable_photo_subtitle":                         unsupportedBool("Whether photo subtitles are enabled."),
		"extract_chapters_during_library_scan":          optionalBool("Whether chapter images are extracted during the library scan."),
		"enable_chapter_image_extraction":               optionalBool("Whether chapter image extraction is enabled."),
		"chapter_image_interval_seconds":                unsupportedInt("Chapter image interval in seconds."),
		"extract_media_information_during_library_scan": unsupportedBool("Whether media information is extracted during library scan."),
		"download_images_in_advance":                    unsupportedBool("Whether images are downloaded in advance."),
		"cache_images_in_library":                       unsupportedBool("Whether images are cached in the library."),
		"enable_media_conversion":                       unsupportedBool("Whether media conversion is enabled."),
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
		"disabled_metadata_savers":         unsupportedStringList("Disabled metadata savers."),
		"local_metadata_reader_order":      optionalStringList("Local metadata reader order."),
		"disabled_metadata_fetchers":       unsupportedStringList("Disabled metadata fetchers."),
		"metadata_fetcher_order":           unsupportedStringList("Metadata fetcher order for the whole library; Jellyfin only has it per item type, as `metadata_fetcher_order` in `type_options`."),
		"disabled_image_fetchers":          unsupportedStringList("Disabled image fetchers."),
		"image_fetcher_order":              unsupportedStringList("Image fetcher order for the whole library; Jellyfin only has it per item type, as `image_fetcher_order` in `type_options`."),
		"subtitle_fetchers":                combinedStringList(subtitleFetchersDescription, "", "disabled_subtitle_fetchers", "subtitle_fetcher_order"),
		"disabled_subtitle_fetchers":       replacedBy(optionalStringList("Disabled subtitle fetchers."), subtitleFetchersDeprecation, "", "subtitle_fetchers"),
		"subtitle_fetcher_order":           replacedBy(optionalStringList("Subtitle fetcher order."), subtitleFetchersDeprecation, "", "subtitle_fetchers"),
		"save_local_metadata":              optionalBool("Whether local metadata is saved."),
		"save_local_thumbnail_sets":        unsupportedBool("Whether local thumbnail sets are saved."),
		"import_missing_episodes":          unsupportedBool("Whether missing episodes are imported."),
		"enable_automatic_series_grouping": optionalBool("Whether automatic series grouping is enabled."),
		"season_zero_display_name":         optionalString("Season zero display name."),
		"metadata_refresh_mode":            unsupportedString("Metadata refresh mode."),
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
	unsupportedString := func(desc string, sensitive bool) schema.StringAttribute {
		return schema.StringAttribute{
			Description:         desc + " " + unsupportedLibraryOptionMessage,
			MarkdownDescription: desc + " " + unsupportedLibraryOptionMessage,
			Optional:            true,
			Sensitive:           sensitive,
			DeprecationMessage:  unsupportedLibraryOptionMessage,
			Validators:          []validator.String{unsupportedLibraryOptionValidator{}},
		}
	}
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
		"username": unsupportedString("Username.", false),
		"password": unsupportedString("Password.", true),
	}
}

const networkPathRemovedMessage = "Jellyfin 10.10 removed network paths, so setting it is an error on Jellyfin 10.10 and later."

// Jellyfin before 10.10 keeps a network path set in its web UI, which planning
// the prior value carries into the options apply writes back. Unlike
// UseStateForUnknown, which leaves the value unknown when the library is
// created, this plans null there too: apply then sends no network path and the
// server has none, so an unknown value would only show as known after apply.
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

// Attributes with no Jellyfin library option behind them stay in the schema,
// deprecated, so configurations that leave them unset keep working until
// they are removed.
const unsupportedLibraryOptionMessage = "Jellyfin has no such library option, so setting it is an error. The attribute will be removed in a future release."

func typeOptionsAttributes() map[string]schema.Attribute {
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

// Jellyfin enables what the list names but ranks by the order key alone, with
// the names it leaves out last, so the list sets the order too.
func ordersNote(kind, orderAttr string) string {
	return fmt.Sprintf("Unless `%s` is set, writing it also sets Jellyfin's %s order: these names, then the other names the server's order held.", orderAttr, kind)
}

func orderDeprecation(replacement, kinds string) string {
	return fmt.Sprintf("Deprecated: list the enabled %s in priority order in `%s` instead, which then sets the order. Set, it still overrides that order. It will be removed in a future release.", kinds, replacement)
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

func (r *LibraryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	b := wireBinding(&resp.Diagnostics, libraryOptionsWire)
	if b == nil {
		return
	}

	// library_options is computed, so a plan without it carries an unknown
	// object, which the pointer field of the model cannot hold. Read the
	// attributes one by one and convert the options only when they are known.
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

	// Jellyfin does not refuse a duplicate name: it adds the library as
	// "<name>2", and the lookup by name below would then adopt the existing
	// library while the new one is left unmanaged.
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
	ctx = wire.WithAvailable(ctx, newOfferedProviders(r.client).forLibrary(folder.CollectionType))

	// Track the library before its options are applied: a failure below then
	// leaves it in state as tainted, to be replaced, rather than on the server
	// unmanaged, where the next create would refuse the duplicate name.
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

	base, err := parseJSONObject(folder.GetLibraryOptions().RawJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to parse library options", err.Error())
		return
	}

	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}

	payload, err := json.Marshal(base)
	if err != nil {
		resp.Diagnostics.AddError("Failed to serialize library options", err.Error())
		return
	}

	if err := r.client.UpdateVirtualFolder(ctx, folder.ItemID, &client.LibraryOptions{RawJSON: string(payload)}); err != nil {
		resp.Diagnostics.AddError("Failed to update library options", err.Error())
		return
	}

	updated, err := r.findFolder(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read library after options update", err.Error())
		return
	}

	data.ItemID = types.StringValue(updated.ItemID)
	data.ID = types.StringValue(updated.Name)
	data.CollectionType = flattenCollectionType(updated.CollectionType)
	pathValues, diags := types.ListValueFrom(ctx, types.StringType, updated.Locations)
	resp.Diagnostics.Append(diags...)
	data.Paths = pathValues
	resp.Diagnostics.Append(b.FlattenAfterApply(ctx, updated.GetLibraryOptions().RawJSON, &data)...)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
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

	data.CollectionType = flattenCollectionType(folder.CollectionType)
	data.ItemID = types.StringValue(folder.ItemID)
	data.ID = types.StringValue(folder.Name)
	pathValues, diags := types.ListValueFrom(ctx, types.StringType, folder.Locations)
	resp.Diagnostics.Append(diags...)
	data.Paths = pathValues
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

	// With the options unchanged, the update can only be collection_type going
	// from the "" earlier versions stored to mixed, which the server already
	// has. Writing the options anyway would store them as the server reads
	// them, and a plan made without a refresh, which carries the options from
	// state written by an older version, need not match that.
	var plannedOptions, priorOptions types.Object
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("library_options"), &plannedOptions)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("library_options"), &priorOptions)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plannedOptions.Equal(priorOptions) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	folder, err := r.findFolder(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read library for update", err.Error())
		return
	}
	ctx = wire.WithAvailable(ctx, newOfferedProviders(r.client).forLibrary(folder.CollectionType))

	base, err := parseJSONObject(folder.GetLibraryOptions().RawJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to parse library options", err.Error())
		return
	}

	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}

	payload, err := json.Marshal(base)
	if err != nil {
		resp.Diagnostics.AddError("Failed to serialize library options", err.Error())
		return
	}

	if err := r.client.UpdateVirtualFolder(ctx, folder.ItemID, &client.LibraryOptions{RawJSON: string(payload)}); err != nil {
		resp.Diagnostics.AddError("Failed to update library options", err.Error())
		return
	}

	updated, err := r.findFolder(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read library after update", err.Error())
		return
	}

	data.ItemID = types.StringValue(updated.ItemID)
	data.ID = types.StringValue(updated.Name)
	data.CollectionType = flattenCollectionType(updated.CollectionType)
	pathValues, diags := types.ListValueFrom(ctx, types.StringType, updated.Locations)
	resp.Diagnostics.Append(diags...)
	data.Paths = pathValues
	resp.Diagnostics.Append(b.FlattenAfterApply(ctx, updated.GetLibraryOptions().RawJSON, &data)...)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LibraryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data LibraryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RemoveVirtualFolder(ctx, data.Name.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete library", err.Error())
	}
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
// create with a 400. Jellyfin answers a missing path with only "Error
// processing request." and writes the reason to its own log.
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
	b := wireBinding(&resp.Diagnostics, libraryWire)
	if b == nil {
		return
	}
	// The version check runs at plan time because a server that lacks an
	// option drops it only after the library is created, so the failure would
	// taint the new library and every later apply would replace it before
	// failing again.
	resp.Diagnostics.Append(checkServerHasFields(ctx, r.client, b, req.Config)...)
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
	var diags diag.Diagnostics
	var c, p, s []TypeOptionsModel
	if config.ElementsAs(ctx, &c, false).HasError() || plan.ElementsAs(ctx, &p, false).HasError() || len(c) != len(p) {
		return plan, diags
	}
	if !state.IsNull() && !state.IsUnknown() && state.ElementsAs(ctx, &s, false).HasError() {
		return plan, diags
	}

	unknownList := types.ListUnknown(types.StringType)
	for i := range p {
		prior, found := entryWithType(s, c[i].Type, func(e TypeOptionsModel) types.String { return e.Type })
		p[i].MetadataFetchers = unsetFromPrior(c[i].MetadataFetchers, p[i].MetadataFetchers, prior.MetadataFetchers, found, unknownList)
		p[i].MetadataFetcherOrder = orderFromPrior(c[i].MetadataFetchers, prior.MetadataFetchers, c[i].MetadataFetcherOrder, p[i].MetadataFetcherOrder, prior.MetadataFetcherOrder, found)
		p[i].ImageFetchers = unsetFromPrior(c[i].ImageFetchers, p[i].ImageFetchers, prior.ImageFetchers, found, unknownList)
		p[i].ImageFetcherOrder = orderFromPrior(c[i].ImageFetchers, prior.ImageFetchers, c[i].ImageFetcherOrder, p[i].ImageFetcherOrder, prior.ImageFetcherOrder, found)
		p[i].SimilarItemProviders = unsetFromPrior(c[i].SimilarItemProviders, p[i].SimilarItemProviders, prior.SimilarItemProviders, found, unknownList)
		p[i].SimilarItemProviderOrder = orderFromPrior(c[i].SimilarItemProviders, prior.SimilarItemProviders, c[i].SimilarItemProviderOrder, p[i].SimilarItemProviderOrder, prior.SimilarItemProviderOrder, found)

		if c[i].ImageOptions.IsNull() {
			p[i].ImageOptions = unsetFromPrior(c[i].ImageOptions, p[i].ImageOptions, prior.ImageOptions, found, types.ListUnknown(p[i].ImageOptions.ElementType(ctx)))
			continue
		}
		imageOptions, d := planImageOptionsByType(ctx, c[i].ImageOptions, p[i].ImageOptions, prior.ImageOptions)
		diags.Append(d...)
		if diags.HasError() {
			return plan, diags
		}
		p[i].ImageOptions = imageOptions
	}

	out, d := types.ListValueFrom(ctx, plan.ElementType(ctx), p)
	diags.Append(d...)
	return out, diags
}

func planImageOptionsByType(ctx context.Context, config, plan, state types.List) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	if config.IsUnknown() || plan.IsNull() || plan.IsUnknown() {
		return plan, diags
	}
	var c, p, s []ImageOptionsModel
	if config.ElementsAs(ctx, &c, false).HasError() || plan.ElementsAs(ctx, &p, false).HasError() || len(c) != len(p) {
		return plan, diags
	}
	if !state.IsNull() && !state.IsUnknown() && state.ElementsAs(ctx, &s, false).HasError() {
		return plan, diags
	}

	for i := range p {
		prior, found := entryWithType(s, c[i].Type, func(e ImageOptionsModel) types.String { return e.Type })
		p[i].Limit = unsetFromPrior(c[i].Limit, p[i].Limit, prior.Limit, found, types.Int64Unknown())
		p[i].MinWidth = unsetFromPrior(c[i].MinWidth, p[i].MinWidth, prior.MinWidth, found, types.Int64Unknown())
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

// orderFromPrior plans an unset order attribute unknown while its list
// attribute is configured to other names than the prior entry's, as the list
// then writes the order.
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
