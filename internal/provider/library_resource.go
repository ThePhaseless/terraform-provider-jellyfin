// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

var (
	_ resource.Resource                = &LibraryResource{}
	_ resource.ResourceWithImportState = &LibraryResource{}
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
	Type              types.String `tfsdk:"type"`
	MetadataFetchers  types.List   `tfsdk:"metadata_fetchers"`
	ImageFetchers     types.List   `tfsdk:"image_fetchers"`
	ImageOptions      types.List   `tfsdk:"image_options"`
	ImageFetcherOrder types.List   `tfsdk:"image_fetcher_order"`
}

// ImageOptionsModel describes one ImageOptions entry.
type ImageOptionsModel struct {
	Type  types.String `tfsdk:"type"`
	Limit types.Int64  `tfsdk:"limit"`
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
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"collection_type": schema.StringAttribute{
				Description:         "The collection type (e.g., `movies`, `tvshows`, `music`, `books`, `homevideos`, `boxsets`, `mixed`).",
				MarkdownDescription: "The collection type (e.g., `movies`, `tvshows`, `music`, `books`, `homevideos`, `boxsets`, `mixed`).",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("movies", "tvshows", "music", "books", "homevideos", "boxsets", "mixed"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"paths": schema.ListAttribute{
				Description:         "List of file system paths for this library.",
				MarkdownDescription: "List of file system paths for this library.",
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

	return map[string]schema.Attribute{
		"enable_photos":                                 optionalBool("Whether photos are enabled."),
		"enable_realtime_monitor":                       optionalBool("Whether realtime monitoring is enabled."),
		"enable_emby_photos":                            optionalBool("Whether Emby photos are enabled."),
		"enable_photo_subtitle":                         optionalBool("Whether photo subtitles are enabled."),
		"extract_chapters_during_library_scan":          optionalBool("Whether chapters are extracted during library scan."),
		"enable_chapter_image_extraction":               optionalBool("Whether chapter image extraction is enabled."),
		"chapter_image_interval_seconds":                optionalInt("Chapter image interval in seconds."),
		"extract_media_information_during_library_scan": optionalBool("Whether media information is extracted during library scan."),
		"download_images_in_advance":                    optionalBool("Whether images are downloaded in advance."),
		"cache_images_in_library":                       optionalBool("Whether images are cached in the library."),
		"enable_media_conversion":                       optionalBool("Whether media conversion is enabled."),
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
		"disabled_metadata_savers":         optionalStringList("Disabled metadata savers."),
		"local_metadata_reader_order":      optionalStringList("Local metadata reader order."),
		"disabled_metadata_fetchers":       optionalStringList("Disabled metadata fetchers."),
		"metadata_fetcher_order":           optionalStringList("Metadata fetcher order."),
		"disabled_image_fetchers":          optionalStringList("Disabled image fetchers."),
		"image_fetcher_order":              optionalStringList("Image fetcher order."),
		"disabled_subtitle_fetchers":       optionalStringList("Disabled subtitle fetchers."),
		"subtitle_fetcher_order":           optionalStringList("Subtitle fetcher order."),
		"save_local_metadata":              optionalBool("Whether local metadata is saved."),
		"save_local_thumbnail_sets":        optionalBool("Whether local thumbnail sets are saved."),
		"import_missing_episodes":          optionalBool("Whether missing episodes are imported."),
		"enable_automatic_series_grouping": optionalBool("Whether automatic series grouping is enabled."),
		"season_zero_display_name":         optionalString("Season zero display name."),
		"metadata_refresh_mode":            optionalString("Metadata refresh mode."),
		"disabled":                         optionalBool("Whether the library is disabled."),
		"type_options": schema.ListNestedAttribute{
			NestedObject: schema.NestedAttributeObject{
				Attributes: typeOptionsAttributes(),
			},
			Description:         "Type-specific options.",
			MarkdownDescription: "Type-specific options.",
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
	return map[string]schema.Attribute{
		"path":         optionalString("Local path."),
		"network_path": optionalString("Network path."),
		"username":     optionalString("Username."),
		"password": schema.StringAttribute{
			Description:         "Password.",
			MarkdownDescription: "Password.",
			Optional:            true,
			Computed:            true,
			Sensitive:           true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
	}
}

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
		"type":              optionalString("Item type."),
		"metadata_fetchers": optionalStringList("Metadata fetchers for this type."),
		"image_fetchers":    optionalStringList("Image fetchers for this type."),
		"image_options": schema.ListNestedAttribute{
			NestedObject: schema.NestedAttributeObject{
				Attributes: imageOptionsAttributes(),
			},
			Description:         "Image options for this type.",
			MarkdownDescription: "Image options for this type.",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		},
		"image_fetcher_order": optionalStringList("Image fetcher order for this type."),
	}
}

func imageOptionsAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"type": schema.StringAttribute{
			Description:         "Image type.",
			MarkdownDescription: "Image type.",
			Optional:            true,
			Computed:            true,
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
		"type":                types.StringType,
		"metadata_fetchers":   types.ListType{ElemType: types.StringType},
		"image_fetchers":      types.ListType{ElemType: types.StringType},
		"image_options":       types.ListType{ElemType: imageOptionsObjectType()},
		"image_fetcher_order": types.ListType{ElemType: types.StringType},
	}}
}

func imageOptionsObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"type":  types.StringType,
		"limit": types.Int64Type,
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
	var data LibraryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var paths []string
	resp.Diagnostics.Append(data.Paths.ElementsAs(ctx, &paths, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.AddVirtualFolder(ctx, data.Name.ValueString(), data.CollectionType.ValueString(), paths, nil); err != nil {
		resp.Diagnostics.AddError("Failed to create library", err.Error())
		return
	}

	folder, err := r.findFolder(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read library after creation", err.Error())
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

	if err := r.client.UpdateVirtualFolder(ctx, data.Name.ValueString(), &client.LibraryOptions{RawJSON: string(payload)}); err != nil {
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
	data.CollectionType = types.StringValue(updated.CollectionType)
	pathValues, diags := types.ListValueFrom(ctx, types.StringType, updated.Locations)
	resp.Diagnostics.Append(diags...)
	data.Paths = pathValues
	data.LibraryOptions = flattenLibraryOptions(ctx, updated.GetLibraryOptions().RawJSON, &resp.Diagnostics)

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

	data.CollectionType = types.StringValue(folder.CollectionType)
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

	if err := r.client.UpdateVirtualFolder(ctx, state.Name.ValueString(), &client.LibraryOptions{RawJSON: string(payload)}); err != nil {
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
	data.CollectionType = types.StringValue(updated.CollectionType)
	pathValues, diags := types.ListValueFrom(ctx, types.StringType, updated.Locations)
	resp.Diagnostics.Append(diags...)
	data.Paths = pathValues
	data.LibraryOptions = flattenLibraryOptions(ctx, updated.GetLibraryOptions().RawJSON, &resp.Diagnostics)

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

func (r *LibraryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func overlayLibraryOptions(ctx context.Context, m map[string]json.RawMessage, opts *LibraryOptionsModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if opts == nil {
		return diags
	}

	putJSONBool(m, "EnablePhotos", opts.EnablePhotos)
	putJSONBool(m, "EnableRealtimeMonitor", opts.EnableRealtimeMonitor)
	putJSONBool(m, "EnableEmbiPhotos", opts.EnableEmbiPhotos)
	putJSONBool(m, "EnablePhotoSubtitle", opts.EnablePhotoSubtitle)
	putJSONBool(m, "ExtractChaptersDuringLibraryScan", opts.ExtractChaptersDuringLibraryScan)
	putJSONBool(m, "EnableChapterImageExtraction", opts.EnableChapterImageExtraction)
	putJSONInt64(m, "ChapterImageIntervalSeconds", opts.ChapterImageIntervalSeconds)
	putJSONBool(m, "ExtractMediaInformationDuringLibraryScan", opts.ExtractMediaInformationDuringLibraryScan)
	putJSONBool(m, "DownloadImagesInAdvance", opts.DownloadImagesInAdvance)
	putJSONBool(m, "CacheImagesInLibrary", opts.CacheImagesInLibrary)
	putJSONBool(m, "EnableMediaConversion", opts.EnableMediaConversion)
	if d := overlayPathInfos(ctx, m, opts.PathInfos); d.HasError() {
		diags.Append(d...)
		return diags
	}
	putJSONString(m, "PreferredMetadataLanguage", opts.PreferredMetadataLanguage)
	putJSONString(m, "MetadataCountryCode", opts.MetadataCountryCode)
	if d := putJSONStringList(ctx, m, "DisabledMetadataSavers", opts.DisabledMetadataSavers); d.HasError() {
		diags.Append(d...)
		return diags
	}
	if d := putJSONStringList(ctx, m, "LocalMetadataReaderOrder", opts.LocalMetadataReaderOrder); d.HasError() {
		diags.Append(d...)
		return diags
	}
	if d := putJSONStringList(ctx, m, "DisabledMetadataFetchers", opts.DisabledMetadataFetchers); d.HasError() {
		diags.Append(d...)
		return diags
	}
	if d := putJSONStringList(ctx, m, "MetadataFetcherOrder", opts.MetadataFetcherOrder); d.HasError() {
		diags.Append(d...)
		return diags
	}
	if d := putJSONStringList(ctx, m, "DisabledImageFetchers", opts.DisabledImageFetchers); d.HasError() {
		diags.Append(d...)
		return diags
	}
	if d := putJSONStringList(ctx, m, "ImageFetcherOrder", opts.ImageFetcherOrder); d.HasError() {
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
	putJSONBool(m, "SaveLocalThumbnailSets", opts.SaveLocalThumbnailSets)
	putJSONBool(m, "ImportMissingEpisodes", opts.ImportMissingEpisodes)
	putJSONBool(m, "EnableAutomaticSeriesGrouping", opts.EnableAutomaticSeriesGrouping)
	putJSONString(m, "SeasonZeroDisplayName", opts.SeasonZeroDisplayName)
	putJSONString(m, "MetadataRefreshMode", opts.MetadataRefreshMode)
	putJSONBool(m, "Disabled", opts.Disabled)
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
		putJSONString(entry, "Username", e.Username)
		putJSONString(entry, "Password", e.Password)
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
	rawEntries := make([]map[string]json.RawMessage, len(entries))
	for i, e := range entries {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "Type", e.Type)
		if d := putJSONStringList(ctx, entry, "MetadataFetchers", e.MetadataFetchers); d.HasError() {
			diags.Append(d...)
			return diags
		}
		if d := putJSONStringList(ctx, entry, "ImageFetchers", e.ImageFetchers); d.HasError() {
			diags.Append(d...)
			return diags
		}
		if d := overlayImageOptions(ctx, entry, e.ImageOptions); d.HasError() {
			diags.Append(d...)
			return diags
		}
		if d := putJSONStringList(ctx, entry, "ImageFetcherOrder", e.ImageFetcherOrder); d.HasError() {
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
	rawEntries := make([]map[string]json.RawMessage, len(entries))
	for i, e := range entries {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "Type", e.Type)
		putJSONInt64(entry, "Limit", e.Limit)
		rawEntries[i] = entry
	}
	b, err := json.Marshal(rawEntries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal image options", err.Error()))
	}
	m["ImageOptions"] = b
	return diags
}

func flattenLibraryOptions(ctx context.Context, raw string, diags *diag.Diagnostics) *LibraryOptionsModel {
	m, err := parseJSONObject(raw)
	if err != nil {
		diags.AddError("Failed to parse library options", err.Error())
		return nil
	}

	opts := &LibraryOptionsModel{}
	opts.EnablePhotos = getJSONBool(m, "EnablePhotos")
	opts.EnableRealtimeMonitor = getJSONBool(m, "EnableRealtimeMonitor")
	opts.EnableEmbiPhotos = getJSONBool(m, "EnableEmbiPhotos")
	opts.EnablePhotoSubtitle = getJSONBool(m, "EnablePhotoSubtitle")
	opts.ExtractChaptersDuringLibraryScan = getJSONBool(m, "ExtractChaptersDuringLibraryScan")
	opts.EnableChapterImageExtraction = getJSONBool(m, "EnableChapterImageExtraction")
	opts.ChapterImageIntervalSeconds = getJSONInt64(m, "ChapterImageIntervalSeconds")
	opts.ExtractMediaInformationDuringLibraryScan = getJSONBool(m, "ExtractMediaInformationDuringLibraryScan")
	opts.DownloadImagesInAdvance = getJSONBool(m, "DownloadImagesInAdvance")
	opts.CacheImagesInLibrary = getJSONBool(m, "CacheImagesInLibrary")
	opts.EnableMediaConversion = getJSONBool(m, "EnableMediaConversion")
	opts.PathInfos = flattenPathInfos(ctx, m, diags)
	opts.PreferredMetadataLanguage = getJSONString(m, "PreferredMetadataLanguage")
	opts.MetadataCountryCode = getJSONString(m, "MetadataCountryCode")
	opts.DisabledMetadataSavers, _ = getJSONStringList(ctx, m, "DisabledMetadataSavers")
	opts.LocalMetadataReaderOrder, _ = getJSONStringList(ctx, m, "LocalMetadataReaderOrder")
	opts.DisabledMetadataFetchers, _ = getJSONStringList(ctx, m, "DisabledMetadataFetchers")
	opts.MetadataFetcherOrder, _ = getJSONStringList(ctx, m, "MetadataFetcherOrder")
	opts.DisabledImageFetchers, _ = getJSONStringList(ctx, m, "DisabledImageFetchers")
	opts.ImageFetcherOrder, _ = getJSONStringList(ctx, m, "ImageFetcherOrder")
	opts.DisabledSubtitleFetchers, _ = getJSONStringList(ctx, m, "DisabledSubtitleFetchers")
	opts.SubtitleFetcherOrder, _ = getJSONStringList(ctx, m, "SubtitleFetcherOrder")
	opts.SaveLocalMetadata = getJSONBool(m, "SaveLocalMetadata")
	opts.SaveLocalThumbnailSets = getJSONBool(m, "SaveLocalThumbnailSets")
	opts.ImportMissingEpisodes = getJSONBool(m, "ImportMissingEpisodes")
	opts.EnableAutomaticSeriesGrouping = getJSONBool(m, "EnableAutomaticSeriesGrouping")
	opts.SeasonZeroDisplayName = getJSONString(m, "SeasonZeroDisplayName")
	opts.MetadataRefreshMode = getJSONString(m, "MetadataRefreshMode")
	opts.Disabled = getJSONBool(m, "Disabled")
	opts.TypeOptions = flattenTypeOptions(ctx, m, diags)
	return opts
}

func flattenPathInfos(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
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
			"username":     getJSONString(e, "Username"),
			"password":     getJSONString(e, "Password"),
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
			"type":                getJSONString(e, "Type"),
			"metadata_fetchers":   types.ListNull(types.StringType),
			"image_fetchers":      types.ListNull(types.StringType),
			"image_options":       flattenImageOptions(ctx, e, diags),
			"image_fetcher_order": types.ListNull(types.StringType),
		}
		if v, d := getJSONStringList(ctx, e, "MetadataFetchers"); !d.HasError() {
			attrs["metadata_fetchers"] = v
		}
		if v, d := getJSONStringList(ctx, e, "ImageFetchers"); !d.HasError() {
			attrs["image_fetchers"] = v
		}
		if v, d := getJSONStringList(ctx, e, "ImageFetcherOrder"); !d.HasError() {
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

func flattenImageOptions(ctx context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
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
			"type":  getJSONString(e, "Type"),
			"limit": getJSONInt64(e, "Limit"),
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
