// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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

// Plugin statuses Jellyfin lists in GET /Plugins.
const (
	// pluginStatusActive is the status of a version Jellyfin has loaded.
	pluginStatusActive = "Active"
	// pluginStatusDeleted is the status of a version Jellyfin could not remove
	// from disk and deletes at the next restart instead.
	pluginStatusDeleted = "Deleted"
	// pluginStatusDisabled is the status of a version disabled on the
	// dashboard's Plugins page, which Jellyfin does not load.
	pluginStatusDisabled = "Disabled"
	// pluginStatusNotSupported is the status of a version built for another
	// Jellyfin version than the server's, which Jellyfin does not load.
	pluginStatusNotSupported = "NotSupported"
	// pluginStatusMalfunctioned is the status of a version that failed to
	// load.
	pluginStatusMalfunctioned = "Malfunctioned"
)

// usablePluginStatus reports whether a version with status s is one Jellyfin
// keeps and can load.
func usablePluginStatus(s string) bool {
	return s != pluginStatusDeleted && s != pluginStatusNotSupported && s != pluginStatusMalfunctioned
}

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
	Status           types.String `tfsdk:"status"`
	RepositoryURL    types.String `tfsdk:"repository_url"`
}

func (r *PluginResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin"
}

func (r *PluginResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Installs a plugin on the Jellyfin server. The server may require a restart after installation.\n\n" +
			"Jellyfin's Update Plugins scheduled task (key PluginUpdates) upgrades every plugin to the newest compatible version its repositories offer, at startup and every 24 hours by default. " +
			"While that task has at least one trigger, automatic updates are on and version is a minimum: any installed version at least as new satisfies it, and creating the resource adopts a newer installed version instead of installing an older one. " +
			"While the task has no triggers, or the server has no such task, version is exact, and the next apply reinstalls the configured version over any other. " +
			"The setting is global; the autoUpdate flag in a plugin's meta.json is not read. " +
			"Creating the resource never keeps a version Jellyfin lists as NotSupported or Malfunctioned: it installs the configured version, and fails if no enabled repository offers a build of it for this server.\n\n" +
			"To hold every plugin at its configured version, remove the task's triggers with jellyfin_scheduled_task. " +
			"Whenever a jellyfin_scheduled_task manages PluginUpdates, give every jellyfin_plugin depends_on = [jellyfin_scheduled_task.<name>], or put the depends_on on the module that holds the plugins. " +
			"Terraform then applies the schedule before the plugins and before any jellyfin_restart that depends on them, and an apply that removes the triggers also reinstalls each plugin the task moved past its version. " +
			"Without depends_on, a plan that turns updates on or off can fail with an error asking for it, terraform destroy included. " +
			"If the task resource itself depends on a plugin, as a for_each over tasks that plugins add can, manage PluginUpdates in a jellyfin_scheduled_task of its own. " +
			"Keep the task and the plugins on one provider configuration: across provider aliases the provider cannot see the planned schedule, so it plans a plugin against the server's current one, without a warning, until the next plan. " +
			"When a plan cannot know the schedule, as with triggers unknown until apply or -refresh=false, it warns, and the next plan corrects it.\n\n" +
			"Replacing the resource also replaces the resources that take its id, such as jellyfin_plugin_configuration, because the id is unknown until the plugin is installed again. They then write their configuration again, which Jellyfin accepts only once a restart has loaded the reinstalled plugin, so put a jellyfin_restart between them, as the jellyfin_security_plugin_configuration example does.",
		MarkdownDescription: "Installs a plugin on the Jellyfin server. The server may require a restart after installation.\n\n" +
			"Jellyfin's *Update Plugins* scheduled task (key `PluginUpdates`) upgrades every plugin to the newest compatible version its repositories offer, at startup and every 24 hours by default. " +
			"While that task has at least one trigger, automatic updates are on and `version` is a minimum: any installed version at least as new satisfies it, and creating the resource adopts a newer installed version instead of installing an older one. " +
			"While the task has no triggers, or the server has no such task, `version` is exact, and the next apply reinstalls the configured version over any other. " +
			"The setting is global; the `autoUpdate` flag in a plugin's `meta.json` is not read. " +
			"Creating the resource never keeps a version Jellyfin lists as `NotSupported` or `Malfunctioned`: it installs the configured version, and fails if no enabled repository offers a build of it for this server.\n\n" +
			"To hold every plugin at its configured version, remove the task's triggers with `jellyfin_scheduled_task`. " +
			"Whenever a `jellyfin_scheduled_task` manages `PluginUpdates`, give every `jellyfin_plugin` `depends_on = [jellyfin_scheduled_task.<name>]`, or put the `depends_on` on the module that holds the plugins. " +
			"Terraform then applies the schedule before the plugins and before any `jellyfin_restart` that depends on them, and an apply that removes the triggers also reinstalls each plugin the task moved past its `version`. " +
			"Without `depends_on`, a plan that turns updates on or off can fail with an error asking for it, `terraform destroy` included. " +
			"If the task resource itself depends on a plugin, as a `for_each` over tasks that plugins add can, manage `PluginUpdates` in a `jellyfin_scheduled_task` of its own. " +
			"Keep the task and the plugins on one provider configuration: across provider aliases the provider cannot see the planned schedule, so it plans a plugin against the server's current one, without a warning, until the next plan. " +
			"When a plan cannot know the schedule, as with triggers unknown until apply or `-refresh=false`, it warns, and the next plan corrects it.\n\n" +
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
				Description:         "The version to install, as the repository lists it (e.g. 13.0.0.0), or latest (the newest offered) or supported (the Jellyfin Security release this provider was tested against). A keyword is resolved only at install or when set, stays in state as written, and accepts any version installed later. Omitted, it installs as supported for Jellyfin Security and latest otherwise, then tracks installed_version, as it also does after a keyword is removed. A version is a minimum while automatic plugin updates are on, else exact; see the resource description. Changing it reinstalls the plugin, replacing the resource, unless the installed version already satisfies the new value.",
				MarkdownDescription: "The version to install, as the repository lists it (e.g. `13.0.0.0`), or `latest` (the newest offered) or `supported` (the Jellyfin Security release this provider was tested against). A keyword is resolved only at install or when set, stays in state as written, and accepts any version installed later. Omitted, it installs as `supported` for Jellyfin Security and `latest` otherwise, then tracks `installed_version`, as it also does after a keyword is removed. A version is a minimum while automatic plugin updates are on, else exact; see the resource description. Changing it reinstalls the plugin, replacing the resource, unless the installed version already satisfies the new value.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					omittedVersionPlanModifier{},
				},
			},
			"installed_version": schema.StringAttribute{
				Description:         "The version Jellyfin lists for the plugin, which can be newer than version while automatic plugin updates are on. While an update waits for a restart, this keeps the previous version, whose status reads Superseded. A version Jellyfin deletes at the next restart is ignored.",
				MarkdownDescription: "The version Jellyfin lists for the plugin, which can be newer than `version` while automatic plugin updates are on. While an update waits for a restart, this keeps the previous version, whose `status` reads `Superseded`. A version Jellyfin deletes at the next restart is ignored.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Description:         "The status Jellyfin lists for installed_version: Active (loaded), Restart (waiting for a restart to load), Superseded (a newer version waits for a restart to replace it), Disabled (disabled on the dashboard's Plugins page), NotSupported (built for another Jellyfin version, so not loaded) or Malfunctioned (failed to load; the server log says why). Creating the resource fails if the version it installed is NotSupported or Malfunctioned, and the next apply replaces the tainted plugin.",
				MarkdownDescription: "The status Jellyfin lists for `installed_version`: `Active` (loaded), `Restart` (waiting for a restart to load), `Superseded` (a newer version waits for a restart to replace it), `Disabled` (disabled on the dashboard's Plugins page), `NotSupported` (built for another Jellyfin version, so not loaded) or `Malfunctioned` (failed to load; the server log says why). Creating the resource fails if the version it installed is `NotSupported` or `Malfunctioned`, and the next apply replaces the tainted plugin.",
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

	if data.RepositoryURL.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing plugin repository URL",
			"The repository_url attribute must be set when installing a plugin so the provider can reproduce the install source.",
		)
		return
	}
	pinned := !data.Version.IsNull() && !data.Version.IsUnknown()
	resolvedVersion, err := r.resolvePluginVersion(ctx, data.Name.ValueString(), data.Version, &resp.Diagnostics)
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

	plugins, err := r.client.GetInstalledPlugins(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to check installed plugins", fmt.Sprintf("listing installed plugins: %s", err))
		return
	}
	// The schedule decides whether a newer installed version is adopted, and
	// private state keeps it for a plan that does not refresh. Create reads it
	// from the server and not from the walk's shared read, because an apply of
	// a jellyfin_scheduled_task in this walk can change it.
	schedule, err := pluginUpdatesOn(ctx, r.client)
	if err != nil {
		scheduleReadError(&resp.Diagnostics, err)
		return
	}
	installed, adopted := installedToKeep(plugins, data.Name.ValueString(), resolvedVersion, schedule)

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

	// A configured keyword must come back from apply as written, so only an
	// omitted version takes the resolved one, or the newer one adopted.
	if !pinned {
		data.Version = types.StringValue(resolvedVersion)
		if adopted {
			data.Version = types.StringValue(installed.Version)
		}
	}

	// A create_before_destroy replacement's destroy spares a version created
	// this run, so it would never remove one Jellyfin cannot load.
	if usablePluginStatus(installed.Status) {
		r.recordCreated(*installed)
	}
	data.ID = types.StringValue(installed.ID)
	data.InstalledVersion = types.StringValue(installed.Version)
	data.Status = types.StringValue(installed.Status)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(storeVersionPinned(ctx, resp.Private, pinned)...)
	resp.Diagnostics.Append(storeAutoUpdate(ctx, resp.Private, schedule)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The state is set, so Terraform keeps the plugin as tainted and replaces
	// it at the next apply.
	if detail := unloadablePluginDetail(*installed); detail != "" {
		resp.Diagnostics.AddError("Plugin cannot be loaded", detail)
	}
}

