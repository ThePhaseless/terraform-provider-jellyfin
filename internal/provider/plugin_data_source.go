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

var _ datasource.DataSource = &PluginDataSource{}

// NewPluginDataSource creates a new plugin data source.
func NewPluginDataSource() datasource.DataSource {
	return &PluginDataSource{}
}

// PluginDataSource defines the data source implementation.
type PluginDataSource struct {
	client *client.Client
}

// PluginDataSourceModel describes the data source data model.
type PluginDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Version     types.String `tfsdk:"version"`
	Status      types.String `tfsdk:"status"`
	Description types.String `tfsdk:"description"`
}

func (d *PluginDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin"
}

func (d *PluginDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an installed Jellyfin plugin by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "The plugin name to look up.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The plugin ID.",
				Computed:            true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "The installed plugin version.",
				Computed:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "The plugin status (e.g., `Active`, `Disabled`).",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The plugin description.",
				Computed:            true,
			},
		},
	}
}

func (d *PluginDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PluginDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PluginDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plugins, err := d.client.GetInstalledPlugins()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get installed plugins", err.Error())
		return
	}

	name := data.Name.ValueString()
	var match *client.InstalledPlugin
	for i := range plugins {
		if plugins[i].Name == name {
			match = &plugins[i]
			break
		}
	}

	if match == nil {
		resp.Diagnostics.AddError("Plugin not found", fmt.Sprintf("No installed plugin with name %q was found.", name))
		return
	}

	data.ID = types.StringValue(match.Id)
	data.Name = types.StringValue(match.Name)
	data.Version = types.StringValue(match.Version)
	data.Status = types.StringValue(match.Status)
	data.Description = types.StringValue(match.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
