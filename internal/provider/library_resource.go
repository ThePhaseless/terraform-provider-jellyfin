// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"strings"

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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

var (
	_ resource.Resource                = &LibraryResource{}
	_ resource.ResourceWithImportState = &LibraryResource{}
	_ resource.ResourceWithModifyPlan  = &LibraryResource{}
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
					stringplanmodifier.RequiresReplace(),
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

// flattenCollectionType reads a library without a collection type as mixed.
// Jellyfin gives such a library the same null collection type as one created
// as mixed, and only its library listing tells them apart, so an empty string
// would force a replacement that changes nothing.
func flattenCollectionType(collectionType string) types.String {
	if collectionType == "" {
		return types.StringValue("mixed")
	}
	return types.StringValue(collectionType)
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
		"disabled_subtitle_fetchers":       optionalStringList("Disabled subtitle fetchers."),
		"subtitle_fetcher_order":           optionalStringList("Subtitle fetcher order."),
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
		// Not computed: Jellyfin 10.11 and later never return a network path,
		// so on them a computed value would only show as known after apply in
		// every plan that sets path_infos.
		"network_path": schema.StringAttribute{
			Description:         "Network path. " + networkPathRemovedMessage,
			MarkdownDescription: "Network path. " + networkPathRemovedMessage,
			Optional:            true,
			DeprecationMessage:  networkPathRemovedMessage,
		},
		"username": unsupportedString("Username.", false),
		"password": unsupportedString("Password.", true),
	}
}

const networkPathRemovedMessage = "Jellyfin 10.11 removed network paths, so setting it is an error on Jellyfin 10.11 and later."

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
	return map[string]schema.Attribute{
		"type":                   optionalString("Item type."),
		"metadata_fetchers":      optionalStringList("Metadata fetchers for this type."),
		"metadata_fetcher_order": optionalStringList("Metadata fetcher order for this type."),
		"image_fetchers":         optionalStringList("Image fetchers for this type."),
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
		"image_fetcher_order":         optionalStringList("Image fetcher order for this type."),
		"similar_item_providers":      optionalStringList("Similar item providers for this type. Needs Jellyfin 12 or later: on Jellyfin 10.x it reads as null and setting it is an error."),
		"similar_item_provider_order": optionalStringList("Similar item provider order for this type. Needs Jellyfin 12 or later: on Jellyfin 10.x it reads as null and setting it is an error."),
	}
}

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

func pathInfoObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"path":         types.StringType,
		"network_path": types.StringType,
		"username":     types.StringType,
		"password":     types.StringType,
	}}
}

func typeOptionsObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"type":                        types.StringType,
		"metadata_fetchers":           types.ListType{ElemType: types.StringType},
		"metadata_fetcher_order":      types.ListType{ElemType: types.StringType},
		"image_fetchers":              types.ListType{ElemType: types.StringType},
		"image_options":               types.ListType{ElemType: imageOptionsObjectType()},
		"image_fetcher_order":         types.ListType{ElemType: types.StringType},
		"similar_item_providers":      types.ListType{ElemType: types.StringType},
		"similar_item_provider_order": types.ListType{ElemType: types.StringType},
	}}
}

func imageOptionsObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"type":      types.StringType,
		"limit":     types.Int64Type,
		"min_width": types.Int64Type,
	}}
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

	// Track the library before its options are applied: a failure below then
	// leaves it in state as tainted, to be replaced, rather than on the server
	// unmanaged, where the next create would refuse the duplicate name.
	resp.Diagnostics.Append(resp.State.Set(ctx, &LibraryResourceModel{
		ID:             types.StringValue(folder.Name),
		Name:           data.Name,
		CollectionType: flattenCollectionType(folder.CollectionType),
		Paths:          data.Paths,
		LibraryOptions: flattenLibraryOptions(ctx, folder.GetLibraryOptions().RawJSON, &resp.Diagnostics),
		ItemID:         types.StringValue(folder.ItemID),
	})...)
	if resp.Diagnostics.HasError() {
		return
	}

	base, err := parseJSONObject(folder.GetLibraryOptions().RawJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to parse library options", err.Error())
		return
	}

	if data.LibraryOptions != nil {
		if d := overlayLibraryOptions(ctx, base, data.LibraryOptions); d.HasError() {
			resp.Diagnostics.Append(d...)
			return
		}
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
	got := flattenLibraryOptions(ctx, updated.GetLibraryOptions().RawJSON, &resp.Diagnostics)
	checkSimilarItemSettingsKept(ctx, data.LibraryOptions, got, &resp.Diagnostics)
	data.LibraryOptions = keepPlannedNulls(ctx, data.LibraryOptions, got)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LibraryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
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

	data.CollectionType = flattenCollectionType(folder.CollectionType)
	data.ItemID = types.StringValue(folder.ItemID)
	data.ID = types.StringValue(folder.Name)
	pathValues, diags := types.ListValueFrom(ctx, types.StringType, folder.Locations)
	resp.Diagnostics.Append(diags...)
	data.Paths = pathValues
	data.LibraryOptions = flattenLibraryOptions(ctx, folder.GetLibraryOptions().RawJSON, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LibraryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
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

	folder, err := r.findFolder(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read library for update", err.Error())
		return
	}

	base, err := parseJSONObject(folder.GetLibraryOptions().RawJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to parse library options", err.Error())
		return
	}

	if data.LibraryOptions != nil {
		if d := overlayLibraryOptions(ctx, base, data.LibraryOptions); d.HasError() {
			resp.Diagnostics.Append(d...)
			return
		}
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
	got := flattenLibraryOptions(ctx, updated.GetLibraryOptions().RawJSON, &resp.Diagnostics)
	checkSimilarItemSettingsKept(ctx, data.LibraryOptions, got, &resp.Diagnostics)
	data.LibraryOptions = keepPlannedNulls(ctx, data.LibraryOptions, got)

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
	resp.Diagnostics.Append(r.checkServerVersion(ctx, req.Config)...)
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

// checkServerVersion runs at plan time because a server that lacks an option
// drops it only after the library is created, so the failure would taint the
// new library and every later apply would replace it before failing again.
func (r *LibraryResource) checkServerVersion(ctx context.Context, config tfsdk.Config) diag.Diagnostics {
	var diags diag.Diagnostics
	similarItems, networkPaths := configuredVersionedAttributes(ctx, config)
	if r.client == nil || (len(similarItems) == 0 && len(networkPaths) == 0) {
		return diags
	}
	info, err := r.client.GetPublicSystemInfo(ctx)
	if err != nil {
		diags.AddError("Failed to read the Jellyfin version", err.Error())
		return diags
	}
	return versionedAttributeErrors(info.Version, similarItems, networkPaths)
}

// configuredVersionedAttributes leaves out values unknown at plan time, which
// may still turn out null. Terraform plans again during apply, once they are
// known, so they are checked before the library is written.
func configuredVersionedAttributes(ctx context.Context, config tfsdk.Config) (similarItems, networkPaths []path.Path) {
	optionsPath := path.Root("library_options")
	isSet := func(v attr.Value) bool { return !v.IsNull() && !v.IsUnknown() }

	var typeOptions []TypeOptionsModel
	var list types.List
	if !config.GetAttribute(ctx, optionsPath.AtName("type_options"), &list).HasError() && isSet(list) &&
		!list.ElementsAs(ctx, &typeOptions, false).HasError() {
		for i, e := range typeOptions {
			entry := optionsPath.AtName("type_options").AtListIndex(i)
			if isSet(e.SimilarItemProviders) {
				similarItems = append(similarItems, entry.AtName("similar_item_providers"))
			}
			if isSet(e.SimilarItemProviderOrder) {
				similarItems = append(similarItems, entry.AtName("similar_item_provider_order"))
			}
		}
	}

	var pathInfos []PathInfoModel
	if !config.GetAttribute(ctx, optionsPath.AtName("path_infos"), &list).HasError() && isSet(list) &&
		!list.ElementsAs(ctx, &pathInfos, false).HasError() {
		for i, e := range pathInfos {
			if isSet(e.NetworkPath) {
				networkPaths = append(networkPaths, optionsPath.AtName("path_infos").AtListIndex(i).AtName("network_path"))
			}
		}
	}
	return similarItems, networkPaths
}

func versionedAttributeErrors(version string, similarItems, networkPaths []path.Path) diag.Diagnostics {
	var diags diag.Diagnostics
	if !hasLeadingDigit(version) {
		return diags
	}
	if compareDottedVersions(version, "12") < 0 {
		for _, p := range similarItems {
			diags.AddAttributeError(p, "Similar item settings not supported",
				fmt.Sprintf("The server runs Jellyfin %s, and similar item providers need Jellyfin 12 or later. Remove %s for this server.", version, p))
		}
	}
	if compareDottedVersions(version, "10.11") >= 0 {
		for _, p := range networkPaths {
			diags.AddAttributeError(p, "Network paths not supported",
				fmt.Sprintf("The server runs Jellyfin %s, and Jellyfin 10.11 removed network paths, so the server would drop the value. Remove %s from the configuration.", version, p))
		}
	}
	return diags
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
		p[i].MetadataFetcherOrder = unsetFromPrior(c[i].MetadataFetcherOrder, p[i].MetadataFetcherOrder, prior.MetadataFetcherOrder, found, unknownList)
		p[i].ImageFetchers = unsetFromPrior(c[i].ImageFetchers, p[i].ImageFetchers, prior.ImageFetchers, found, unknownList)
		p[i].ImageFetcherOrder = unsetFromPrior(c[i].ImageFetcherOrder, p[i].ImageFetcherOrder, prior.ImageFetcherOrder, found, unknownList)
		p[i].SimilarItemProviders = unsetFromPrior(c[i].SimilarItemProviders, p[i].SimilarItemProviders, prior.SimilarItemProviders, found, unknownList)
		p[i].SimilarItemProviderOrder = unsetFromPrior(c[i].SimilarItemProviderOrder, p[i].SimilarItemProviderOrder, prior.SimilarItemProviderOrder, found, unknownList)

		if c[i].ImageOptions.IsNull() {
			p[i].ImageOptions = unsetFromPrior(c[i].ImageOptions, p[i].ImageOptions, prior.ImageOptions, found, types.ListUnknown(imageOptionsObjectType()))
			continue
		}
		imageOptions, d := planImageOptionsByType(ctx, c[i].ImageOptions, p[i].ImageOptions, prior.ImageOptions)
		diags.Append(d...)
		if diags.HasError() {
			return plan, diags
		}
		p[i].ImageOptions = imageOptions
	}

	out, d := types.ListValueFrom(ctx, typeOptionsObjectType(), p)
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

	out, d := types.ListValueFrom(ctx, imageOptionsObjectType(), p)
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

func overlayLibraryOptions(ctx context.Context, m map[string]json.RawMessage, opts *LibraryOptionsModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if opts == nil {
		return diags
	}

	putJSONBool(m, "EnablePhotos", opts.EnablePhotos)
	putJSONBool(m, "EnableRealtimeMonitor", opts.EnableRealtimeMonitor)
	putJSONBool(m, "ExtractChapterImagesDuringLibraryScan", opts.ExtractChaptersDuringLibraryScan)
	putJSONBool(m, "EnableChapterImageExtraction", opts.EnableChapterImageExtraction)
	if d := overlayPathInfos(ctx, m, opts.PathInfos); d.HasError() {
		diags.Append(d...)
		return diags
	}
	putJSONString(m, "PreferredMetadataLanguage", opts.PreferredMetadataLanguage)
	putJSONString(m, "MetadataCountryCode", opts.MetadataCountryCode)
	if d := putJSONStringList(ctx, m, "LocalMetadataReaderOrder", opts.LocalMetadataReaderOrder); d.HasError() {
		diags.Append(d...)
		return diags
	}
	if d := putJSONStringList(ctx, m, "DisabledSubtitleFetchers", opts.DisabledSubtitleFetchers); d.HasError() {
		diags.Append(d...)
		return diags
	}
	if d := putJSONStringList(ctx, m, "SubtitleFetcherOrder", opts.SubtitleFetcherOrder); d.HasError() {
		diags.Append(d...)
		return diags
	}
	putJSONBool(m, "SaveLocalMetadata", opts.SaveLocalMetadata)
	putJSONBool(m, "EnableAutomaticSeriesGrouping", opts.EnableAutomaticSeriesGrouping)
	putJSONString(m, "SeasonZeroDisplayName", opts.SeasonZeroDisplayName)
	if !opts.Disabled.IsNull() && !opts.Disabled.IsUnknown() {
		putJSONBool(m, "Enabled", types.BoolValue(!opts.Disabled.ValueBool()))
	}
	if d := overlayTypeOptions(ctx, m, opts.TypeOptions); d.HasError() {
		diags.Append(d...)
		return diags
	}

	return diags
}

func overlayPathInfos(ctx context.Context, m map[string]json.RawMessage, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}
	var entries []PathInfoModel
	if d := v.ElementsAs(ctx, &entries, false); d.HasError() {
		diags.Append(d...)
		return diags
	}
	rawEntries := make([]map[string]json.RawMessage, len(entries))
	for i, e := range entries {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "Path", e.Path)
		putJSONString(entry, "NetworkPath", e.NetworkPath)
		rawEntries[i] = entry
	}
	b, err := json.Marshal(rawEntries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal path infos", err.Error()))
	}
	m["PathInfos"] = b
	return diags
}

func overlayTypeOptions(ctx context.Context, m map[string]json.RawMessage, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}
	var entries []TypeOptionsModel
	if d := v.ElementsAs(ctx, &entries, false); d.HasError() {
		diags.Append(d...)
		return diags
	}
	existing, err := parseJSONObjectList(m["TypeOptions"])
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to parse type options", err.Error()))
	}
	rawEntries := make([]map[string]json.RawMessage, len(entries))
	for i, e := range entries {
		entry := jsonEntryWithType(existing, e.Type)
		putJSONString(entry, "Type", e.Type)
		for key, list := range map[string]types.List{
			"MetadataFetchers":         e.MetadataFetchers,
			"MetadataFetcherOrder":     e.MetadataFetcherOrder,
			"ImageFetchers":            e.ImageFetchers,
			"ImageFetcherOrder":        e.ImageFetcherOrder,
			"SimilarItemProviders":     e.SimilarItemProviders,
			"SimilarItemProviderOrder": e.SimilarItemProviderOrder,
		} {
			if d := putJSONStringList(ctx, entry, key, list); d.HasError() {
				diags.Append(d...)
				return diags
			}
		}
		if d := overlayImageOptions(ctx, entry, e.ImageOptions); d.HasError() {
			diags.Append(d...)
			return diags
		}
		rawEntries[i] = entry
	}
	b, err := json.Marshal(rawEntries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal type options", err.Error()))
	}
	m["TypeOptions"] = b
	return diags
}

