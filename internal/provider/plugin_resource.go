// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/release"
)

const (
	pluginInstallTimeout = 2 * time.Minute
	pluginPollInterval   = 2 * time.Second
)

// pluginStatusDeleted is the status Jellyfin lists for a version it could not
// remove from disk and deletes at the next restart instead.
const pluginStatusDeleted = "Deleted"

const (
	pluginVersionSupported = "supported"
	pluginVersionLatest    = "latest"
)

var (
	_ resource.Resource                = &PluginResource{}
	_ resource.ResourceWithImportState = &PluginResource{}
	_ resource.ResourceWithModifyPlan  = &PluginResource{}
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
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Version          types.String `tfsdk:"version"`
	InstalledVersion types.String `tfsdk:"installed_version"`
	RepositoryURL    types.String `tfsdk:"repository_url"`
}

func (r *PluginResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin"
}

func (r *PluginResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Installs a plugin on the Jellyfin server. The server may require a restart after installation.\n\n" +
			"Jellyfin's Update Plugins scheduled task (key PluginUpdates) runs at startup and every 24 hours by default and upgrades every plugin to the newest compatible version its repositories offer. " +
			"No API exempts a single plugin: the task skips only a plugin whose meta.json, in its folder under the server's plugins directory, sets \"autoUpdate\": false. Jellyfin reads that file at startup and writes its own copy back whenever the plugin's status changes, so edit it while Jellyfin is stopped, and every install of the plugin, including the reinstall a replacement makes, writes a new one set to true. " +
			"A pinned version therefore drifts once the task runs: the newer version loads at the next restart, and because changing version replaces the resource, the next apply reinstalls the pinned version, which the task upgrades again at the following startup. " +
			"To keep a pin, set that flag on the server, or remove the task's triggers with jellyfin_scheduled_task, which stops automatic updates for every plugin. To follow updates instead, leave version unset, set it to latest, or add lifecycle { ignore_changes = [version] }.\n\n" +
			"Replacing the resource also replaces the resources that take its id, such as jellyfin_plugin_configuration, because the id is unknown until the plugin is installed again. They then write their configuration again, which Jellyfin accepts only once a restart has loaded the reinstalled plugin, so put a jellyfin_restart between them, as the jellyfin_security_plugin_configuration example does.",
		MarkdownDescription: "Installs a plugin on the Jellyfin server. The server may require a restart after installation.\n\n" +
			"Jellyfin's *Update Plugins* scheduled task (key `PluginUpdates`) runs at startup and every 24 hours by default and upgrades every plugin to the newest compatible version its repositories offer. " +
			"No API exempts a single plugin: the task skips only a plugin whose `meta.json`, in its folder under the server's `plugins` directory, sets `\"autoUpdate\": false`. Jellyfin reads that file at startup and writes its own copy back whenever the plugin's status changes, so edit it while Jellyfin is stopped, and every install of the plugin, including the reinstall a replacement makes, writes a new one set to `true`. " +
			"A pinned `version` therefore drifts once the task runs: the newer version loads at the next restart, and because changing `version` replaces the resource, the next apply reinstalls the pinned version, which the task upgrades again at the following startup. " +
			"To keep a pin, set that flag on the server, or remove the task's triggers with `jellyfin_scheduled_task`, which stops automatic updates for every plugin. To follow updates instead, leave `version` unset, set it to `latest`, or add `lifecycle { ignore_changes = [version] }`.\n\n" +
			"Replacing the resource also replaces the resources that take its `id`, such as `jellyfin_plugin_configuration`, because the `id` is unknown until the plugin is installed again. They then write their configuration again, which Jellyfin accepts only once a restart has loaded the reinstalled plugin, so put a `jellyfin_restart` between them, as the `jellyfin_security_plugin_configuration` example does.",
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
				Description:         "The plugin version to install, as the repository lists it (e.g. 13.0.0.0), or a keyword: latest installs the newest version the repositories offer, and supported installs the Jellyfin Security release this provider was tested against, in the build the server accepts. A keyword is resolved only when the plugin is installed, or when it replaces another value, and stays in state as written; installed_version holds the result. Omitted, it installs as supported does for Jellyfin Security and as latest does for any other plugin, and then holds the installed version, as it also does once a keyword is removed from the configuration. Changing the value reinstalls the plugin, unless the new value names the installed version, as a keyword set on an imported plugin usually does; then only state changes.",
				MarkdownDescription: "The plugin version to install, as the repository lists it (e.g. `13.0.0.0`), or a keyword: `latest` installs the newest version the repositories offer, and `supported` installs the Jellyfin Security release this provider was tested against, in the build the server accepts. A keyword is resolved only when the plugin is installed, or when it replaces another value, and stays in state as written; `installed_version` holds the result. Omitted, it installs as `supported` does for Jellyfin Security and as `latest` does for any other plugin, and then holds the installed version, as it also does once a keyword is removed from the configuration. Changing the value reinstalls the plugin, unless the new value names the installed version, as a keyword set on an imported plugin usually does; then only state changes.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					omittedVersionPlanModifier{},
				},
			},
			"installed_version": schema.StringAttribute{
				Description:         "The version Jellyfin lists for the plugin. While an update waits for a restart Jellyfin lists both versions, and this keeps the one it held before.",
				MarkdownDescription: "The version Jellyfin lists for the plugin. While an update waits for a restart Jellyfin lists both versions, and this keeps the one it held before.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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
				// An unset value is planned unknown when version changes in place;
				// replacing on it would reinstall the plugin.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
		},
	}
}

