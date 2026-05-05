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
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &PluginRepositoryResource{}
	_ resource.ResourceWithImportState = &PluginRepositoryResource{}
)

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

	if repositoryNameExists(repos, data.Name.ValueString()) {
		resp.Diagnostics.AddError(
			"Plugin repository already exists",
			fmt.Sprintf("A plugin repository named %q already exists. Repository names must be unique for this resource to manage them safely.", data.Name.ValueString()),
		)
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
	index, err := findPluginRepositoryIndex(repos, data.Name.ValueString(), data.URL.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Plugin repository is ambiguous", err.Error())
		return
	}
	if index >= 0 {
		repo := repos[index]
		data.URL = types.StringValue(repo.Url)
		data.Enabled = types.BoolValue(repo.Enabled)
		found = true
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
	index, err := findPluginRepositoryIndex(repos, state.Name.ValueString(), state.URL.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Plugin repository is ambiguous", err.Error())
		return
	}
	if index < 0 {
		resp.Diagnostics.AddError(
			"Plugin repository not found",
			fmt.Sprintf("Plugin repository %q was not found on the server. It may have been removed outside of Terraform.", state.Name.ValueString()),
		)
		return
	}

	if state.Name.ValueString() != data.Name.ValueString() && repositoryNameExists(repos, data.Name.ValueString()) {
		resp.Diagnostics.AddError(
			"Plugin repository already exists",
			fmt.Sprintf("A plugin repository named %q already exists. Repository names must be unique for this resource to manage them safely.", data.Name.ValueString()),
		)
		return
	}

	for i, repo := range repos {
		if i == index {
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
	index, err := findPluginRepositoryIndex(repos, data.Name.ValueString(), data.URL.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Plugin repository is ambiguous", err.Error())
		return
	}
	if index < 0 {
		return
	}

	for i, repo := range repos {
		if i != index {
			filtered = append(filtered, repo)
		}
	}

	if err := r.client.SetPluginRepositories(filtered); err != nil {
		resp.Diagnostics.AddError("Failed to set plugin repositories", err.Error())
	}
}

func (r *PluginRepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func repositoryNameExists(repos []client.PluginRepository, name string) bool {
	for _, repo := range repos {
		if repo.Name == name {
			return true
		}
	}

	return false
}

// findPluginRepositoryIndex returns the matching repository index, -1 when no
// repository is found, or an error when the repository name is ambiguous and
// the URL does not disambiguate it.
func findPluginRepositoryIndex(repos []client.PluginRepository, name, url string) (int, error) {
	matches := make([]int, 0, 1)
	for i, repo := range repos {
		if repo.Name == name {
			matches = append(matches, i)
		}
	}

	switch len(matches) {
	case 0:
		return -1, nil
	case 1:
		return matches[0], nil
	}

	if url != "" {
		for _, i := range matches {
			if repos[i].Url == url {
				return i, nil
			}
		}
	}

	return -1, fmt.Errorf("multiple plugin repositories named %q exist on the server; use unique repository names to manage or import this resource safely", name)
}