func overlayImageOptions(ctx context.Context, m map[string]json.RawMessage, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}
	var entries []ImageOptionsModel
	if d := v.ElementsAs(ctx, &entries, false); d.HasError() {
		diags.Append(d...)
		return diags
	}
	existing, err := parseJSONObjectList(m["ImageOptions"])
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to parse image options", err.Error()))
	}
	rawEntries := make([]map[string]json.RawMessage, len(entries))
	for i, e := range entries {
		entry := jsonEntryWithType(existing, e.Type)
		putJSONString(entry, "Type", e.Type)
		putJSONInt64(entry, "Limit", e.Limit)
		putJSONInt64(entry, "MinWidth", e.MinWidth)
		rawEntries[i] = entry
	}
	b, err := json.Marshal(rawEntries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal image options", err.Error()))
	}
	m["ImageOptions"] = b
	return diags
}

func parseJSONObjectList(raw json.RawMessage) ([]map[string]json.RawMessage, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func jsonEntryWithType(entries []map[string]json.RawMessage, typ types.String) map[string]json.RawMessage {
	entry := map[string]json.RawMessage{}
	if e, ok := entryWithType(entries, typ, func(e map[string]json.RawMessage) types.String { return getJSONString(e, "Type") }); ok {
		maps.Copy(entry, e)
	}
	return entry
}

func flattenLibraryOptions(ctx context.Context, raw string, diags *diag.Diagnostics) *LibraryOptionsModel {
	m, err := parseJSONObject(raw)
	if err != nil {
		diags.AddError("Failed to parse library options", err.Error())
		return nil
	}

	// The attributes with no Jellyfin library option behind them stay null.
	opts := &LibraryOptionsModel{
		DisabledMetadataSavers:   types.ListNull(types.StringType),
		DisabledMetadataFetchers: types.ListNull(types.StringType),
		MetadataFetcherOrder:     types.ListNull(types.StringType),
		DisabledImageFetchers:    types.ListNull(types.StringType),
		ImageFetcherOrder:        types.ListNull(types.StringType),
	}
	opts.EnablePhotos = getJSONBool(m, "EnablePhotos")
	opts.EnableRealtimeMonitor = getJSONBool(m, "EnableRealtimeMonitor")
	opts.ExtractChaptersDuringLibraryScan = getJSONBool(m, "ExtractChapterImagesDuringLibraryScan")
	opts.EnableChapterImageExtraction = getJSONBool(m, "EnableChapterImageExtraction")
	opts.PathInfos = flattenPathInfos(ctx, m, diags)
	opts.PreferredMetadataLanguage = getJSONString(m, "PreferredMetadataLanguage")
	opts.MetadataCountryCode = getJSONString(m, "MetadataCountryCode")
	opts.LocalMetadataReaderOrder, _ = getJSONStringList(ctx, m, "LocalMetadataReaderOrder")
	opts.DisabledSubtitleFetchers, _ = getJSONStringList(ctx, m, "DisabledSubtitleFetchers")
	opts.SubtitleFetcherOrder, _ = getJSONStringList(ctx, m, "SubtitleFetcherOrder")
	opts.SaveLocalMetadata = getJSONBool(m, "SaveLocalMetadata")
	opts.EnableAutomaticSeriesGrouping = getJSONBool(m, "EnableAutomaticSeriesGrouping")
	opts.SeasonZeroDisplayName = getJSONString(m, "SeasonZeroDisplayName")
	if enabled := getJSONBool(m, "Enabled"); !enabled.IsNull() {
		opts.Disabled = types.BoolValue(!enabled.ValueBool())
	}
	opts.TypeOptions = flattenTypeOptions(ctx, m, diags)
	return opts
}

func flattenPathInfos(_ context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["PathInfos"]
	if !ok || isJSONNull(raw) {
		return types.ListNull(pathInfoObjectType())
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse path infos", err.Error())
		return types.ListNull(pathInfoObjectType())
	}
	objType := pathInfoObjectType()
	objects := make([]attr.Value, len(entries))
	for i, e := range entries {
		attrs := map[string]attr.Value{
			"path":         getJSONString(e, "Path"),
			"network_path": getJSONString(e, "NetworkPath"),
			"username":     types.StringNull(),
			"password":     types.StringNull(),
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

func flattenTypeOptions(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["TypeOptions"]
	if !ok || isJSONNull(raw) {
		return types.ListNull(typeOptionsObjectType())
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse type options", err.Error())
		return types.ListNull(typeOptionsObjectType())
	}
	objType := typeOptionsObjectType()
	objects := make([]attr.Value, len(entries))
	for i, e := range entries {
		attrs := map[string]attr.Value{
			"type":          getJSONString(e, "Type"),
			"image_options": flattenImageOptions(ctx, e, diags),
		}
		for name, key := range map[string]string{
			"metadata_fetchers":           "MetadataFetchers",
			"metadata_fetcher_order":      "MetadataFetcherOrder",
			"image_fetchers":              "ImageFetchers",
			"image_fetcher_order":         "ImageFetcherOrder",
			"similar_item_providers":      "SimilarItemProviders",
			"similar_item_provider_order": "SimilarItemProviderOrder",
		} {
			attrs[name] = types.ListNull(types.StringType)
			if v, d := getJSONStringList(ctx, e, key); !d.HasError() {
				attrs[name] = v
			}
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

func flattenImageOptions(_ context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["ImageOptions"]
	if !ok || isJSONNull(raw) {
		return types.ListNull(imageOptionsObjectType())
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse image options", err.Error())
		return types.ListNull(imageOptionsObjectType())
	}
	objType := imageOptionsObjectType()
	objects := make([]attr.Value, len(entries))
	for i, e := range entries {
		attrs := map[string]attr.Value{
			"type":      getJSONString(e, "Type"),
			"limit":     getJSONInt64(e, "Limit"),
			"min_width": getJSONInt64(e, "MinWidth"),
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

// keepPlannedNulls returns got with, inside type_options and path_infos
// elements, the attributes the plan left null set back to null where the
// server returned an empty list or an empty string. Attributes left unset
// inside list elements are planned from the prior state, where null is a
// known value, so the value after apply has to match the plan exactly.
func keepPlannedNulls(ctx context.Context, planned, got *LibraryOptionsModel) *LibraryOptionsModel {
	if planned == nil || got == nil {
		return got
	}
	got.TypeOptions = reconcileTypeOptions(ctx, planned.TypeOptions, got.TypeOptions)
	got.PathInfos = reconcilePathInfos(ctx, planned.PathInfos, got.PathInfos)
	return got
}

// checkSimilarItemSettingsKept exists because Jellyfin 10.x has no similar
// item settings and drops them, and Terraform's own inconsistent-result error
// cannot name the attribute: library_options holds a sensitive value.
// ModifyPlan rejects configured values earlier, but not values planned from
// the prior state, such as after a server downgrade.
func checkSimilarItemSettingsKept(ctx context.Context, planned, got *LibraryOptionsModel, diags *diag.Diagnostics) {
	if planned == nil || got == nil || planned.TypeOptions.IsNull() || planned.TypeOptions.IsUnknown() || got.TypeOptions.IsNull() || got.TypeOptions.IsUnknown() {
		return
	}
	var p, g []TypeOptionsModel
	if planned.TypeOptions.ElementsAs(ctx, &p, false).HasError() || got.TypeOptions.ElementsAs(ctx, &g, false).HasError() || len(p) != len(g) {
		return
	}
	var dropped []string
	for i := range g {
		if !p[i].SimilarItemProviders.IsNull() && !p[i].SimilarItemProviders.IsUnknown() && g[i].SimilarItemProviders.IsNull() {
			dropped = append(dropped, fmt.Sprintf("type_options[%d].similar_item_providers", i))
		}
		if !p[i].SimilarItemProviderOrder.IsNull() && !p[i].SimilarItemProviderOrder.IsUnknown() && g[i].SimilarItemProviderOrder.IsNull() {
			dropped = append(dropped, fmt.Sprintf("type_options[%d].similar_item_provider_order", i))
		}
	}
	if len(dropped) > 0 {
		diags.AddError(
			"Similar item settings not supported",
			fmt.Sprintf("The Jellyfin server did not keep %s. Similar item providers need Jellyfin 12 or later; remove these attributes for older servers.", strings.Join(dropped, ", ")),
		)
	}
}

func nullIfPlannedNullList(planned types.List, got *types.List) {
	if planned.IsNull() && !got.IsNull() && !got.IsUnknown() && len(got.Elements()) == 0 {
		*got = types.ListNull(got.ElementType(context.Background()))
	}
}

func nullIfPlannedNullString(planned types.String, got *types.String) {
	if planned.IsNull() && !got.IsNull() && !got.IsUnknown() && got.ValueString() == "" {
		*got = types.StringNull()
	}
}

func reconcileTypeOptions(ctx context.Context, planned, got types.List) types.List {
	if planned.IsNull() || planned.IsUnknown() || got.IsNull() || got.IsUnknown() {
		return got
	}
	var p, g []TypeOptionsModel
	if planned.ElementsAs(ctx, &p, false).HasError() || got.ElementsAs(ctx, &g, false).HasError() || len(p) != len(g) {
		return got
	}
	for i := range g {
		nullIfPlannedNullList(p[i].MetadataFetchers, &g[i].MetadataFetchers)
		nullIfPlannedNullList(p[i].MetadataFetcherOrder, &g[i].MetadataFetcherOrder)
		nullIfPlannedNullList(p[i].ImageFetchers, &g[i].ImageFetchers)
		nullIfPlannedNullList(p[i].ImageOptions, &g[i].ImageOptions)
		nullIfPlannedNullList(p[i].ImageFetcherOrder, &g[i].ImageFetcherOrder)
		nullIfPlannedNullList(p[i].SimilarItemProviders, &g[i].SimilarItemProviders)
		nullIfPlannedNullList(p[i].SimilarItemProviderOrder, &g[i].SimilarItemProviderOrder)
	}
	out, diags := types.ListValueFrom(ctx, typeOptionsObjectType(), g)
	if diags.HasError() {
		return got
	}
	return out
}

func reconcilePathInfos(ctx context.Context, planned, got types.List) types.List {
	if planned.IsNull() || planned.IsUnknown() || got.IsNull() || got.IsUnknown() {
		return got
	}
	var p, g []PathInfoModel
	if planned.ElementsAs(ctx, &p, false).HasError() || got.ElementsAs(ctx, &g, false).HasError() || len(p) != len(g) {
		return got
	}
	for i := range g {
		nullIfPlannedNullString(p[i].NetworkPath, &g[i].NetworkPath)
	}
	out, diags := types.ListValueFrom(ctx, pathInfoObjectType(), g)
	if diags.HasError() {
		return got
	}
	return out
}
