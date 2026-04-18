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

var _ datasource.DataSource = &LibrariesDataSource{}

// NewLibrariesDataSource creates a new libraries list data source.
func NewLibrariesDataSource() datasource.DataSource {
	return &LibrariesDataSource{}
}

// LibrariesDataSource defines the data source implementation.
type LibrariesDataSource struct {
	client *client.Client
}

// LibrariesDataSourceLibraryModel describes a single library element in the list.
type LibrariesDataSourceLibraryModel struct {
	Name           types.String `tfsdk:"name"`
	CollectionType types.String `tfsdk:"collection_type"`
	ItemID         types.String `tfsdk:"item_id"`
}

// LibrariesDataSourceModel describes the data source data model.
type LibrariesDataSourceModel struct {
	Libraries []LibrariesDataSourceLibraryModel `tfsdk:"libraries"`
}

func (d *LibrariesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_libraries"
}

func (d *LibrariesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all Jellyfin media libraries (virtual folders) on the server.",
		Attributes: map[string]schema.Attribute{
			"libraries": schema.ListNestedAttribute{
				MarkdownDescription: "The list of libraries.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "The library name.",
							Computed:            true,
						},
						"collection_type": schema.StringAttribute{
							MarkdownDescription: "The collection type.",
							Computed:            true,
						},
						"item_id": schema.StringAttribute{
							MarkdownDescription: "The internal item ID assigned by Jellyfin.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *LibrariesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *LibrariesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	folders, err := d.client.GetVirtualFolders()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get virtual folders", err.Error())
		return
	}

	data := LibrariesDataSourceModel{
		Libraries: make([]LibrariesDataSourceLibraryModel, 0, len(folders)),
	}
	for _, f := range folders {
		data.Libraries = append(data.Libraries, LibrariesDataSourceLibraryModel{
			Name:           types.StringValue(f.Name),
			CollectionType: types.StringValue(f.CollectionType),
			ItemID:         types.StringValue(f.ItemId),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
