// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &PluginRepositoryResource{}

// NewPluginRepositoryResource creates a new plugin repository resource.
func NewPluginRepositoryResource() resource.Resource {
	return &PluginRepositoryResource{}
}

// PluginRepositoryResource defines the resource implementation.
type PluginRepositoryResource struct {
	client *client.Client
}

// PluginRepositoryResourceModel describes the resource data model.
type PluginRepositoryResourceModel struct {
	Name    types.String `tfsdk:"name"`
	URL     types.String `tfsdk:"url"`
	Enabled types.Bool   `tfsdk:"enabled"`
}

func (r *PluginRepositoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin_repository"
}

func (r *PluginRepositoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a plugin repository in Jellyfin. " +
			"Plugin repositories are managed as a set — this resource adds, updates, or removes " +
			"a single repository from the server's list.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "The repository name.",
				Required:            true,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "The repository URL.",
				Required:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the repository is enabled.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
		},
	}
}

func (r *PluginRepositoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PluginRepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PluginRepositoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repos, err := r.client.GetPluginRepositories()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get plugin repositories", err.Error())
		return
	}

	newRepo := client.PluginRepository{
		Name:    data.Name.ValueString(),
		Url:     data.URL.ValueString(),
		Enabled: data.Enabled.ValueBool(),
	}

	repos = append(repos, newRepo)

	if err := r.client.SetPluginRepositories(repos); err != nil {
		resp.Diagnostics.AddError("Failed to set plugin repositories", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginRepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PluginRepositoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repos, err := r.client.GetPluginRepositories()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get plugin repositories", err.Error())
		return
	}

	found := false
	for _, repo := range repos {
		if repo.Name == data.Name.ValueString() {
			data.URL = types.StringValue(repo.Url)
			data.Enabled = types.BoolValue(repo.Enabled)
			found = true
			break
		}
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginRepositoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PluginRepositoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state PluginRepositoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repos, err := r.client.GetPluginRepositories()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get plugin repositories", err.Error())
		return
	}

	updated := make([]client.PluginRepository, 0, len(repos))
	for _, repo := range repos {
		if repo.Name == state.Name.ValueString() {
			repo.Name = data.Name.ValueString()
			repo.Url = data.URL.ValueString()
			repo.Enabled = data.Enabled.ValueBool()
		}
		updated = append(updated, repo)
	}

	if err := r.client.SetPluginRepositories(updated); err != nil {
		resp.Diagnostics.AddError("Failed to set plugin repositories", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginRepositoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PluginRepositoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repos, err := r.client.GetPluginRepositories()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get plugin repositories", err.Error())
		return
	}

	filtered := make([]client.PluginRepository, 0, len(repos))
	for _, repo := range repos {
		if repo.Name != data.Name.ValueString() {
			filtered = append(filtered, repo)
		}
	}

	if err := r.client.SetPluginRepositories(filtered); err != nil {
		resp.Diagnostics.AddError("Failed to set plugin repositories", err.Error())
	}
}
