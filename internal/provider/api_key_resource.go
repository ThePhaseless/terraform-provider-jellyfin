// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

var (
	_ resource.Resource                = &APIKeyResource{}
	_ resource.ResourceWithImportState = &APIKeyResource{}
)

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
	ID          types.String `tfsdk:"id"`
	AppName     types.String `tfsdk:"app_name"`
	AccessToken types.String `tfsdk:"access_token"`
}

func (r *APIKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *APIKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an API key in Jellyfin. API keys provide authentication tokens for " +
			"external applications to access the Jellyfin API.",
		MarkdownDescription: "Manages an API key in Jellyfin. API keys provide authentication tokens for " +
			"external applications to access the Jellyfin API.",
		Attributes: map[string]schema.Attribute{
			"app_name": schema.StringAttribute{
				Description:         "The application name for the API key.",
				MarkdownDescription: "The application name for the API key.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": schema.StringAttribute{
				Description:         "The API key resource identifier.",
				MarkdownDescription: "The API key resource identifier.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"access_token": schema.StringAttribute{
				Description:         "The generated access token (API key). This is set by Jellyfin upon creation.",
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

	// Snapshot existing keys before creation so we can identify the new one.
	before, err := r.client.GetAPIKeys(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list API keys before creation", err.Error())
		return
	}
	existingTokens := make(map[string]struct{}, len(before))
	for _, k := range before {
		existingTokens[k.AccessToken] = struct{}{}
	}

	if err := r.client.CreateAPIKey(ctx, data.AppName.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to create API key", err.Error())
		return
	}

	// Find the newly created key by diffing against the pre-creation snapshot.
	after, err := r.client.GetAPIKeys(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list API keys after creation", err.Error())
		return
	}

	var newKey *client.APIKey
	for i := range after {
		if _, existed := existingTokens[after[i].AccessToken]; !existed {
			newKey = &after[i]
			break
		}
	}

	if newKey == nil {
		resp.Diagnostics.AddError("Failed to find created API key", "The newly created API key was not found in the server response.")
		return
	}

	data.AccessToken = types.StringValue(newKey.AccessToken)
	data.ID = types.StringValue(newKey.AccessToken)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *APIKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data APIKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Look up the key by access token to verify it still exists.
	keys, err := r.client.GetAPIKeys(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read API keys", err.Error())
		return
	}

	found := false
	for _, key := range keys {
		if key.AccessToken == data.AccessToken.ValueString() {
			data.AppName = types.StringValue(key.AppName)
			data.ID = types.StringValue(key.AccessToken)
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

	if err := r.client.DeleteAPIKey(ctx, data.AccessToken.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete API key", err.Error())
	}
}

func (r *APIKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("access_token"), req, resp)
}
