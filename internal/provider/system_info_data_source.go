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

var _ datasource.DataSource = &SystemInfoDataSource{}

// NewSystemInfoDataSource creates a new system info data source.
func NewSystemInfoDataSource() datasource.DataSource {
	return &SystemInfoDataSource{}
}

// SystemInfoDataSource defines the data source implementation.
type SystemInfoDataSource struct {
	client *client.Client
}

// SystemInfoDataSourceModel describes the data source data model.
type SystemInfoDataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	ServerName      types.String `tfsdk:"server_name"`
	Version         types.String `tfsdk:"version"`
	OperatingSystem types.String `tfsdk:"operating_system"`
	LocalAddress    types.String `tfsdk:"local_address"`
}

func (d *SystemInfoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_info"
}

func (d *SystemInfoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves system information from the Jellyfin server.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique server identifier.",
				Computed:            true,
			},
			"server_name": schema.StringAttribute{
				MarkdownDescription: "The server name.",
				Computed:            true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "The Jellyfin server version.",
				Computed:            true,
			},
			"operating_system": schema.StringAttribute{
				MarkdownDescription: "The server operating system.",
				Computed:            true,
			},
			"local_address": schema.StringAttribute{
				MarkdownDescription: "The local network address of the server.",
				Computed:            true,
			},
		},
	}
}

func (d *SystemInfoDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SystemInfoDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	info, err := d.client.GetSystemInfo()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get system info", err.Error())
		return
	}

	data := SystemInfoDataSourceModel{
		ID:              types.StringValue(info.Id),
		ServerName:      types.StringValue(info.ServerName),
		Version:         types.StringValue(info.Version),
		OperatingSystem: types.StringValue(info.OperatingSystem),
		LocalAddress:    types.StringValue(info.LocalAddress),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
