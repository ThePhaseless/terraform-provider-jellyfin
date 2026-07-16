// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

var (
	_ resource.Resource                = &PluginResource{}
	_ resource.ResourceWithImportState = &PluginResource{}
)

// NewPluginResource creates a new plugin resource.
func NewPluginResource() resource.Resource {
	return &PluginResource{}
}

// PluginResource defines the resource implementation.
type PluginResource struct {
	client *client.Client
}

// PluginResourceModel describes the resource data model.
type PluginResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Version       types.String `tfsdk:"version"`
	RepositoryURL types.String `tfsdk:"repository_url"`
}

func (r *PluginResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin"
}

func (r *PluginResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Installs a plugin on the Jellyfin server. The server may require a restart after installation.",
		MarkdownDescription: "Installs a plugin on the Jellyfin server. The server may require a restart after installation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "The plugin ID assigned by Jellyfin after installation.",
				MarkdownDescription: "The plugin ID assigned by Jellyfin after installation.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description:         "The plugin package name. Used as the import key (e.g. `terraform import jellyfin_plugin.x \"SSO-Auth\"`).",
				MarkdownDescription: "The plugin package name. Used as the import key (e.g. `terraform import jellyfin_plugin.x \"SSO-Auth\"`).",
				Required:            true,
				Validators:          requiredIdentifierValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.StringAttribute{
				Description:         "The plugin version to install.",
				MarkdownDescription: "The plugin version to install.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"repository_url": schema.StringAttribute{
				Description:         "The repository URL from which to install the plugin. Required when creating the resource and resolved automatically on import when the exact package version is still available.",
				MarkdownDescription: "The repository URL from which to install the plugin. Required when creating the resource and resolved automatically on import when the exact package version is still available.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *PluginResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PluginResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PluginResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.RepositoryURL.IsUnknown() || data.RepositoryURL.IsNull() || data.RepositoryURL.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing plugin repository URL",
			"The repository_url attribute must be set when installing a plugin so the provider can reproduce the install source.",
		)
		return
	}

	// Check if the plugin is already installed. Jellyfin returns 404 when
	// POSTing an install for a plugin that is already present, so we detect
	// that case up front and treat it as idempotent rather than erroring.
	pluginID, err := r.findInstalledPlugin(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to check installed plugins", err.Error())
		return
	}

	if pluginID == "" {
		if err := r.client.InstallPlugin(ctx, data.Name.ValueString(), data.Version.ValueString(), data.RepositoryURL.ValueString()); err != nil {
			// If the install failed because the plugin is already installed
			// (e.g. a concurrent install raced ahead of us), treat it as
			// success and reconcile via the installed list below.
			if !client.IsNotFound(err) {
				resp.Diagnostics.AddError("Failed to install plugin", err.Error())
				return
			}
		}

		// Wait for the plugin to appear in the installed list.
		id, err := r.waitForPlugin(ctx, data.Name.ValueString(), 30*time.Second)
		if err != nil {
			resp.Diagnostics.AddError("Plugin installed but not found in installed list", err.Error())
			return
		}
		pluginID = id
	}

	data.ID = types.StringValue(pluginID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PluginResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plugins, err := r.client.GetInstalledPlugins(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get installed plugins", err.Error())
		return
	}

	found := false
	for _, p := range plugins {
		if p.ID == data.ID.ValueString() || p.Name == data.Name.ValueString() {
			data.ID = types.StringValue(p.ID)
			data.Name = types.StringValue(p.Name)
			data.Version = types.StringValue(p.Version)
			found = true
			break
		}
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	// Populate repository_url from available packages if not already set.
	if data.RepositoryURL.IsNull() || data.RepositoryURL.ValueString() == "" {
		repoURL := r.resolveRepositoryURL(ctx, data.Name.ValueString(), data.Version.ValueString())
		if repoURL != "" {
			data.RepositoryURL = types.StringValue(repoURL)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All attributes require replace, so Update should never be called.
	resp.Diagnostics.AddError("Update not supported", "Plugin updates require replacement.")
}

func (r *PluginResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PluginResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UninstallPlugin(ctx, data.ID.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to uninstall plugin", err.Error())
	}
}

func (r *PluginResource) waitForPlugin(ctx context.Context, name string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		plugins, err := r.client.GetInstalledPlugins(ctx)
		if err != nil {
			return "", err
		}
		for _, p := range plugins {
			if p.Name == name {
				return p.ID, nil
			}
		}
		tflog.Debug(ctx, "Waiting for plugin to appear", map[string]interface{}{"plugin": name})
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("plugin %q did not appear within %s", name, timeout)
}

func (r *PluginResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Plugins can be imported by name (e.g. `terraform import jellyfin_plugin.x
	// "SSO-Auth"`) or by the server-assigned UUID. We set the import ID into both
	// `id` and `name` so Read can match whichever one is correct — it already
	// checks `p.ID == data.ID || p.Name == data.Name` and overwrites both with
	// the canonical values from the server afterward.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// findInstalledPlugin returns the plugin ID if a plugin with the given name is
// already installed, or an empty string if it is not.
func (r *PluginResource) findInstalledPlugin(ctx context.Context, name string) (string, error) {
	plugins, err := r.client.GetInstalledPlugins(ctx)
	if err != nil {
		return "", fmt.Errorf("listing installed plugins: %w", err)
	}
	for _, p := range plugins {
		if p.Name == name {
			return p.ID, nil
		}
	}
	return "", nil
}

// resolveRepositoryURL attempts to find the repository URL for a plugin by
// querying the /Packages endpoint and matching on name and version.
func (r *PluginResource) resolveRepositoryURL(ctx context.Context, name, version string) string {
	pkgs, err := r.client.GetAvailablePackages(ctx)
	if err != nil {
		tflog.Debug(ctx, "Could not resolve repository URL for plugin (packages unavailable)", map[string]interface{}{
			"plugin": name,
			"error":  err.Error(),
		})
		return ""
	}

	for _, pkg := range pkgs {
		if pkg.Name == name {
			for _, v := range pkg.Versions {
				if v.Version == version && v.RepositoryURL != "" {
					return v.RepositoryURL
				}
			}

			tflog.Debug(ctx, "Could not resolve repository URL for plugin (exact version unavailable)", map[string]interface{}{
				"plugin":  name,
				"version": version,
			})
			return ""
		}
	}

	return ""
}
