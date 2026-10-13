// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
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
				Description:         "The application name for the API key. Defaults to `Terraform`.",
				MarkdownDescription: "The application name for the API key. Defaults to `Terraform`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("Terraform"),
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
				Sensitive:           true,
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
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *APIKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data APIKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Jellyfin does not return the new key, so Create diffs the listings before
	// and after; a concurrent Create would pollute that diff.
	apiKeyCreateMu.Lock()
	defer apiKeyCreateMu.Unlock()

	before, err := r.client.GetAPIKeys(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list API keys before creation", err.Error())
		return
	}

	if err := r.client.CreateAPIKey(ctx, data.AppName.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to create API key", err.Error())
		return
	}

	after, err := r.client.GetAPIKeys(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list API keys after creation", err.Error())
		return
	}

	newKey := createdAPIKey(before, after, data.AppName.ValueString())
	if newKey == nil {
		resp.Diagnostics.AddError("Failed to find created API key", "The newly created API key was not found in the server response.")
		return
	}

	data.AccessToken = types.StringValue(newKey.AccessToken)
	data.ID = types.StringValue(newKey.AccessToken)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// apiKeyCreateMu serializes the creates of this provider, each of which finds
// the key it made by comparing the keys listed before and after.
var apiKeyCreateMu sync.Mutex

// createdAPIKey returns the key after lists and before does not: the one
// named appName, else the only new one, else nil.
func createdAPIKey(before, after []client.APIKey, appName string) *client.APIKey {
	existing := make(map[string]bool, len(before))
	for _, k := range before {
		existing[k.AccessToken] = true
	}
	var created []*client.APIKey
	for i := range after {
		if existing[after[i].AccessToken] {
			continue
		}
		if after[i].AppName == appName {
			return &after[i]
		}
		created = append(created, &after[i])
	}
	if len(created) == 1 {
		return created[0]
	}
	return nil
}

func (r *APIKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data APIKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	keys, err := r.client.GetAPIKeys(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read API keys", err.Error())
		return
	}

	i := slices.IndexFunc(keys, func(k client.APIKey) bool { return k.AccessToken == data.AccessToken.ValueString() })
	if i < 0 {
		resp.State.RemoveResource(ctx)
		return
	}
	data.AppName = types.StringValue(keys[i].AppName)
	data.ID = types.StringValue(keys[i].AccessToken)

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

	if err := r.client.DeleteAPIKey(ctx, data.AccessToken.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete API key", err.Error())
	}
}

// ImportState takes the access token of the key or, when no key has that
// token, its app name. Read fills in the rest from the access token.
func (r *APIKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	keys, err := r.client.GetAPIKeys(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list API keys", err.Error())
		return
	}

	key, problem := importedAPIKey(keys, req.ID)
	if key == nil {
		resp.Diagnostics.AddError("Failed to import API key", problem)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), key.AccessToken)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("access_token"), key.AccessToken)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app_name"), key.AppName)...)
}

// importedAPIKey returns the key whose access token is id, else the only key
// whose app name is id. Without one it returns nil and what is wrong, which
// never contains an access token.
func importedAPIKey(keys []client.APIKey, id string) (*client.APIKey, string) {
	if i := slices.IndexFunc(keys, func(k client.APIKey) bool { return k.AccessToken == id }); i >= 0 {
		return &keys[i], ""
	}

	var named []*client.APIKey
	for i := range keys {
		if keys[i].AppName == id {
			named = append(named, &keys[i])
		}
	}
	switch len(named) {
	case 0:
		return nil, fmt.Sprintf("No API key has %q as its access token or app name.", id)
	case 1:
		return named[0], ""
	default:
		return nil, fmt.Sprintf("%d API keys have the app name %q. Import one of them by its access token.", len(named), id)
	}
}
