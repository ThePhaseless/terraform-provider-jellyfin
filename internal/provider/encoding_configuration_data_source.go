// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

var _ datasource.DataSource = &EncodingConfigurationDataSource{}

// NewEncodingConfigurationDataSource creates a new encoding configuration data source.
func NewEncodingConfigurationDataSource() datasource.DataSource {
	return &EncodingConfigurationDataSource{}
}

// EncodingConfigurationDataSource defines the data source implementation.
type EncodingConfigurationDataSource struct {
	client *client.Client
}

// EncodingConfigurationDataSourceModel describes the data source data model.
type EncodingConfigurationDataSourceModel struct {
	ConfigJSON jsontypes.Normalized `tfsdk:"config_json"`
}

func (d *EncodingConfigurationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_encoding_configuration"
}

func (d *EncodingConfigurationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves the Jellyfin server encoding configuration.",
		Attributes: map[string]schema.Attribute{
			"config_json": schema.StringAttribute{
				MarkdownDescription: "The full encoding configuration as a JSON string.",
				Computed:            true,
				CustomType:          jsontypes.NormalizedType{},
			},
		},
	}
}

func (d *EncodingConfigurationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *EncodingConfigurationDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	config, err := d.client.GetEncodingOptions()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get encoding configuration", err.Error())
		return
	}

	normalized, err := normalizeJSON(config.RawJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize encoding configuration", err.Error())
		return
	}

	data := EncodingConfigurationDataSourceModel{
		ConfigJSON: jsontypes.NewNormalizedValue(normalized),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
