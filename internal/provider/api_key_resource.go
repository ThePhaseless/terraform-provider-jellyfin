// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &APIKeyResource{}

// NewAPIKeyResource creates a new API key resource.
func NewAPIKeyResource() resource.Resource {
	return &APIKeyResource{}
}

// APIKeyResource defines the resource implementation.
type APIKeyResource struct {
	client *client.Client
}

// APIKeyResourceModel describes the resource data model.
type APIKeyResourceModel struct {
	AppName     types.String `tfsdk:"app_name"`
	AccessToken types.String `tfsdk:"access_token"`
}

func (r *APIKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *APIKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an API key in Jellyfin. API keys provide authentication tokens for " +
			"external applications to access the Jellyfin API.",
		Attributes: map[string]schema.Attribute{
			"app_name": schema.StringAttribute{
				MarkdownDescription: "The application name for the API key.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"access_token": schema.StringAttribute{
				MarkdownDescription: "The generated access token (API key). This is set by Jellyfin upon creation.",
				Computed:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *APIKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *APIKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data APIKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.CreateAPIKey(data.AppName.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to create API key", err.Error())
		return
	}

	// Read back the key to get the generated access token.
	key, err := r.client.GetAPIKeyByAppName(data.AppName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read created API key", err.Error())
		return
	}

	data.AccessToken = types.StringValue(key.AccessToken)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *APIKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data APIKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Look up the key by access token to verify it still exists.
	keys, err := r.client.GetAPIKeys()
	if err != nil {
		resp.Diagnostics.AddError("Failed to read API keys", err.Error())
		return
	}

	found := false
	for _, key := range keys {
		if key.AccessToken == data.AccessToken.ValueString() {
			data.AppName = types.StringValue(key.AppName)
			found = true
			break
		}
	}

	if !found {
		// Key was deleted outside of Terraform.
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *APIKeyResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// app_name requires replacement, so Update should never be called.
	resp.Diagnostics.AddError("Unexpected Update", "API key resource does not support in-place updates.")
}

func (r *APIKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data APIKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteAPIKey(data.AccessToken.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete API key", err.Error())
	}
}
