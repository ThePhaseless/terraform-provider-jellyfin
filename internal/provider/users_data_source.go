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

var _ datasource.DataSource = &UsersDataSource{}

// NewUsersDataSource creates a new users list data source.
func NewUsersDataSource() datasource.DataSource {
	return &UsersDataSource{}
}

// UsersDataSource defines the data source implementation.
type UsersDataSource struct {
	client *client.Client
}

// UsersDataSourceUserModel describes a single user element in the list.
type UsersDataSourceUserModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	IsAdministrator types.Bool   `tfsdk:"is_administrator"`
	IsDisabled      types.Bool   `tfsdk:"is_disabled"`
}

// UsersDataSourceModel describes the data source data model.
type UsersDataSourceModel struct {
	Users []UsersDataSourceUserModel `tfsdk:"users"`
}

func (d *UsersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

func (d *UsersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all Jellyfin users on the server.",
		Attributes: map[string]schema.Attribute{
			"users": schema.ListNestedAttribute{
				MarkdownDescription: "The list of users.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The unique user identifier.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The username.",
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
					},
				},
			},
		},
	}
}

func (d *UsersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *UsersDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	users, err := d.client.GetUsers()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get users", err.Error())
		return
	}

	data := UsersDataSourceModel{
		Users: make([]UsersDataSourceUserModel, 0, len(users)),
	}
	for _, u := range users {
		data.Users = append(data.Users, UsersDataSourceUserModel{
			ID:              types.StringValue(u.Id),
			Name:            types.StringValue(u.Name),
			IsAdministrator: types.BoolValue(u.Policy.IsAdministrator),
			IsDisabled:      types.BoolValue(u.Policy.IsDisabled),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