func (r *PluginResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
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
	// A configured keyword must come back from apply as written, so only an
	// omitted version takes the resolved one.
	if data.Version.IsNull() || data.Version.IsUnknown() {
		data.Version = types.StringValue(resolvedVersion)
	}

	installed, err := r.findInstalledPlugin(ctx, data.Name.ValueString(), resolvedVersion)
	if err != nil {
		resp.Diagnostics.AddError("Failed to check installed plugins", err.Error())
		return
	}

	if installed == nil {
		if err := r.client.InstallPlugin(ctx, data.Name.ValueString(), resolvedVersion, data.RepositoryURL.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to install plugin", err.Error()+notOfferedHint(err, data.Name.ValueString(), resolvedVersion))
			return
		}

		installed, err = r.waitForPlugin(ctx, data.Name.ValueString(), resolvedVersion, pluginInstallTimeout)
		if err != nil {
			resp.Diagnostics.AddError("Plugin install did not complete", err.Error())
			return
		}
	}

	r.recordCreated(*installed)
	data.ID = types.StringValue(installed.ID)
	data.InstalledVersion = types.StringValue(installed.Version)

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

	// State written before installed_version existed holds the installed
	// version in version.
	held := data.InstalledVersion.ValueString()
	if data.InstalledVersion.IsNull() || data.InstalledVersion.IsUnknown() {
		held = data.Version.ValueString()
	}
	if isPluginVersionKeyword(held) {
		held = ""
	}

	p, found := selectInstalledPlugin(plugins, data.ID.ValueString(), data.Name.ValueString(), held)
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	data.ID = types.StringValue(p.ID)
	data.Name = types.StringValue(p.Name)
	data.InstalledVersion = types.StringValue(p.Version)
	if !versionDescribes(data.Version, p.Version) {
		data.Version = types.StringValue(p.Version)
	}

	if data.RepositoryURL.IsNull() || data.RepositoryURL.ValueString() == "" {
		repoURL := r.resolveRepositoryURL(ctx, data.Name.ValueString(), p.Version)
		if repoURL != "" {
			data.RepositoryURL = types.StringValue(repoURL)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// ModifyPlan decides whether a change of version reinstalls the plugin, which
// needs the client to resolve a keyword.
func (r *PluginResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var plan, state PluginResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || plan.Version.Equal(state.Version) {
		return
	}
	installed := state.InstalledVersion.ValueString()
	if state.InstalledVersion.IsNull() && !isPluginVersionKeyword(state.Version.ValueString()) {
		installed = state.Version.ValueString()
	}
	keeps, err := r.versionNamesInstalled(ctx, plan.Name.ValueString(), plan.Version, installed)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("version"), "Failed to resolve plugin version", err.Error())
		return
	}
	if !keeps {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("version"))
	}
}

