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

var _ datasource.DataSource = &APIKeyDataSource{}

// NewAPIKeyDataSource creates a new API key data source.
func NewAPIKeyDataSource() datasource.DataSource {
	return &APIKeyDataSource{}
}

// APIKeyDataSource defines the data source implementation.
type APIKeyDataSource struct {
	client *client.Client
}

// APIKeyDataSourceModel describes the data source data model.
type APIKeyDataSourceModel struct {
	AppName     types.String `tfsdk:"app_name"`
	AccessToken types.String `tfsdk:"access_token"`
	DateCreated types.String `tfsdk:"date_created"`
}

func (d *APIKeyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (d *APIKeyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Jellyfin API key by app name. The lookup fails if multiple keys share the same name.",
		Attributes: map[string]schema.Attribute{
			"app_name": schema.StringAttribute{
				MarkdownDescription: "The application name of the API key.",
				Required:            true,
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
	}
}

func (d *APIKeyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *APIKeyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data APIKeyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key, err := d.client.GetAPIKeyByAppName(data.AppName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to get API key", err.Error())
		return
	}

	data.AppName = types.StringValue(key.AppName)
	data.AccessToken = types.StringValue(key.AccessToken)
	data.DateCreated = types.StringValue(key.DateCreated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
