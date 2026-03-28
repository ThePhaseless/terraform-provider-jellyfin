// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &UserResource{}
	_ resource.ResourceWithImportState = &UserResource{}
)

// NewUserResource creates a new user resource.
func NewUserResource() resource.Resource {
	return &UserResource{}
}

// UserResource defines the resource implementation.
type UserResource struct {
	client *client.Client
}

// UserResourceModel describes the resource data model.
type UserResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Password         types.String `tfsdk:"password"`
	IsAdministrator  types.Bool   `tfsdk:"is_administrator"`
	IsDisabled       types.Bool   `tfsdk:"is_disabled"`
	EnableAllFolders types.Bool   `tfsdk:"enable_all_folders"`
}

func (r *UserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jellyfin user.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique user identifier.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The username.",
				Required:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "The user password.",
				Optional:            true,
				Sensitive:           true,
			},
			"is_administrator": schema.BoolAttribute{
				MarkdownDescription: "Whether the user is an administrator.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"is_disabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the user is disabled.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"enable_all_folders": schema.BoolAttribute{
				MarkdownDescription: "Whether the user has access to all folders.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
		},
	}
}

func (r *UserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}

	r.client = c
}

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	password := ""
	if !data.Password.IsNull() {
		password = data.Password.ValueString()
	}

	user, err := r.client.CreateUser(data.Name.ValueString(), password)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create user", err.Error())
		return
	}

	data.ID = types.StringValue(user.Id)

	// Read back the user to get the full policy with provider IDs.
	freshUser, err := r.client.GetUserByID(user.Id)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read user after creation", err.Error())
		return
	}

	// Update user policy, preserving required fields.
	freshUser.Policy.IsAdministrator = data.IsAdministrator.ValueBool()
	freshUser.Policy.IsDisabled = data.IsDisabled.ValueBool()
	freshUser.Policy.EnableAllFolders = data.EnableAllFolders.ValueBool()

	if err := r.client.UpdateUserPolicy(freshUser.Id, &freshUser.Policy); err != nil {
		resp.Diagnostics.AddError("Failed to update user policy", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := r.client.GetUserByID(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read user", err.Error())
		return
	}

	data.Name = types.StringValue(user.Name)
	data.IsAdministrator = types.BoolValue(user.Policy.IsAdministrator)
	data.IsDisabled = types.BoolValue(user.Policy.IsDisabled)
	data.EnableAllFolders = types.BoolValue(user.Policy.EnableAllFolders)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read current user to get existing policy with required fields.
	currentUser, err := r.client.GetUserByID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read current user", err.Error())
		return
	}

	currentUser.Name = data.Name.ValueString()

	if err := r.client.UpdateUser(currentUser); err != nil {
		resp.Diagnostics.AddError("Failed to update user", err.Error())
		return
	}

	// Update policy fields while preserving required provider IDs.
	currentUser.Policy.IsAdministrator = data.IsAdministrator.ValueBool()
	currentUser.Policy.IsDisabled = data.IsDisabled.ValueBool()
	currentUser.Policy.EnableAllFolders = data.EnableAllFolders.ValueBool()

	if err := r.client.UpdateUserPolicy(currentUser.Id, &currentUser.Policy); err != nil {
		resp.Diagnostics.AddError("Failed to update user policy", err.Error())
		return
	}

	// Update password if changed.
	if !data.Password.IsNull() && !data.Password.Equal(state.Password) {
		if err := r.client.UpdateUserPassword(currentUser.Id, "", data.Password.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to update user password", err.Error())
			return
		}
	}

	data.ID = state.ID

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteUser(data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete user", err.Error())
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
