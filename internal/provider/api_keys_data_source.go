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

var _ datasource.DataSource = &APIKeysDataSource{}

// NewAPIKeysDataSource creates a new API keys list data source.
func NewAPIKeysDataSource() datasource.DataSource {
	return &APIKeysDataSource{}
}

// APIKeysDataSource defines the data source implementation.
type APIKeysDataSource struct {
	client *client.Client
}

// APIKeysDataSourceKeyModel describes a single API key element in the list.
type APIKeysDataSourceKeyModel struct {
	AppName     types.String `tfsdk:"app_name"`
	AccessToken types.String `tfsdk:"access_token"`
	DateCreated types.String `tfsdk:"date_created"`
}

// APIKeysDataSourceModel describes the data source data model.
type APIKeysDataSourceModel struct {
	Keys []APIKeysDataSourceKeyModel `tfsdk:"keys"`
}

func (d *APIKeysDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_keys"
}

func (d *APIKeysDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all Jellyfin API keys.",
		Attributes: map[string]schema.Attribute{
			"keys": schema.ListNestedAttribute{
				MarkdownDescription: "The list of API keys.",
				Computed:            true,
				Sensitive:           true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"app_name": schema.StringAttribute{
							MarkdownDescription: "The application name.",
							Computed:            true,
						},
						"access_token": schema.StringAttribute{
							MarkdownDescription: "The API key access token.",
							Computed:            true,
							Sensitive:           true,
						},
						"date_created": schema.StringAttribute{
							MarkdownDescription: "The date the API key was created.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *APIKeysDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *APIKeysDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	keys, err := d.client.GetAPIKeys()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get API keys", err.Error())
		return
	}

	data := APIKeysDataSourceModel{
		Keys: make([]APIKeysDataSourceKeyModel, 0, len(keys)),
	}
	for _, k := range keys {
		data.Keys = append(data.Keys, APIKeysDataSourceKeyModel{
			AppName:     types.StringValue(k.AppName),
			AccessToken: types.StringValue(k.AccessToken),
			DateCreated: types.StringValue(k.DateCreated),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
