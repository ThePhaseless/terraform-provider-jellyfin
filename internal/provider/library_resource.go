// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
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
	LibraryOptions jsontypes.Normalized `tfsdk:"library_options_json"`
	ItemID         types.String         `tfsdk:"item_id"`
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
			"library_options_json": schema.StringAttribute{
				Description:         "Library options as a JSON string. Allows full customization of library settings.",
				MarkdownDescription: "Library options as a JSON string. Allows full customization of library settings.",
				Optional:            true,
				Computed:            true,
				CustomType:          jsontypes.NormalizedType{},
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

	var libraryOpts *client.LibraryOptions
	if !data.LibraryOptions.IsNull() && !data.LibraryOptions.IsUnknown() && data.LibraryOptions.ValueString() != "" {
		libraryOpts = &client.LibraryOptions{RawJSON: data.LibraryOptions.ValueString()}
	}

	if err := r.client.AddVirtualFolder(ctx, data.Name.ValueString(), data.CollectionType.ValueString(), paths, libraryOpts); err != nil {
		resp.Diagnostics.AddError("Failed to create library", err.Error())
		return
	}

	// Read back to get the ItemID.
	folder, err := r.findFolder(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read library after creation", err.Error())
		return
	}

	data.ItemID = types.StringValue(folder.ItemID)
	data.ID = types.StringValue(folder.Name)

	opts := folder.GetLibraryOptions()
	if opts.RawJSON != "" && opts.RawJSON != "{}" {
		data.LibraryOptions = jsontypes.NewNormalizedValue(opts.RawJSON)
	} else {
		data.LibraryOptions = jsontypes.NewNormalizedNull()
	}

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

	opts := folder.GetLibraryOptions()
	if opts.RawJSON != "" && opts.RawJSON != "{}" {
		data.LibraryOptions = jsontypes.NewNormalizedValue(opts.RawJSON)
	} else {
		data.LibraryOptions = jsontypes.NewNormalizedNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LibraryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data LibraryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var libraryOpts *client.LibraryOptions
	if !data.LibraryOptions.IsNull() && !data.LibraryOptions.IsUnknown() && data.LibraryOptions.ValueString() != "" {
		libraryOpts = &client.LibraryOptions{RawJSON: data.LibraryOptions.ValueString()}
	}

	if err := r.client.UpdateVirtualFolder(ctx, data.Name.ValueString(), libraryOpts); err != nil {
		resp.Diagnostics.AddError("Failed to update library", err.Error())
		return
	}

	// Read back to refresh state.
	folder, err := r.findFolder(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read library after update", err.Error())
		return
	}

	data.ItemID = types.StringValue(folder.ItemID)
	data.ID = types.StringValue(folder.Name)

	opts := folder.GetLibraryOptions()
	if opts.RawJSON != "" && opts.RawJSON != "{}" {
		data.LibraryOptions = jsontypes.NewNormalizedValue(opts.RawJSON)
	} else {
		data.LibraryOptions = jsontypes.NewNormalizedNull()
	}

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
