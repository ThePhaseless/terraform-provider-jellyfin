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

var _ datasource.DataSource = &AvailablePackagesDataSource{}

// NewAvailablePackagesDataSource creates a new available packages list data source.
func NewAvailablePackagesDataSource() datasource.DataSource {
	return &AvailablePackagesDataSource{}
}

// AvailablePackagesDataSource defines the data source implementation.
type AvailablePackagesDataSource struct {
	client *client.Client
}

// AvailablePackagesDataSourceVersionModel describes a single available package version.
type AvailablePackagesDataSourceVersionModel struct {
	Version        types.String `tfsdk:"version"`
	RepositoryURL  types.String `tfsdk:"repository_url"`
	RepositoryName types.String `tfsdk:"repository_name"`
	TargetAbi      types.String `tfsdk:"target_abi"`
}

// AvailablePackagesDataSourcePackageModel describes a single available package.
type AvailablePackagesDataSourcePackageModel struct {
	Name        types.String                              `tfsdk:"name"`
	Description types.String                              `tfsdk:"description"`
	Versions    []AvailablePackagesDataSourceVersionModel `tfsdk:"versions"`
}

// AvailablePackagesDataSourceModel describes the data source data model.
type AvailablePackagesDataSourceModel struct {
	Packages []AvailablePackagesDataSourcePackageModel `tfsdk:"packages"`
}

func (d *AvailablePackagesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_available_packages"
}

func (d *AvailablePackagesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all packages available from the configured Jellyfin plugin repositories.",
		Attributes: map[string]schema.Attribute{
			"packages": schema.ListNestedAttribute{
				MarkdownDescription: "The list of available packages.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "The package name.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "The package description.",
							Computed:            true,
						},
						"versions": schema.ListNestedAttribute{
							MarkdownDescription: "The list of available versions for the package.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"version": schema.StringAttribute{
										MarkdownDescription: "The version string.",
										Computed:            true,
									},
									"repository_url": schema.StringAttribute{
										MarkdownDescription: "The repository URL providing this version.",
										Computed:            true,
									},
									"repository_name": schema.StringAttribute{
										MarkdownDescription: "The repository name providing this version.",
										Computed:            true,
									},
									"target_abi": schema.StringAttribute{
										MarkdownDescription: "The Jellyfin server ABI this version targets.",
										Computed:            true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *AvailablePackagesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AvailablePackagesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	packages, err := d.client.GetAvailablePackages()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get available packages", err.Error())
		return
	}

	data := AvailablePackagesDataSourceModel{
		Packages: make([]AvailablePackagesDataSourcePackageModel, 0, len(packages)),
	}
	for _, p := range packages {
		versions := make([]AvailablePackagesDataSourceVersionModel, 0, len(p.Versions))
		for _, v := range p.Versions {
			versions = append(versions, AvailablePackagesDataSourceVersionModel{
				Version:        types.StringValue(v.Version),
				RepositoryURL:  types.StringValue(v.RepositoryUrl),
				RepositoryName: types.StringValue(v.RepositoryName),
				TargetAbi:      types.StringValue(v.TargetAbi),
			})
		}
		data.Packages = append(data.Packages, AvailablePackagesDataSourcePackageModel{
			Name:        types.StringValue(p.Name),
			Description: types.StringValue(p.Description),
			Versions:    versions,
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
