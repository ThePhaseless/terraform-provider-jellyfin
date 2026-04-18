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

var _ datasource.DataSource = &PluginsDataSource{}

// NewPluginsDataSource creates a new plugins list data source.
func NewPluginsDataSource() datasource.DataSource {
	return &PluginsDataSource{}
}

// PluginsDataSource defines the data source implementation.
type PluginsDataSource struct {
	client *client.Client
}

// PluginsDataSourcePluginModel describes a single plugin element in the list.
type PluginsDataSourcePluginModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Version types.String `tfsdk:"version"`
	Status  types.String `tfsdk:"status"`
}

// PluginsDataSourceModel describes the data source data model.
type PluginsDataSourceModel struct {
	Plugins []PluginsDataSourcePluginModel `tfsdk:"plugins"`
}

func (d *PluginsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugins"
}

func (d *PluginsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all installed Jellyfin plugins.",
		Attributes: map[string]schema.Attribute{
			"plugins": schema.ListNestedAttribute{
				MarkdownDescription: "The list of installed plugins.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The plugin ID.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The plugin name.",
							Computed:            true,
						},
						"version": schema.StringAttribute{
							MarkdownDescription: "The installed plugin version.",
							Computed:            true,
						},
						"status": schema.StringAttribute{
							MarkdownDescription: "The plugin status.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *PluginsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PluginsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	plugins, err := d.client.GetInstalledPlugins()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get installed plugins", err.Error())
		return
	}

	data := PluginsDataSourceModel{
		Plugins: make([]PluginsDataSourcePluginModel, 0, len(plugins)),
	}
	for _, p := range plugins {
		data.Plugins = append(data.Plugins, PluginsDataSourcePluginModel{
			ID:      types.StringValue(p.Id),
			Name:    types.StringValue(p.Name),
			Version: types.StringValue(p.Version),
			Status:  types.StringValue(p.Status),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
