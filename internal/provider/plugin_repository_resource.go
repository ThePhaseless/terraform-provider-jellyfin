// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
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
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	URL     types.String `tfsdk:"url"`
	Enabled types.Bool   `tfsdk:"enabled"`
}

func (r *PluginRepositoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin_repository"
}

func (r *PluginRepositoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a plugin repository in Jellyfin. " +
			"Plugin repositories are managed as a set — this resource adds, updates, or removes " +
			"a single repository from the server's list.",
		MarkdownDescription: "Manages a plugin repository in Jellyfin. " +
			"Plugin repositories are managed as a set — this resource adds, updates, or removes " +
			"a single repository from the server's list.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description:         "The repository name.",
				MarkdownDescription: "The repository name.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"url": schema.StringAttribute{
				Description:         "The repository URL.",
				MarkdownDescription: "The repository URL.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"id": schema.StringAttribute{
				Description:         "The plugin repository resource identifier, which is its name.",
				MarkdownDescription: "The plugin repository resource identifier, which is its name.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					idFollowsName{},
				},
			},
			"enabled": schema.BoolAttribute{
				Description:         "Whether the repository is enabled.",
				MarkdownDescription: "Whether the repository is enabled.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
		},
	}
}

func (r *PluginRepositoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *PluginRepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PluginRepositoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverConfigurationMu.Lock()
	defer serverConfigurationMu.Unlock()

	repos, err := r.client.GetPluginRepositories(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get plugin repositories", err.Error())
		return
	}

	if repositoryNameTaken(repos, data.Name.ValueString(), &resp.Diagnostics) {
		return
	}

	repos = append(repos, client.PluginRepository{
		Name:    data.Name.ValueString(),
		URL:     data.URL.ValueString(),
		Enabled: data.Enabled.ValueBool(),
	})

	if err := r.client.SetPluginRepositories(ctx, repos); err != nil {
		resp.Diagnostics.AddError("Failed to set plugin repositories", err.Error())
		return
	}

	data.ID = data.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginRepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PluginRepositoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repos, err := r.client.GetPluginRepositories(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get plugin repositories", err.Error())
		return
	}

	index, err := findPluginRepositoryIndex(repos, data.Name.ValueString(), data.URL.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Plugin repository is ambiguous", err.Error())
		return
	}
	if index < 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	data = pluginRepositoryModel(repos[index])
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

	serverConfigurationMu.Lock()
	defer serverConfigurationMu.Unlock()

	repos, err := r.client.GetPluginRepositories(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get plugin repositories", err.Error())
		return
	}

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

	if state.Name.ValueString() != data.Name.ValueString() && repositoryNameTaken(repos, data.Name.ValueString(), &resp.Diagnostics) {
		return
	}

	repos[index].Name = data.Name.ValueString()
	repos[index].URL = data.URL.ValueString()
	repos[index].Enabled = data.Enabled.ValueBool()

	if err := r.client.SetPluginRepositories(ctx, repos); err != nil {
		resp.Diagnostics.AddError("Failed to set plugin repositories", err.Error())
		return
	}

	repos, err = r.client.GetPluginRepositories(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read plugin repositories after update", err.Error())
		return
	}
	if index >= len(repos) || repos[index].Name != data.Name.ValueString() {
		index, err = findPluginRepositoryIndex(repos, data.Name.ValueString(), data.URL.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Plugin repository is ambiguous", err.Error())
			return
		}
	}
	if index < 0 || index >= len(repos) {
		resp.Diagnostics.AddError("Plugin repository not found", fmt.Sprintf("Plugin repository %q was not found after update.", state.Name.ValueString()))
		return
	}
	data = pluginRepositoryModel(repos[index])
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginRepositoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PluginRepositoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverConfigurationMu.Lock()
	defer serverConfigurationMu.Unlock()

	repos, err := r.client.GetPluginRepositories(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get plugin repositories", err.Error())
		return
	}

	index, err := findPluginRepositoryIndex(repos, data.Name.ValueString(), data.URL.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Plugin repository is ambiguous", err.Error())
		return
	}
	if index < 0 {
		return
	}

	if err := r.client.SetPluginRepositories(ctx, slices.Delete(repos, index, index+1)); err != nil {
		resp.Diagnostics.AddError("Failed to set plugin repositories", err.Error())
	}
}

func (r *PluginRepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

// serverConfigurationMu serializes writes that post back what they read from the server
// configuration, which holds the plugin repositories, so they do not drop each other's changes.
var serverConfigurationMu sync.Mutex

// idFollowsName plans id as the planned name, which it always equals, so a
// rename plans the id that Update stores.
type idFollowsName struct{}

func (idFollowsName) Description(context.Context) string {
	return "The value is the planned name."
}

func (m idFollowsName) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (idFollowsName) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var name types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("name"), &name)...)
	if !name.IsNull() && !name.IsUnknown() {
		resp.PlanValue = name
	}
}

func pluginRepositoryModel(repo client.PluginRepository) PluginRepositoryResourceModel {
	return PluginRepositoryResourceModel{
		ID:      types.StringValue(repo.Name),
		Name:    types.StringValue(repo.Name),
		URL:     types.StringValue(repo.URL),
		Enabled: types.BoolValue(repo.Enabled),
	}
}

// repositoryNameTaken, when name is taken, adds the error that says so to diags.
func repositoryNameTaken(repos []client.PluginRepository, name string, diags *diag.Diagnostics) bool {
	if !slices.ContainsFunc(repos, func(repo client.PluginRepository) bool { return repo.Name == name }) {
		return false
	}
	diags.AddError(
		"Plugin repository already exists",
		fmt.Sprintf("A plugin repository named %q already exists. Repository names must be unique for this resource to manage them safely.", name),
	)
	return true
}

// findPluginRepositoryIndex returns the matching index, -1 when none matches,
// or an error when the name is ambiguous and url does not disambiguate it.
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
			if repos[i].URL == url {
				return i, nil
			}
		}
	}

	return -1, fmt.Errorf("multiple plugin repositories named %q exist on the server; use unique repository names to manage or import this resource safely", name)
}
