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

var _ datasource.DataSource = &PluginRepositoriesDataSource{}

// NewPluginRepositoriesDataSource creates a new plugin repositories list data source.
func NewPluginRepositoriesDataSource() datasource.DataSource {
	return &PluginRepositoriesDataSource{}
}

// PluginRepositoriesDataSource defines the data source implementation.
type PluginRepositoriesDataSource struct {
	client *client.Client
}

// PluginRepositoriesDataSourceRepoModel describes a single repository element in the list.
type PluginRepositoriesDataSourceRepoModel struct {
	Name    types.String `tfsdk:"name"`
	URL     types.String `tfsdk:"url"`
	Enabled types.Bool   `tfsdk:"enabled"`
}

// PluginRepositoriesDataSourceModel describes the data source data model.
type PluginRepositoriesDataSourceModel struct {
	Repositories []PluginRepositoriesDataSourceRepoModel `tfsdk:"repositories"`
}

func (d *PluginRepositoriesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin_repositories"
}

func (d *PluginRepositoriesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all configured Jellyfin plugin repositories.",
		Attributes: map[string]schema.Attribute{
			"repositories": schema.ListNestedAttribute{
				MarkdownDescription: "The list of plugin repositories.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "The repository name.",
							Computed:            true,
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
				},
			},
		},
	}
}

func (d *PluginRepositoriesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PluginRepositoriesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	repos, err := d.client.GetPluginRepositories()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get plugin repositories", err.Error())
		return
	}

	data := PluginRepositoriesDataSourceModel{
		Repositories: make([]PluginRepositoriesDataSourceRepoModel, 0, len(repos)),
	}
	for _, r := range repos {
		data.Repositories = append(data.Repositories, PluginRepositoriesDataSourceRepoModel{
			Name:    types.StringValue(r.Name),
			URL:     types.StringValue(r.Url),
			Enabled: types.BoolValue(r.Enabled),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
