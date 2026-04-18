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
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &SystemConfigurationDataSource{}

// NewSystemConfigurationDataSource creates a new system configuration data source.
func NewSystemConfigurationDataSource() datasource.DataSource {
	return &SystemConfigurationDataSource{}
}

// SystemConfigurationDataSource defines the data source implementation.
type SystemConfigurationDataSource struct {
	client *client.Client
}

// SystemConfigurationDataSourceModel describes the data source data model.
type SystemConfigurationDataSourceModel struct {
	ServerName               types.String         `tfsdk:"server_name"`
	IsStartupWizardCompleted types.Bool           `tfsdk:"is_startup_wizard_completed"`
	ConfigJSON               jsontypes.Normalized `tfsdk:"config_json"`
}

func (d *SystemConfigurationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_configuration"
}

func (d *SystemConfigurationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves the Jellyfin server system configuration.",
		Attributes: map[string]schema.Attribute{
			"server_name": schema.StringAttribute{
				MarkdownDescription: "The server display name.",
				Computed:            true,
			},
			"is_startup_wizard_completed": schema.BoolAttribute{
				MarkdownDescription: "Whether the initial setup wizard has been completed.",
				Computed:            true,
			},
			"config_json": schema.StringAttribute{
				MarkdownDescription: "The full system configuration as a JSON string.",
				Computed:            true,
				CustomType:          jsontypes.NormalizedType{},
			},
		},
	}
}

func (d *SystemConfigurationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SystemConfigurationDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	config, err := d.client.GetSystemConfiguration()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get system configuration", err.Error())
		return
	}

	normalized, err := normalizeJSON(config.RawJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize system configuration", err.Error())
		return
	}

	data := SystemConfigurationDataSourceModel{
		ServerName:               types.StringValue(config.ServerName),
		IsStartupWizardCompleted: types.BoolValue(config.IsStartupWizardCompleted),
		ConfigJSON:               jsontypes.NewNormalizedValue(normalized),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