// versionNamesInstalled reports whether version, planned to replace another value, names the
// installed version; a keyword is resolved as an install would resolve it.
func (r *PluginResource) versionNamesInstalled(ctx context.Context, name string, version types.String, installed string) (bool, error) {
	if version.IsUnknown() || version.IsNull() || installed == "" {
		return false, nil
	}
	want := version.ValueString()
	if isPluginVersionKeyword(want) {
		if r.client == nil {
			return false, nil
		}
		resolved, err := r.resolvePluginVersion(ctx, name, version)
		if err != nil {
			return false, err
		}
		if resolved == "" {
			return false, fmt.Errorf("no package named %q found in configured repositories, so %q cannot be resolved; register the plugin repository first", name, want)
		}
		want = resolved
	}
	return samePluginVersion(installed, want), nil
}

// Update only runs for a change ModifyPlan found to name the installed
// version, so Jellyfin has nothing to change.
func (r *PluginResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state PluginResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.ID.IsUnknown() {
		plan.ID = state.ID
	}
	if plan.InstalledVersion.IsUnknown() {
		plan.InstalledVersion = state.InstalledVersion
	}
	if plan.RepositoryURL.IsUnknown() {
		plan.RepositoryURL = state.RepositoryURL
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PluginResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PluginResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	kept, err := r.uninstall(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to uninstall plugin", err.Error())
		return
	}
	if len(kept) > 0 {
		resp.Diagnostics.AddWarning(
			"Plugin left installed",
			fmt.Sprintf("Jellyfin bundles %s %s and does not let users uninstall it, so it stays installed; it is only removed from Terraform state.", kept[0].Name, strings.Join(pluginVersions(kept), ", ")),
		)
	}
}

// uninstall removes every listed version Jellyfin lets users uninstall, other than one a
// jellyfin_plugin created this run, and returns the bundled versions it cannot remove.
func (r *PluginResource) uninstall(ctx context.Context, id string) ([]client.InstalledPlugin, error) {
	listed, err := r.listedPlugins(ctx, id)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, p := range listed {
		if !p.CanUninstall {
			continue
		}
		if r.createdThisRun(p) {
			tflog.Debug(ctx, "Leaving the plugin version another jellyfin_plugin created in this run", map[string]interface{}{"plugin": p.Name, "version": p.Version})
			continue
		}
		if err := r.client.UninstallPluginVersion(ctx, p.ID, p.Version); err != nil {
			errs = append(errs, err)
		}
	}

	remaining, err := r.listedPlugins(ctx, id)
	if err != nil {
		return nil, errors.Join(append(errs, err)...)
	}
	var left, bundled []client.InstalledPlugin
	for _, p := range remaining {
		switch {
		case !p.CanUninstall:
			bundled = append(bundled, p)
		case !r.createdThisRun(p):
			left = append(left, p)
		}
	}
	if len(left) > 0 {
		return nil, errors.Join(append(errs, fmt.Errorf("plugin %s is still listed at version %s after uninstalling it", id, strings.Join(pluginVersions(left), ", ")))...)
	}
	return bundled, nil
}

// pluginVersionsCreated holds the plugin versions a jellyfin_plugin created or adopted,
// keyed by client (one Terraform run), so a destroy spares a create_before_destroy replacement.
var pluginVersionsCreated sync.Map

type createdPluginVersion struct {
	client  *client.Client
	id      string
	version string
}

func (r *PluginResource) recordCreated(p client.InstalledPlugin) {
	pluginVersionsCreated.Store(createdPluginVersion{r.client, normalizeGUID(p.ID), p.Version}, struct{}{})
}

func (r *PluginResource) createdThisRun(p client.InstalledPlugin) bool {
	_, ok := pluginVersionsCreated.Load(createdPluginVersion{r.client, normalizeGUID(p.ID), p.Version})
	return ok
}

func pluginVersions(plugins []client.InstalledPlugin) []string {
	versions := make([]string, len(plugins))
	for i, p := range plugins {
		versions[i] = p.Version
	}
	return versions
}

// selectInstalledPlugin returns the GET /Plugins entry for id (either GUID spelling) or name:
// the one at version, else the newest, skipping versions pending deletion.
func selectInstalledPlugin(plugins []client.InstalledPlugin, id, name, version string) (client.InstalledPlugin, bool) {
	var selected client.InstalledPlugin
	found := false
	for _, p := range plugins {
		if p.Status == pluginStatusDeleted || (normalizeGUID(p.ID) != normalizeGUID(id) && p.Name != name) {
			continue
		}
		if version != "" && samePluginVersion(p.Version, version) {
			return p, true
		}
		if !found || release.Compare(p.Version, selected.Version) > 0 {
			selected, found = p, true
		}
	}
	return selected, found
}

// ImportablePlugins returns, for each plugin that plugins, the answer of GET
// /Plugins, lists, the entry that a jellyfin_plugin imported by the plugin's
// ID reads: one entry per plugin, although Jellyfin lists each version it
// holds, and none for a plugin it deletes at the next restart.
func ImportablePlugins(plugins []client.InstalledPlugin) []client.InstalledPlugin {
	var out []client.InstalledPlugin
	seen := map[string]bool{}
	for _, p := range plugins {
		id := normalizeGUID(p.ID)
		if seen[id] {
			continue
		}
		seen[id] = true
		if selected, found := selectInstalledPlugin(plugins, p.ID, "", ""); found {
			out = append(out, selected)
		}
	}
	return out
}

// listedPlugins returns the entries GET /Plugins lists for the plugin with
// the given id, leaving out versions Jellyfin deletes at the next restart.
func (r *PluginResource) listedPlugins(ctx context.Context, id string) ([]client.InstalledPlugin, error) {
	plugins, err := r.client.GetInstalledPlugins(ctx)
	if err != nil {
		return nil, err
	}
	var listed []client.InstalledPlugin
	for _, p := range plugins {
		if normalizeGUID(p.ID) == normalizeGUID(id) && p.Status != pluginStatusDeleted {
			listed = append(listed, p)
		}
	}
	return listed, nil
}

// waitForPlugin blocks until name is installed at version and returns the
// entry GET /Plugins lists for it.
func (r *PluginResource) waitForPlugin(ctx context.Context, name, version string, timeout time.Duration) (*client.InstalledPlugin, error) {
	deadline := time.Now().Add(timeout)
	var seen string
	for time.Now().Before(deadline) {
		plugins, err := r.client.GetInstalledPlugins(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range plugins {
			if p.Name != name || p.Status == pluginStatusDeleted {
				continue
			}
			if samePluginVersion(p.Version, version) {
				return &p, nil
			}
			seen = p.Version
		}
		tflog.Debug(ctx, "Waiting for plugin to appear", map[string]interface{}{"plugin": name, "version": version, "seen": seen})
		if err := pause(ctx, min(pluginPollInterval, time.Until(deadline))); err != nil {
			return nil, err
		}
	}
	if seen != "" {
		return nil, fmt.Errorf("plugin %q is installed at %s but %s did not appear within %s", name, seen, version, timeout)
	}
	return nil, fmt.Errorf("plugin %q did not appear within %s", name, timeout)
}

// notOfferedHint explains the 404 Jellyfin answers an install with when none
// of its enabled repositories offers the package at that version for this
// server.
func notOfferedHint(err error, name, version string) string {
	if !client.IsNotFound(err) {
		return ""
	}
	return fmt.Sprintf("\n\nNo enabled plugin repository offers %s %s for this server's Jellyfin version. Check the version as the repository lists it (four parts, such as 13.0.0.0), and the repository_url.", name, version)
}

func (r *PluginResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// findInstalledPlugin returns the entry GET /Plugins lists for name at
// version, or nil if that version is not installed.
func (r *PluginResource) findInstalledPlugin(ctx context.Context, name, version string) (*client.InstalledPlugin, error) {
	plugins, err := r.client.GetInstalledPlugins(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing installed plugins: %w", err)
	}
	for _, p := range plugins {
		if p.Name == name && p.Status != pluginStatusDeleted && samePluginVersion(p.Version, version) {
			return &p, nil
		}
	}
	return nil, nil
}

// versionDescribes reports whether version in state still describes installed: a keyword,
// or the installed release in any spelling samePluginVersion accepts.
func versionDescribes(version types.String, installed string) bool {
	if version.IsNull() || version.IsUnknown() {
		return false
	}
	return isPluginVersionKeyword(version.ValueString()) || samePluginVersion(installed, version.ValueString())
}

// omittedVersionPlanModifier plans the installed version for a version the
// configuration no longer sets while state holds a keyword.
type omittedVersionPlanModifier struct{}

func (omittedVersionPlanModifier) Description(context.Context) string {
	return "Plans the installed version when the configuration drops a version keyword."
}

func (m omittedVersionPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (omittedVersionPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if !req.ConfigValue.IsNull() || req.StateValue.IsNull() || !isPluginVersionKeyword(req.StateValue.ValueString()) {
		return
	}
	var installed types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("installed_version"), &installed)...)
	if installed.IsNull() || installed.IsUnknown() || installed.ValueString() == "" {
		return
	}
	resp.PlanValue = installed
}

func isPluginVersionKeyword(version string) bool {
	return version == pluginVersionSupported || version == pluginVersionLatest
}

// samePluginVersion reports whether two plugin versions denote the same release, so
// 2.5.22.0 matches 2.5.22. An empty want matches anything.
func samePluginVersion(got, want string) bool {
	if want == "" {
		return true
	}
	return release.Compare(got, want) == 0
}

// resolveRepositoryURL finds the repository URL of the plugin name at
// version from the packages the server offers, or returns "".
func (r *PluginResource) resolveRepositoryURL(ctx context.Context, name, version string) string {
	pkgs, err := r.client.GetAvailablePackages(ctx)
	if err != nil {
		tflog.Debug(ctx, "Could not resolve repository URL for plugin (packages unavailable)", map[string]interface{}{
			"plugin": name,
			"error":  err.Error(),
		})
		return ""
	}
	repositoryURL := PluginRepositoryURL(pkgs, name, version)
	if repositoryURL == "" {
		tflog.Debug(ctx, "Could not resolve repository URL for plugin (exact version unavailable)", map[string]interface{}{
			"plugin":  name,
			"version": version,
		})
	}
	return repositoryURL
}

// PluginRepositoryURL returns the repository URL jellyfin_plugin reads for
// the plugin name at version: that of the first package pkgs lists with the
// name, for its entry at exactly that version, or "".
func PluginRepositoryURL(pkgs []client.PackageInfo, name, version string) string {
	for _, pkg := range pkgs {
		if pkg.Name != name {
			continue
		}
		for _, v := range pkg.Versions {
			if v.Version == version && v.RepositoryURL != "" {
				return v.RepositoryURL
			}
		}
		return ""
	}
	return ""
}

// resolvePluginVersion resolves "supported", "latest" or an unset version to the version to
// install, in the build this server is offered; any other value is used as-is.
func (r *PluginResource) resolvePluginVersion(ctx context.Context, name string, version types.String) (string, error) {
	supported := supportedVersionForPlugin(name)

	switch {
	case version.IsNull() || version.IsUnknown() || version.ValueString() == "":
		if supported != "" {
			return r.resolveSupportedBuild(ctx, name, supported)
		}
		return r.resolveLatestVersion(ctx, name)

	case version.ValueString() == pluginVersionSupported:
		if supported == "" {
			return "", fmt.Errorf("version %q is not available for plugin %q — no supported version is defined", pluginVersionSupported, name)
		}
		return r.resolveSupportedBuild(ctx, name, supported)

	case version.ValueString() == pluginVersionLatest:
		latest, err := r.resolveLatestVersion(ctx, name)
		if err != nil {
			return "", err
		}
		if supported != "" && latest != "" {
			if c := release.Compare(pluginRelease(latest), pluginRelease(supported)); c > 0 {
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

// resolveSupportedBuild returns the build of the supported release that this
// server is offered, or "" if the repositories do not offer the plugin.
func (r *PluginResource) resolveSupportedBuild(ctx context.Context, name, supported string) (string, error) {
	pkgs, err := r.client.GetAvailablePackages(ctx)
	if err != nil {
		return "", fmt.Errorf("listing available packages: %w", err)
	}
	for _, pkg := range pkgs {
		if pkg.Name == name {
			return pickReleaseBuild(pkg.Versions, supported), nil
		}
	}
	return "", nil
}

// pickReleaseBuild returns want when it is offered, otherwise the highest
// offered build of the same release, otherwise want.
func pickReleaseBuild(offered []client.VersionInfo, want string) string {
	best := ""
	for _, v := range offered {
		if v.Version == want {
			return want
		}
		if pluginRelease(v.Version) == pluginRelease(want) && (best == "" || release.Compare(v.Version, best) > 0) {
			best = v.Version
		}
	}
	if best == "" {
		return want
	}
	return best
}

// pluginRelease drops the build segment from a four-segment plugin version
// (2.6.3.1 → 2.6.3).
func pluginRelease(version string) string {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, ".")
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
