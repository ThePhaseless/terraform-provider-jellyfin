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

var _ datasource.DataSource = &PluginRepositoryDataSource{}

// NewPluginRepositoryDataSource creates a new plugin repository data source.
func NewPluginRepositoryDataSource() datasource.DataSource {
	return &PluginRepositoryDataSource{}
}

// PluginRepositoryDataSource defines the data source implementation.
type PluginRepositoryDataSource struct {
	client *client.Client
}

// PluginRepositoryDataSourceModel describes the data source data model.
type PluginRepositoryDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	URL     types.String `tfsdk:"url"`
	Enabled types.Bool   `tfsdk:"enabled"`
}

func (d *PluginRepositoryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin_repository"
}

func (d *PluginRepositoryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Jellyfin plugin repository by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "The repository name to look up.",
				Required:            true,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "The repository URL.",
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the repository is enabled.",
				Computed:            true,
			},
		},
	}
}

func (d *PluginRepositoryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PluginRepositoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PluginRepositoryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repos, err := d.client.GetPluginRepositories()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get plugin repositories", err.Error())
		return
	}

	name := data.Name.ValueString()
	var match *client.PluginRepository
	for i := range repos {
		if repos[i].Name == name {
			match = &repos[i]
			break
		}
	}

	if match == nil {
		resp.Diagnostics.AddError("Plugin repository not found", fmt.Sprintf("No plugin repository with name %q was found.", name))
		return
	}

	data.Name = types.StringValue(match.Name)
	data.URL = types.StringValue(match.Url)
	data.Enabled = types.BoolValue(match.Enabled)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
