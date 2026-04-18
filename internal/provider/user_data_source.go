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

var _ datasource.DataSource = &UserDataSource{}

// NewUserDataSource creates a new user data source.
func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

// UserDataSource defines the data source implementation.
type UserDataSource struct {
	client *client.Client
}

// UserDataSourceModel describes the data source data model.
type UserDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	IsAdministrator  types.Bool   `tfsdk:"is_administrator"`
	IsDisabled       types.Bool   `tfsdk:"is_disabled"`
	EnableAllFolders types.Bool   `tfsdk:"enable_all_folders"`
}

func (d *UserDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *UserDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Jellyfin user by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "The username to look up.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique user identifier.",
				Computed:            true,
			},
			"is_administrator": schema.BoolAttribute{
				MarkdownDescription: "Whether the user is an administrator.",
				Computed:            true,
			},
			"is_disabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the user is disabled.",
				Computed:            true,
			},
			"enable_all_folders": schema.BoolAttribute{
				MarkdownDescription: "Whether the user has access to all folders.",
				Computed:            true,
			},
		},
	}
}

func (d *UserDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data UserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	users, err := d.client.GetUsers()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get users", err.Error())
		return
	}

	name := data.Name.ValueString()
	var match *client.User
	for i := range users {
		if users[i].Name == name {
			match = &users[i]
			break
		}
	}

	if match == nil {
		resp.Diagnostics.AddError("User not found", fmt.Sprintf("No user with name %q was found.", name))
		return
	}

	data.ID = types.StringValue(match.Id)
	data.Name = types.StringValue(match.Name)
	data.IsAdministrator = types.BoolValue(match.Policy.IsAdministrator)
	data.IsDisabled = types.BoolValue(match.Policy.IsDisabled)
	data.EnableAllFolders = types.BoolValue(match.Policy.EnableAllFolders)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
