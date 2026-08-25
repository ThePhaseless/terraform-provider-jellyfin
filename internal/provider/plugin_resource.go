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

// pluginInstallTimeout bounds the wait for a Jellyfin install to land on disk;
// the download runs asynchronously after the API call returns.
const (
	pluginInstallTimeout = 2 * time.Minute
	pluginPollInterval   = 2 * time.Second
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
				Description:         "The plugin version to install. Omit to install the latest available version from the repository.",
				MarkdownDescription: "The plugin version to install. Omit to install the latest available version from the repository.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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
	// Resolve version: "supported" (or unset for known plugins) uses the
	// hardcoded supported version; "latest" resolves the newest from the
	// repository manifest; any other value is used as-is.
	resolvedVersion, err := r.resolvePluginVersion(ctx, data.Name.ValueString(), data.Version)
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve plugin version", err.Error())
		return
	}
	if resolvedVersion == "" {
		resp.Diagnostics.AddError(
			"Plugin not found in available packages",
			fmt.Sprintf("No package named %q found in configured repositories. Register the plugin repository first.", data.Name.ValueString()),
		)
		return
	}
	data.Version = types.StringValue(resolvedVersion)

	// Jellyfin returns 404 when POSTing an install for a version that is
	// already present, so detect that up front and treat it as idempotent
	// rather than erroring.
	pluginID, err := r.findInstalledPlugin(ctx, data.Name.ValueString(), data.Version.ValueString())
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

		id, err := r.waitForPlugin(ctx, data.Name.ValueString(), data.Version.ValueString(), pluginInstallTimeout)
		if err != nil {
			resp.Diagnostics.AddError("Plugin install did not complete", err.Error())
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

// waitForPlugin blocks until name is installed at version and returns its id.
//
// Matching the version, not just the name, is what lets a caller restart the
// server afterwards and be sure it loads the assembly this install put down:
// Jellyfin's install is asynchronous and returns long before the download
// lands, so a name-only match returns while the previous version is still the
// only one on disk. Jellyfin registers the new version as soon as it is
// written, with status "Restart" and the version it replaces "Superceded", so
// this does not wait on a restart that has not happened yet.
func (r *PluginResource) waitForPlugin(ctx context.Context, name, version string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var seen string
	for time.Now().Before(deadline) {
		plugins, err := r.client.GetInstalledPlugins(ctx)
		if err != nil {
			return "", err
		}
		for _, p := range plugins {
			if p.Name != name {
				continue
			}
			if samePluginVersion(p.Version, version) {
				return p.ID, nil
			}
			seen = p.Version
		}
		tflog.Debug(ctx, "Waiting for plugin to appear", map[string]interface{}{"plugin": name, "version": version, "seen": seen})
		time.Sleep(pluginPollInterval)
	}
	if seen != "" {
		return "", fmt.Errorf("plugin %q is installed at %s but %s did not appear within %s", name, seen, version, timeout)
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
func (r *PluginResource) findInstalledPlugin(ctx context.Context, name, version string) (string, error) {
	plugins, err := r.client.GetInstalledPlugins(ctx)
	if err != nil {
		return "", fmt.Errorf("listing installed plugins: %w", err)
	}
	for _, p := range plugins {
		if p.Name == name && samePluginVersion(p.Version, version) {
			return p.ID, nil
		}
	}
	return "", nil
}

// samePluginVersion reports whether two plugin versions denote the same
// release. Jellyfin reports four-segment assembly versions (2.5.22.0) while a
// configuration may carry the three-segment release (2.5.22), so the trailing
// zero must not make them differ. An empty want matches anything.
func samePluginVersion(got, want string) bool {
	if want == "" {
		return true
	}
	return compareDottedVersions(got, want) == 0
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

// resolvePluginVersion resolves the version for a plugin install.
//
//   - "supported" or unset for a known plugin → hardcoded supported version
//   - "latest" → newest version from the repository manifest, with a warning
//     if newer than the supported version (when one exists)
//   - Any other value (e.g. "2.5.20.0") → used as-is
//   - Unset for unknown plugins → resolves latest from the repository.
func (r *PluginResource) resolvePluginVersion(ctx context.Context, name string, version types.String) (string, error) {
	supported := supportedVersionForPlugin(name)

	switch {
	case version.IsNull() || version.IsUnknown() || version.ValueString() == "":
		// Unset: use supported for known plugins, latest for others.
		if supported != "" {
			return supported, nil
		}
		return r.resolveLatestVersion(ctx, name)

	case version.ValueString() == "supported":
		if supported == "" {
			return "", fmt.Errorf("version %q is not available for plugin %q — no supported version is defined", "supported", name)
		}
		return supported, nil

	case version.ValueString() == "latest":
		latest, err := r.resolveLatestVersion(ctx, name)
		if err != nil {
			return "", err
		}
		if supported != "" && latest != "" {
			if c := compareDottedVersions(latest, supported); c > 0 {
				resp := "" // placeholder — warning is logged below
				_ = resp
				tflog.Warn(ctx, "Plugin version newer than supported", map[string]interface{}{
					"plugin":    name,
					"latest":    latest,
					"supported": supported,
					"warning":   fmt.Sprintf("Installing %s v%s which is newer than the tested/supported v%s. The typed Terraform resource may not cover all properties in this version.", name, latest, supported),
				})
			}
		}
		return latest, nil

	default:
		return version.ValueString(), nil
	}
}

// resolveLatestVersion fetches the latest version for a plugin from the
// configured repository manifests via the /Packages endpoint.
func (r *PluginResource) resolveLatestVersion(ctx context.Context, name string) (string, error) {
	pkgs, err := r.client.GetAvailablePackages(ctx)
	if err != nil {
		return "", fmt.Errorf("listing available packages: %w", err)
	}

	for _, pkg := range pkgs {
		if pkg.Name == name {
			if len(pkg.Versions) == 0 {
				return "", fmt.Errorf("plugin %q has no available versions", name)
			}
			// Manifests list newest version first.
			return pkg.Versions[0].Version, nil
		}
	}

	return "", nil
}

// supportedVersionForPlugin returns the hardcoded supported version for a
// plugin by name, or "" if no supported version is tracked.
func supportedVersionForPlugin(name string) string {
	switch name {
	case "Jellyfin Security":
		return supportedSecurityPluginVersion()
	default:
		return ""
	}
}
