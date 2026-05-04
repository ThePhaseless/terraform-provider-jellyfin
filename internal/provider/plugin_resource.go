// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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
		MarkdownDescription: "Installs a plugin on the Jellyfin server. The server may require a restart after installation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The plugin ID assigned by Jellyfin after installation.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The plugin package name.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "The plugin version to install.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"repository_url": schema.StringAttribute{
				MarkdownDescription: "The repository URL from which to install the plugin. Resolved automatically on import.",
				Optional:            true,
				Computed:            true,
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

	if err := r.client.InstallPlugin(data.Name.ValueString(), data.Version.ValueString(), data.RepositoryURL.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to install plugin", err.Error())
		return
	}

	// Wait for the plugin to appear in the installed list.
	pluginID, err := r.waitForPlugin(ctx, data.Name.ValueString(), 30*time.Second)
	if err != nil {
		resp.Diagnostics.AddError("Plugin installed but not found in installed list", err.Error())
		return
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

	plugins, err := r.client.GetInstalledPlugins()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get installed plugins", err.Error())
		return
	}

	found := false
	for _, p := range plugins {
		if p.Id == data.ID.ValueString() || p.Name == data.Name.ValueString() {
			data.ID = types.StringValue(p.Id)
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

	if err := r.client.UninstallPlugin(data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to uninstall plugin", err.Error())
	}
}

func (r *PluginResource) waitForPlugin(ctx context.Context, name string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		plugins, err := r.client.GetInstalledPlugins()
		if err != nil {
			return "", err
		}
		for _, p := range plugins {
			if p.Name == name {
				return p.Id, nil
			}
		}
		tflog.Debug(ctx, "Waiting for plugin to appear", map[string]interface{}{"plugin": name})
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("plugin %q did not appear within %s", name, timeout)
}

func (r *PluginResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// resolveRepositoryURL attempts to find the repository URL for a plugin by
// querying the /Packages endpoint and matching on name and version.
func (r *PluginResource) resolveRepositoryURL(ctx context.Context, name, version string) string {
	pkgs, err := r.client.GetAvailablePackages()
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
				if v.Version == version && v.RepositoryUrl != "" {
					return v.RepositoryUrl
				}
			}
			// If exact version not found, fall back to the first version's repo URL
			// (the repository is typically the same across versions).
			if len(pkg.Versions) > 0 && pkg.Versions[0].RepositoryUrl != "" {
				tflog.Debug(ctx, "Exact version not found in packages, falling back to first available version's repository URL", map[string]interface{}{
					"plugin":            name,
					"requested_version": version,
					"fallback_version":  pkg.Versions[0].Version,
				})
				return pkg.Versions[0].RepositoryUrl
			}
		}
	}

	return ""
}