// installedToKeep returns the GET /Plugins entry of name that Create keeps
// instead of installing version, or nil to install it: the entry at version
// if Jellyfin can load it, else, while schedule has automatic plugin updates
// on, the newest newer entry Jellyfin can load, which adopted reports. An
// entry Jellyfin cannot load is never kept, so the install replaces it.
func installedToKeep(plugins []client.InstalledPlugin, name, version string, schedule autoUpdate) (keep *client.InstalledPlugin, adopted bool) {
	var newest *client.InstalledPlugin
	for i := range plugins {
		p := &plugins[i]
		switch {
		case p.Name != name || !usablePluginStatus(p.Status):
		case samePluginVersion(p.Version, version):
			return p, false
		case release.Compare(p.Version, version) > 0 && (newest == nil || release.Compare(p.Version, newest.Version) > 0):
			newest = p
		}
	}
	if newest != nil && schedule.minimum() {
		return newest, true
	}
	return nil, false
}

// unloadablePluginDetail explains why Jellyfin does not load p, the version
// Create installed, or returns "" when its status does not say so.
func unloadablePluginDetail(p client.InstalledPlugin) string {
	// Create never keeps a version Jellyfin cannot load, so the replacement
	// installs the configured version again whichever order it runs in.
	const next = "Terraform keeps the plugin as tainted, and the next apply installs the configured version again, after uninstalling this one unless the resource sets create_before_destroy"
	switch p.Status {
	case pluginStatusNotSupported:
		return fmt.Sprintf("Jellyfin lists %s %s with the status NotSupported: it targets another Jellyfin version than this server runs, so Jellyfin does not load it. "+
			next+"; set version to a build for this server.", p.Name, p.Version)
	case pluginStatusMalfunctioned:
		return fmt.Sprintf("Jellyfin lists %s %s with the status Malfunctioned: Jellyfin failed to load it, and the server log names the error. "+
			next+".", p.Name, p.Version)
	}
	return ""
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

	p, found := selectInstalledPlugin(plugins, data.ID.ValueString(), data.Name.ValueString(), heldVersion(data))
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	schedule, ok := r.scheduleForRead(ctx, p, &resp.Diagnostics)
	if !ok {
		return
	}
	pinned, diags := storedVersionPinned(ctx, req.Private, data.Version)
	resp.Diagnostics.Append(diags...)

	data.ID = types.StringValue(p.ID)
	data.Name = types.StringValue(p.Name)
	data.InstalledVersion = types.StringValue(p.Version)
	data.Status = types.StringValue(p.Status)
	// Read cannot see the configuration, so a version it only holds because
	// it was omitted follows the installed one even while updates are on.
	if !versionDescribes(data.Version, p.Version, schedule.minimum() && pinned) {
		data.Version = types.StringValue(p.Version)
	}

	if data.RepositoryURL.ValueString() == "" {
		repoURL := r.resolveRepositoryURL(ctx, data.Name.ValueString(), p.Version)
		if repoURL != "" {
			data.RepositoryURL = types.StringValue(repoURL)
		}
	}

	resp.Diagnostics.Append(storeAutoUpdate(ctx, resp.Private, schedule)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// scheduleForRead returns the Update Plugins schedule a refresh of p reads
// version against: the one a jellyfin_scheduled_task planned in this walk,
// else the server's.
func (r *PluginResource) scheduleForRead(ctx context.Context, p client.InstalledPlugin, diags *diag.Diagnostics) (autoUpdate, bool) {
	walk := autoUpdateWalkFor(r.client)
	id := normalizeGUID(p.ID)
	if planned, ok := walk.plannedFor(id, p.Name); ok {
		return planned, true
	}
	// decided takes a schedule a task planned while the server answered over
	// the server's.
	server, err := walk.serverSchedule(ctx, r.client)
	if err != nil {
		scheduleReadError(diags, err)
		return "", false
	}
	return walk.decided(id, p.Name, server), true
}

// ModifyPlan decides whether a change of version reinstalls the plugin, from
// the Update Plugins schedule the refresh read version against, and checks
// that schedule against one a jellyfin_scheduled_task plans in this walk. It
// reads neither the schedule nor the walk's plan to decide, so the apply's
// plan matches the plan's.
func (r *PluginResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var configured, configuredRepository types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("version"), &configured)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("repository_url"), &configuredRepository)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(storeVersionPinned(ctx, resp.Private, !configured.IsNull())...)
	// Create reads the schedule itself.
	if req.State.Raw.IsNull() {
		return
	}

	var plan, state PluginResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	schedule, recorded, diags := storedAutoUpdate(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	installed := heldVersion(state)
	required, known := r.requiredVersion(ctx, plan.Name.ValueString(), plan.Version, state.Version, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Version.Equal(state.Version) {
		keeps := known && installed != ""
		if schedule.minimum() {
			keeps = keeps && release.Compare(installed, required) >= 0
		} else {
			keeps = keeps && samePluginVersion(installed, required)
		}
		if !keeps {
			resp.RequiresReplace = append(resp.RequiresReplace, path.Root("version"))
		}
	}

	// Only an installed version newer than the required one is kept with
	// updates on and reinstalled with them off, unless the plan replaces the
	// plugin anyway: name and repository_url replace it as their plan
	// modifiers do, which the framework does not tell ModifyPlan. A
	// replacement -replace asks for is not seen at all; a tainted plugin is
	// planned with a null state and never gets here.
	replaced := !plan.Name.Equal(state.Name) || (!configuredRepository.IsNull() && !configuredRepository.Equal(state.RepositoryURL))
	sensitive := !replaced && !configured.IsNull() && known && release.Compare(installed, required) > 0
	if r.client != nil {
		resp.Diagnostics.Append(autoUpdateWalkFor(r.client).checkPlugin(normalizeGUID(state.ID.ValueString()), pluginAutoUpdate{
			name: plan.Name.ValueString(), used: schedule, recorded: recorded, sensitive: sensitive, installed: installed, required: required,
		})...)
	}
}

// heldVersion returns "" when state records none. State written before
// installed_version existed holds the installed version in version.
func heldVersion(state PluginResourceModel) string {
	held := state.InstalledVersion.ValueString()
	if state.InstalledVersion.IsNull() || state.InstalledVersion.IsUnknown() {
		held = state.Version.ValueString()
	}
	if isPluginVersionKeyword(held) {
		return ""
	}
	return held
}

// requiredVersion returns the version planned requires and whether there is
// one: planned itself, or the version a keyword that replaces prior resolves
// to as an install would resolve it. A keyword kept from prior, or a version
// unknown until apply, requires none.
func (r *PluginResource) requiredVersion(ctx context.Context, name string, planned, prior types.String, diags *diag.Diagnostics) (string, bool) {
	if planned.IsUnknown() || planned.IsNull() {
		return "", false
	}
	want := planned.ValueString()
	if !isPluginVersionKeyword(want) {
		return want, true
	}
	if planned.Equal(prior) || r.client == nil {
		return "", false
	}
	resolved, err := r.resolvePluginVersion(ctx, name, planned, diags)
	if err == nil && resolved == "" {
		err = fmt.Errorf("no package named %q found in configured repositories, so %q cannot be resolved; register the plugin repository first", name, want)
	}
	if err != nil {
		diags.AddAttributeError(path.Root("version"), "Failed to resolve plugin version", err.Error())
		return "", false
	}
	return resolved, true
}

// Update only runs for a change ModifyPlan found the installed version to
// satisfy, so Jellyfin has nothing to change. The planned private state
// already holds whether the configuration sets version.
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
	if plan.Status.IsUnknown() {
		plan.Status = state.Status
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

func (r *PluginResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
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
			tflog.Debug(ctx, "Leaving the plugin version another jellyfin_plugin created in this run", map[string]any{"plugin": p.Name, "version": p.Version})
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
// keyed by client, which the provider configures anew for each Terraform walk, so a
// destroy spares a create_before_destroy replacement.
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
// the one at version, preferring one Jellyfin can load, as a reinstall next to one it
// cannot is, else the newest, skipping versions pending deletion.
func selectInstalledPlugin(plugins []client.InstalledPlugin, id, name, version string) (client.InstalledPlugin, bool) {
	var selected, atVersion client.InstalledPlugin
	found, foundAtVersion := false, false
	for _, p := range plugins {
		switch {
		case p.Status == pluginStatusDeleted || (normalizeGUID(p.ID) != normalizeGUID(id) && p.Name != name):
		case version != "" && samePluginVersion(p.Version, version):
			if usablePluginStatus(p.Status) {
				return p, true
			}
			if !foundAtVersion {
				atVersion, foundAtVersion = p, true
			}
		case !found || release.Compare(p.Version, selected.Version) > 0:
			selected, found = p, true
		}
	}
	if foundAtVersion {
		return atVersion, true
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
// entry GET /Plugins lists for it, preferring one Jellyfin can load over one
// listed at the same version before the install.
func (r *PluginResource) waitForPlugin(ctx context.Context, name, version string, timeout time.Duration) (*client.InstalledPlugin, error) {
	deadline := time.Now().Add(timeout)
	var seen string
	for time.Now().Before(deadline) {
		plugins, err := r.client.GetInstalledPlugins(ctx)
		if err != nil {
			return nil, err
		}
		var unloadable *client.InstalledPlugin
		for _, p := range plugins {
			switch {
			case p.Name != name || p.Status == pluginStatusDeleted:
			case !samePluginVersion(p.Version, version):
				seen = p.Version
			case usablePluginStatus(p.Status):
				return &p, nil
			case unloadable == nil:
				unloadable = &p
			}
		}
		if unloadable != nil {
			return unloadable, nil
		}
		tflog.Debug(ctx, "Waiting for plugin to appear", map[string]any{"plugin": name, "version": version, "seen": seen})
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

// versionDescribes reports whether version in state still describes installed: a keyword,
// the installed release in any spelling samePluginVersion accepts, or, as a
// minimum, any release up to it.
func versionDescribes(version types.String, installed string, minimum bool) bool {
	if version.IsNull() || version.IsUnknown() {
		return false
	}
	v := version.ValueString()
	switch {
	case isPluginVersionKeyword(v):
		return true
	case minimum:
		return release.Compare(installed, v) >= 0
	}
	return samePluginVersion(installed, v)
}

// omittedVersionPlanModifier plans the installed version for a version the
// configuration no longer sets while state holds a keyword, or a pinned
// version that the installed one, updated while it was a minimum, has passed.
type omittedVersionPlanModifier struct{}

func (omittedVersionPlanModifier) Description(context.Context) string {
	return "Plans the installed version when the configuration drops a version keyword, or a pinned version the installed one has passed."
}

func (m omittedVersionPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (omittedVersionPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if !req.ConfigValue.IsNull() || req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	var installed types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("installed_version"), &installed)...)
	if installed.ValueString() == "" {
		return
	}
	if held := req.StateValue.ValueString(); isPluginVersionKeyword(held) || !samePluginVersion(held, installed.ValueString()) {
		resp.PlanValue = installed
	}
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
		tflog.Debug(ctx, "Could not resolve repository URL for plugin (packages unavailable)", map[string]any{
			"plugin": name,
			"error":  err.Error(),
		})
		return ""
	}
	repositoryURL := PluginRepositoryURL(pkgs, name, version)
	if repositoryURL == "" {
		tflog.Debug(ctx, "Could not resolve repository URL for plugin (exact version unavailable)", map[string]any{
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
// install, in the build this server is offered; any other value is used as-is. It warns
// in diags when latest resolves past the supported release.
func (r *PluginResource) resolvePluginVersion(ctx context.Context, name string, version types.String, diags *diag.Diagnostics) (string, error) {
	supported := supportedVersionForPlugin(name)

	switch {
	case version.ValueString() == "":
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
		if supported != "" && latest != "" && release.Compare(pluginRelease(latest), pluginRelease(supported)) > 0 {
			diags.AddAttributeWarning(path.Root("version"), "Plugin version newer than supported", fmt.Sprintf(
				"latest resolves %s to %s, which is newer than %s, the release this provider was tested against, so the typed Terraform resource for its configuration may not cover every setting of this version. "+
					"Set version to supported to install the tested release.", name, latest, supported))
		}
		return latest, nil

	default:
		return version.ValueString(), nil
	}
}

// availablePackage returns the package named name, or nil if none is offered.
func (r *PluginResource) availablePackage(ctx context.Context, name string) (*client.PackageInfo, error) {
	pkgs, err := r.client.GetAvailablePackages(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing available packages: %w", err)
	}
	i := slices.IndexFunc(pkgs, func(pkg client.PackageInfo) bool { return pkg.Name == name })
	if i < 0 {
		return nil, nil
	}
	return &pkgs[i], nil
}

// resolveLatestVersion returns "" if the repositories do not offer the plugin.
func (r *PluginResource) resolveLatestVersion(ctx context.Context, name string) (string, error) {
	pkg, err := r.availablePackage(ctx, name)
	switch {
	case err != nil || pkg == nil:
		return "", err
	case len(pkg.Versions) == 0:
		return "", fmt.Errorf("plugin %q has no available versions", name)
	}
	// Manifests list newest version first.
	return pkg.Versions[0].Version, nil
}

// resolveSupportedBuild returns the build of the supported release that this
// server is offered, or "" if the repositories do not offer the plugin.
func (r *PluginResource) resolveSupportedBuild(ctx context.Context, name, supported string) (string, error) {
	pkg, err := r.availablePackage(ctx, name)
	if err != nil || pkg == nil {
		return "", err
	}
	return pickReleaseBuild(pkg.Versions, supported), nil
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
