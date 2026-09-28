// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
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
	PendingRestart  types.Bool   `tfsdk:"pending_restart"`
}

func (d *SystemInfoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_info"
}

func (d *SystemInfoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computedString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Description:         desc,
			MarkdownDescription: desc,
			Computed:            true,
		}
	}
	const pendingRestart = "Whether the Jellyfin server has a pending restart (e.g. after a plugin install). True until the server is restarted."
	resp.Schema = schema.Schema{
		Description:         "Retrieves system information from the Jellyfin server.",
		MarkdownDescription: "Retrieves system information from the Jellyfin server.",
		Attributes: map[string]schema.Attribute{
			"id":               computedString("The unique server identifier."),
			"server_name":      computedString("The server name."),
			"version":          computedString("The Jellyfin server version."),
			"operating_system": computedString("The server operating system."),
			"local_address":    computedString("The local network address of the server."),
			"pending_restart": schema.BoolAttribute{
				Description:         pendingRestart,
				MarkdownDescription: pendingRestart,
				Computed:            true,
			},
		},
	}
}

func (d *SystemInfoDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, "Data Source", &resp.Diagnostics)
}

func (d *SystemInfoDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	info, err := d.client.GetSystemInfo(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get system info", err.Error())
		return
	}

	data := SystemInfoDataSourceModel{
		ID:              types.StringValue(info.ID),
		ServerName:      types.StringValue(info.ServerName),
		Version:         types.StringValue(info.Version),
		OperatingSystem: types.StringValue(info.OperatingSystem),
		LocalAddress:    types.StringValue(info.LocalAddress),
		PendingRestart:  types.BoolValue(info.HasPendingRestart),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
