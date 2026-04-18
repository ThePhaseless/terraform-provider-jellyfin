// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &LibraryDataSource{}

// NewLibraryDataSource creates a new library data source.
func NewLibraryDataSource() datasource.DataSource {
	return &LibraryDataSource{}
}

// LibraryDataSource defines the data source implementation.
type LibraryDataSource struct {
	client *client.Client
}

// LibraryDataSourceModel describes the data source data model.
type LibraryDataSourceModel struct {
	Name           types.String `tfsdk:"name"`
	CollectionType types.String `tfsdk:"collection_type"`
	Paths          types.List   `tfsdk:"paths"`
	ItemID         types.String `tfsdk:"item_id"`
}

func (d *LibraryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_library"
}

func (d *LibraryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Jellyfin media library (virtual folder) by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "The library name.",
				Required:            true,
			},
			"collection_type": schema.StringAttribute{
				MarkdownDescription: "The collection type (e.g., `movies`, `tvshows`, `music`).",
				Computed:            true,
			},
			"paths": schema.ListAttribute{
				MarkdownDescription: "List of file system paths for this library.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"item_id": schema.StringAttribute{
				MarkdownDescription: "The internal item ID assigned by Jellyfin.",
				Computed:            true,
			},
		},
	}
}

func (d *LibraryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}

	d.client = c
}

func (d *LibraryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data LibraryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	folders, err := d.client.GetVirtualFolders()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get virtual folders", err.Error())
		return
	}

	name := data.Name.ValueString()
	var match *client.VirtualFolder
	for i := range folders {
		if folders[i].Name == name {
			match = &folders[i]
			break
		}
	}

	if match == nil {
		resp.Diagnostics.AddError("Library not found", fmt.Sprintf("No library with name %q was found.", name))
		return
	}

	data.Name = types.StringValue(match.Name)
	data.CollectionType = types.StringValue(match.CollectionType)
	data.ItemID = types.StringValue(match.ItemId)

	pathValues, diags := types.ListValueFrom(ctx, types.StringType, match.Locations)
	resp.Diagnostics.Append(diags...)
	data.Paths = pathValues

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
