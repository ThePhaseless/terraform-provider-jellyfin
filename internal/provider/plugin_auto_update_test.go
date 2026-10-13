// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

// initPrivate gives a request or response the empty private state the
// framework starts it with; the framework's type is internal, so only
// inference can name it.
func initPrivate[T any](p **T) { *p = new(T) }

// privateKey is a key of private state with its JSON value.
type privateKey struct{ key, value string }

func autoUpdateKey(a autoUpdate) privateKey {
	return privateKey{privateAutoUpdate, strconv.Quote(string(a))}
}

func pinnedKey(pinned bool) privateKey {
	return privateKey{privateVersionPinned, strconv.FormatBool(pinned)}
}

func setPrivate(t *testing.T, private privateSetter, keys ...privateKey) {
	t.Helper()
	for _, k := range keys {
		if d := private.SetKey(t.Context(), k.key, []byte(k.value)); d.HasError() {
			t.Fatalf("setting %s: %v", k.key, d)
		}
	}
}

// privateValue returns the JSON private holds at key, or "" for none.
func privateValue(t *testing.T, private privateGetter, key string) string {
	t.Helper()
	raw, d := private.GetKey(t.Context(), key)
	if d.HasError() {
		t.Fatalf("getting %s: %v", key, d)
	}
	return string(raw)
}

// privateKeys returns the keys of jellyfin_plugin that private holds.
func privateKeys(t *testing.T, private privateGetter) []privateKey {
	t.Helper()
	var keys []privateKey
	for _, key := range []string{privateAutoUpdate, privateVersionPinned} {
		if v := privateValue(t, private, key); v != "" {
			keys = append(keys, privateKey{key, v})
		}
	}
	return keys
}

// described lists diags as "<severity> <summary> at <path>", in order.
func described(diags diag.Diagnostics) []string {
	var out []string
	for _, d := range diags {
		s := strings.ToLower(d.Severity().String()) + " " + d.Summary()
		if p, ok := d.(diag.DiagnosticWithPath); ok {
			s += " at " + p.Path().String()
		}
		out = append(out, s)
	}
	return out
}

func bookshelfAt(version, status string) client.InstalledPlugin {
	return client.InstalledPlugin{ID: bookshelfID, Name: "Bookshelf", Version: version, Status: status, CanUninstall: true}
}

func pluginModel(t *testing.T, s tfsdk.State) PluginResourceModel {
	t.Helper()
	var m PluginResourceModel
	if d := s.Get(t.Context(), &m); d.HasError() {
		t.Fatalf("state: %v", d)
	}
	return m
}

// readPlugin refreshes state with private as its prior private state, which
// the response shares, as in the framework.
func readPlugin(t *testing.T, r *PluginResource, state PluginResourceModel, private ...privateKey) *resource.ReadResponse {
	t.Helper()
	req := resource.ReadRequest{State: pluginResourceState(t, state)}
	initPrivate(&req.Private)
	setPrivate(t, req.Private, private...)
	resp := &resource.ReadResponse{State: req.State, Private: req.Private}
	r.Read(t.Context(), req, resp)
	return resp
}

// refreshAndPlan refreshes state and plans the refreshed state for a
// configuration that sets version to configured, as a refreshing plan does
// for one jellyfin_plugin, and returns the refreshed state and the plan.
func refreshAndPlan(t *testing.T, r *PluginResource, state PluginResourceModel, configured types.String, private ...privateKey) (PluginResourceModel, *resource.ModifyPlanResponse) {
	t.Helper()
	read := readPlugin(t, r, state, private...)
	if read.Diagnostics.HasError() || read.State.Raw.IsNull() {
		t.Fatalf("Read: %v, state %v", read.Diagnostics, read.State.Raw)
	}
	refreshed := pluginModel(t, read.State)
	plan := refreshed
	if !configured.IsNull() {
		plan.Version = configured
	}
	return refreshed, planPlugin(t, r, &refreshed, &plan, configured, privateKeys(t, read.Private)...)
}

func createPlugin(t *testing.T, r *PluginResource, plan PluginResourceModel) *resource.CreateResponse {
	t.Helper()
	resp := &resource.CreateResponse{State: pluginResourceNullState(t.Context())}
	initPrivate(&resp.Private)
	r.Create(t.Context(), resource.CreateRequest{Plan: pluginResourcePlan(t, plan)}, resp)
	return resp
}

// Planned trigger lists of the PluginUpdates task.
func updateTriggersOn(t *testing.T) types.List  { return triggerList(t, newTrigger(triggerTypeStartup)) }
func updateTriggersOff(t *testing.T) types.List { return triggerList(t) }
func updateTriggersUnknown(t *testing.T) types.List {
	return types.ListUnknown(scheduledTaskTriggerType(t))
}

// planPluginUpdates plans triggers for the PluginUpdates task through c, as a
// jellyfin_scheduled_task configured with its key does.
func planPluginUpdates(t *testing.T, c *client.Client, triggers types.List) resource.ModifyPlanResponse {
	t.Helper()
	state := storedTask(updatePluginsID, types.StringValue(PluginUpdatesTaskKey))
	config := keyConfig(PluginUpdatesTaskKey)
	config.Triggers = triggers
	plan := *state
	plan.Triggers = triggers
	_, resp := planScheduledTask(t, c, config, state, plan)
	return resp
}

func TestUnitPluginUpdatesOn(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body   string
		status int
		want   autoUpdate
	}{
		"a trigger":             {body: `[{"Id":"a","Key":"PluginUpdates","Triggers":[{"Type":"StartupTrigger"}]}]`, want: autoUpdateOn},
		"no triggers":           {body: `[{"Id":"a","Key":"PluginUpdates","Triggers":[]}]`, want: autoUpdateOff},
		"null triggers":         {body: `[{"Id":"a","Key":"PluginUpdates","Triggers":null}]`, want: autoUpdateOff},
		"no PluginUpdates task": {body: `[{"Id":"b","Key":"RefreshLibrary","Triggers":[{"Type":"StartupTrigger"}]}]`, want: autoUpdateOff},
		"a server error":        {status: http.StatusInternalServerError},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.status != 0 {
					w.WriteHeader(tc.status)
					return
				}
				fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(srv.Close)

			got, err := pluginUpdatesOn(t.Context(), client.NewClient(srv.URL, "k"))
			if (err != nil) != (tc.status != 0) || got != tc.want {
				t.Errorf("pluginUpdatesOn() = %q, %v; want %q and an error %t", got, err, tc.want, tc.status != 0)
			}
		})
	}
}

// Jellyfin's Update Plugins task upgrades a pinned plugin. While it has
// triggers the pinned version is a minimum the upgrade satisfies, so the
// refresh keeps it, but only for a version the configuration sets: Read
// cannot see the configuration, so it follows the stored flag, and without
// one counts a concrete version in state as set.
func TestUnitPluginReadKeepsAPinnedVersionAnUpdateExceeds(t *testing.T) {
	t.Parallel()

	active := func(version string) []client.InstalledPlugin {
		return []client.InstalledPlugin{bookshelfAt(version, "Active")}
	}
	tests := []struct {
		name                                   string
		triggers                               []json.RawMessage
		private                                []privateKey
		version                                string
		listed                                 []client.InstalledPlugin
		wantVersion, wantInstalled, wantStatus string
		wantAutoUpdate                         autoUpdate
	}{
		{"updated past the pin while updates are on", updatesOn, []privateKey{pinnedKey(true)}, "12.0.0.0", active("13.0.0.0"), "12.0.0.0", "13.0.0.0", "Active", autoUpdateOn},
		{"updated past the pin while updates are off", nil, []privateKey{pinnedKey(true)}, "12.0.0.0", active("13.0.0.0"), "13.0.0.0", "13.0.0.0", "Active", autoUpdateOff},
		{"updated past a version the configuration omits", updatesOn, []privateKey{pinnedKey(false)}, "12.0.0.0", active("13.0.0.0"), "13.0.0.0", "13.0.0.0", "Active", autoUpdateOn},
		{"updated past a version no flag is stored for", updatesOn, nil, "12.0.0.0", active("13.0.0.0"), "12.0.0.0", "13.0.0.0", "Active", autoUpdateOn},
		{"updated past a version no flag is stored for while updates are off", nil, nil, "12.0.0.0", active("13.0.0.0"), "13.0.0.0", "13.0.0.0", "Active", autoUpdateOff},
		{"updated past a version with an unreadable flag", updatesOn, []privateKey{{privateVersionPinned, `"yes"`}}, "12.0.0.0", active("13.0.0.0"), "12.0.0.0", "13.0.0.0", "Active", autoUpdateOn},
		{"keyword no flag is stored for", updatesOn, nil, "latest", active("13.0.0.0"), "latest", "13.0.0.0", "Active", autoUpdateOn},
		{"downgraded below the pin while updates are on", updatesOn, []privateKey{pinnedKey(true)}, "12.0.0.0", active("11.0.0.0"), "11.0.0.0", "11.0.0.0", "Active", autoUpdateOn},
		{"keyword while updates are on", updatesOn, []privateKey{pinnedKey(true)}, "latest", active("13.0.0.0"), "latest", "13.0.0.0", "Active", autoUpdateOn},
		{"keyword while updates are off", nil, []privateKey{pinnedKey(true)}, "latest", active("13.0.0.0"), "latest", "13.0.0.0", "Active", autoUpdateOff},
		{
			"the held version while an update waits for a restart", updatesOn, []privateKey{pinnedKey(true)}, "12.0.0.0",
			[]client.InstalledPlugin{bookshelfAt("12.0.0.0", "Superceded"), bookshelfAt("13.0.0.0", "Restart")},
			"12.0.0.0", "12.0.0.0", "Superceded", autoUpdateOn,
		},
		{
			"a reinstall listed next to the version Jellyfin could not load", updatesOn, []privateKey{pinnedKey(true)}, "12.0.0.0",
			[]client.InstalledPlugin{bookshelfAt("12.0.0.0", pluginStatusMalfunctioned), bookshelfAt("12.0.0.0", "Restart")},
			"12.0.0.0", "12.0.0.0", "Restart", autoUpdateOn,
		},
		{
			"only a version Jellyfin could not load", updatesOn, []privateKey{pinnedKey(true)}, "12.0.0.0",
			[]client.InstalledPlugin{bookshelfAt("12.0.0.0", pluginStatusNotSupported)},
			"12.0.0.0", "12.0.0.0", pluginStatusNotSupported, autoUpdateOn,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := newFakePluginResource(t, &fakePluginServer{plugins: tc.listed, pluginUpdateTriggers: tc.triggers})
			resp := readPlugin(t, r, installedBookshelf(tc.version, "12.0.0.0"), tc.private...)
			if resp.Diagnostics.HasError() {
				t.Fatalf("Read: %v", resp.Diagnostics)
			}
			got := pluginModel(t, resp.State)
			if got.Version.ValueString() != tc.wantVersion || got.InstalledVersion.ValueString() != tc.wantInstalled || got.Status.ValueString() != tc.wantStatus {
				t.Errorf("read version %s, installed_version %s, status %s; want %s, %s, %s", got.Version, got.InstalledVersion, got.Status, tc.wantVersion, tc.wantInstalled, tc.wantStatus)
			}
			if got, want := privateValue(t, resp.Private, privateAutoUpdate), strconv.Quote(string(tc.wantAutoUpdate)); got != want {
				t.Errorf("stored auto_update %s, want %s", got, want)
			}
		})
	}
}

func TestUnitPluginReadFailsWhenTheScheduleCannotBeRead(t *testing.T) {
	t.Parallel()

	r := newFakePluginResource(t, &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("13.0.0.0", "Active")}, tasksStatus: http.StatusInternalServerError})
	resp := readPlugin(t, r, installedBookshelf("12.0.0.0", "12.0.0.0"), pinnedKey(true))
	if got, want := described(resp.Diagnostics), []string{"error Failed to read the Update Plugins schedule"}; !slices.Equal(got, want) {
		t.Errorf("Read diagnostics %q, want %q", got, want)
	}
}

// A jellyfin_scheduled_task that the plugin depends on plans first, so the
// refresh reads version against the schedule the apply sets, not the
// server's.
func TestUnitPluginReadUsesThePlannedSchedule(t *testing.T) {
	t.Parallel()

	fake := &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("13.0.0.0", "Active")}, pluginUpdateTriggers: updatesOn}
	r := newFakePluginResource(t, fake)
	if task := planPluginUpdates(t, r.client, updateTriggersOff(t)); task.Diagnostics.HasError() {
		t.Fatalf("planning PluginUpdates: %v", task.Diagnostics)
	}
	before := fake.scheduledTaskRequests()

	resp := readPlugin(t, r, installedBookshelf("12.0.0.0", "12.0.0.0"), pinnedKey(true))
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics)
	}
	if got := pluginModel(t, resp.State).Version.ValueString(); got != "13.0.0.0" {
		t.Errorf("read version %s, want the installed 13.0.0.0 that off reads back", got)
	}
	if got := privateValue(t, resp.Private, privateAutoUpdate); got != `"off"` {
		t.Errorf("stored auto_update %s, want \"off\"", got)
	}
	if n := fake.scheduledTaskRequests() - before; n != 0 {
		t.Errorf("Read sent %d GET /ScheduledTasks, want none", n)
	}
}

// A task that plans while the refresh reads the server's schedule is not
// missed: Read takes the planned schedule over the one it read.
func TestUnitPluginReadRechecksThePlanAfterReadingTheSchedule(t *testing.T) {
	t.Parallel()

	fake := &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("13.0.0.0", "Active")}, pluginUpdateTriggers: updatesOn}
	r := newFakePluginResource(t, fake)
	fake.setTasksHook(func() {
		if d := autoUpdateWalkFor(r.client).planSchedule(autoUpdateOff); d.HasError() {
			t.Errorf("planSchedule: %v", d)
		}
	})

	resp := readPlugin(t, r, installedBookshelf("12.0.0.0", "12.0.0.0"), pinnedKey(true))
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics)
	}
	if got := pluginModel(t, resp.State).Version.ValueString(); got != "13.0.0.0" {
		t.Errorf("read version %s, want the installed 13.0.0.0 that off reads back", got)
	}
	if got := privateValue(t, resp.Private, privateAutoUpdate); got != `"off"` {
		t.Errorf("stored auto_update %s, want \"off\"", got)
	}
}

// A plan that changes nothing stores no private state, so an imported
// plugin, or one whose state an earlier provider version wrote, keeps no
// version_pinned. Its concrete version then counts as pinned, which keeps the
// version jellyfin-import writes, while one the configuration omits moves to
// the installed version in place once, which stores false.
func TestUnitPluginReadCountsAVersionInStateAsPinnedUntilAPlanStoresIt(t *testing.T) {
	t.Parallel()

	fake := &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("12.0.0.0", "Active")}, pluginUpdateTriggers: updatesOn}
	r := newFakePluginResource(t, fake)
	imported := PluginResourceModel{
		ID: types.StringValue(bookshelfID), Name: types.StringValue("Bookshelf"), Version: types.StringNull(),
		InstalledVersion: types.StringNull(), Status: types.StringNull(), RepositoryURL: types.StringNull(),
	}
	read := readPlugin(t, r, imported)
	if read.Diagnostics.HasError() {
		t.Fatalf("import Read: %v", read.Diagnostics)
	}
	state := pluginModel(t, read.State)
	if state.Version.ValueString() != "12.0.0.0" {
		t.Fatalf("imported version %s, want the installed 12.0.0.0", state.Version)
	}
	private := privateKeys(t, read.Private)
	if want := []privateKey{autoUpdateKey(autoUpdateOn)}; !slices.Equal(private, want) {
		t.Fatalf("stored %v after the import, want %v", private, want)
	}

	// The task upgrades the plugin before any plan stores version_pinned.
	fake.mu.Lock()
	fake.plugins = []client.InstalledPlugin{bookshelfAt("13.0.0.0", "Active")}
	fake.mu.Unlock()

	t.Run("version configured", func(t *testing.T) {
		t.Parallel()

		refreshed, plan := refreshAndPlan(t, r, state, types.StringValue("12.0.0.0"), private...)
		if refreshed.Version.ValueString() != "12.0.0.0" || refreshed.InstalledVersion.ValueString() != "13.0.0.0" {
			t.Errorf("refreshed version %s, installed_version %s; want 12.0.0.0 kept and 13.0.0.0", refreshed.Version, refreshed.InstalledVersion)
		}
		var planned PluginResourceModel
		if d := plan.Plan.Get(t.Context(), &planned); d.HasError() || len(plan.Diagnostics) > 0 || !planned.Version.Equal(refreshed.Version) || len(plan.RequiresReplace) > 0 {
			t.Errorf("plan %v, version %s, replace %v; want no change", plan.Diagnostics, planned.Version, plan.RequiresReplace)
		}
	})
	t.Run("version omitted", func(t *testing.T) {
		t.Parallel()

		read := readPlugin(t, r, state, private...)
		refreshed := pluginModel(t, read.State)
		if refreshed.Version.ValueString() != "12.0.0.0" {
			t.Fatalf("refreshed version %s, want 12.0.0.0 until a plan stores that it is omitted", refreshed.Version)
		}
		// omittedVersionPlanModifier plans the installed version.
		planned := refreshed
		planned.Version = refreshed.InstalledVersion
		plan := planPlugin(t, r, &refreshed, &planned, types.StringNull(), privateKeys(t, read.Private)...)
		if len(plan.Diagnostics) > 0 || len(plan.RequiresReplace) > 0 {
			t.Fatalf("plan %v, replace %v; want an update in place", plan.Diagnostics, plan.RequiresReplace)
		}
		if got := privateValue(t, plan.Private, privateVersionPinned); got != "false" {
			t.Errorf("planned version_pinned %s, want false", got)
		}
	})
}

func TestUnitPluginVersionChangeWhileUpdatesAreOn(t *testing.T) {
	t.Parallel()

	supported := supportedSecurityPluginVersion()
	security := PluginResourceModel{
		ID:               types.StringValue(normalizeGUID(jellyfinSecurityPluginID)),
		Name:             types.StringValue(securityPluginName),
		Version:          types.StringValue("99.0.0.0"),
		InstalledVersion: types.StringValue("99.0.0.0"),
		Status:           types.StringValue("Active"),
		RepositoryURL:    types.StringValue(securityPluginRepoURL),
	}
	securityPackages := []client.PackageInfo{{Name: securityPluginName, Versions: []client.VersionInfo{{Version: "99.0.0.0"}, {Version: supported}}}}
	installed := installedBookshelf("13.0.0.0", "13.0.0.0")

	tests := []struct {
		name     string
		packages []client.PackageInfo
		state    PluginResourceModel
		version  string
		private  []privateKey
		replace  bool
	}{
		{"an older version", nil, installed, "12.0.0.0", []privateKey{autoUpdateKey(autoUpdateOn)}, false},
		{"a newer version", nil, installed, "14.0.0.0", []privateKey{autoUpdateKey(autoUpdateOn)}, true},
		{"supported resolving below the installed version", securityPackages, security, pluginVersionSupported, []privateKey{autoUpdateKey(autoUpdateOn)}, false},
		{"latest resolving above the installed version", bookshelfPackage("14.0.0.0", "13.0.0.0"), installed, pluginVersionLatest, []privateKey{autoUpdateKey(autoUpdateOn)}, true},
		{"an older version while updates are off", nil, installed, "12.0.0.0", []privateKey{autoUpdateKey(autoUpdateOff)}, true},
		{"an older version with no schedule stored", nil, installed, "12.0.0.0", nil, true},
		{"an older version with an unreadable schedule stored", nil, installed, "12.0.0.0", []privateKey{{privateAutoUpdate, `"sometimes"`}}, true},
		{"an older version while the schedule is unknown", nil, installed, "12.0.0.0", []privateKey{autoUpdateKey(autoUpdateUnknown)}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := newFakePluginResource(t, &fakePluginServer{packages: tc.packages})
			plan := tc.state
			plan.Version = types.StringValue(tc.version)
			resp := modifyPluginPlan(t, r, tc.state, plan, tc.private...)
			if resp.Diagnostics.HasError() {
				t.Fatalf("ModifyPlan: %v", resp.Diagnostics)
			}
			if got := resp.RequiresReplace.Contains(path.Root("version")); got != tc.replace {
				t.Errorf("replace = %t, want %t", got, tc.replace)
			}
		})
	}
}

func TestUnitPluginPlanStoresWhetherVersionIsConfigured(t *testing.T) {
	t.Parallel()

	state := installedBookshelf("12.0.0.0", "12.0.0.0")
	tests := map[string]struct {
		configured types.String
		destroy    bool
		want       string
	}{
		"omitted":             {configured: types.StringNull(), want: "false"},
		"configured":          {configured: types.StringValue("12.0.0.0"), want: "true"},
		"unknown until apply": {configured: types.StringUnknown(), want: "true"},
		"destroyed":           {configured: types.StringNull(), destroy: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := newFakePluginResource(t, &fakePluginServer{})
			var plan *PluginResourceModel
			if !tc.destroy {
				planned := state
				if !tc.configured.IsNull() {
					planned.Version = tc.configured
				}
				plan = &planned
			}
			resp := planPlugin(t, r, &state, plan, tc.configured)
			if resp.Diagnostics.HasError() {
				t.Fatalf("ModifyPlan: %v", resp.Diagnostics)
			}
			if got := privateValue(t, resp.Private, privateVersionPinned); got != tc.want {
				t.Errorf("stored version_pinned %q, want %q", got, tc.want)
			}
		})
	}
}

// Without depends_on, Terraform may refresh and plan a plugin before it plans
// the task that turns automatic updates off, which would plan the plugin
// against the old schedule and keep a version the new one reinstalls.
func TestUnitPluginPlanFailsWhenTheScheduleIsPlannedAfterThePlugin(t *testing.T) {
	t.Parallel()

	r := newFakePluginResource(t, &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("13.0.0.0", "Active")}, pluginUpdateTriggers: updatesOn})
	refreshed, plan := refreshAndPlan(t, r, installedBookshelf("12.0.0.0", "12.0.0.0"), types.StringValue("12.0.0.0"), pinnedKey(true))
	if len(plan.Diagnostics) > 0 || plan.RequiresReplace.Contains(path.Root("version")) || refreshed.Version.ValueString() != "12.0.0.0" {
		t.Fatalf("plugin plan: %v, replace %v, refreshed version %s; want the pin kept without diagnostics", plan.Diagnostics, plan.RequiresReplace, refreshed.Version)
	}

	task := planPluginUpdates(t, r.client, updateTriggersOff(t))
	if got, want := described(task.Diagnostics), []string{"error Update Plugins schedule planned after the plugins it affects at triggers"}; !slices.Equal(got, want) {
		t.Fatalf("task diagnostics %q, want %q", got, want)
	}
	if detail := task.Diagnostics[0].Detail(); !strings.Contains(detail, "turn Jellyfin's automatic plugin updates off") || !strings.Contains(detail, "while they were on") || !strings.Contains(detail, "Bookshelf (installed 13.0.0.0, required 12.0.0.0)") {
		t.Errorf("task error detail %q, want it to name Bookshelf and both schedules", detail)
	}
}

// The task can also plan between the plugin's refresh and its plan, which
// Terraform runs back to back in one node; the plugin's plan then reports it.
func TestUnitPluginPlanFailsWhenTheScheduleIsPlannedBetweenRefreshAndPlan(t *testing.T) {
	t.Parallel()

	r := newFakePluginResource(t, &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("13.0.0.0", "Active")}, pluginUpdateTriggers: updatesOn})
	read := readPlugin(t, r, installedBookshelf("12.0.0.0", "12.0.0.0"), pinnedKey(true))
	if read.Diagnostics.HasError() {
		t.Fatalf("Read: %v", read.Diagnostics)
	}
	if task := planPluginUpdates(t, r.client, updateTriggersOff(t)); len(task.Diagnostics) > 0 {
		t.Fatalf("task diagnostics %v, want none", task.Diagnostics)
	}

	refreshed := pluginModel(t, read.State)
	plan := planPlugin(t, r, &refreshed, &refreshed, types.StringValue("12.0.0.0"), privateKeys(t, read.Private)...)
	if got, want := described(plan.Diagnostics), []string{"error Plugin planned before the Update Plugins schedule at version"}; !slices.Equal(got, want) {
		t.Fatalf("plugin diagnostics %q, want %q", got, want)
	}
	if detail := plan.Diagnostics[0].Detail(); !strings.Contains(detail, "installed version 13.0.0.0 satisfies version 12.0.0.0") || !strings.Contains(detail, "depends_on = [jellyfin_scheduled_task.<name>]") {
		t.Errorf("plugin error detail %q, want the versions and the depends_on to add", detail)
	}
}

// With depends_on, the task plans first and the plugin is refreshed and
// planned against the schedule it sets, which here reinstalls the pin.
func TestUnitPluginPlanFollowsTheSchedulePlannedFirst(t *testing.T) {
	t.Parallel()

	r := newFakePluginResource(t, &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("13.0.0.0", "Active")}, pluginUpdateTriggers: updatesOn})
	if task := planPluginUpdates(t, r.client, updateTriggersOff(t)); len(task.Diagnostics) > 0 {
		t.Fatalf("task diagnostics %v, want none", task.Diagnostics)
	}
	_, plan := refreshAndPlan(t, r, installedBookshelf("12.0.0.0", "12.0.0.0"), types.StringValue("12.0.0.0"), pinnedKey(true))
	if len(plan.Diagnostics) > 0 || !plan.RequiresReplace.Contains(path.Root("version")) {
		t.Errorf("plugin plan: %v, replace %v; want the pin reinstalled without diagnostics", plan.Diagnostics, plan.RequiresReplace)
	}
}

// A plan the schedule does not change needs no order, so neither order of the
// task and the plugin reports anything.
func TestUnitPluginPlanIgnoresScheduleChangesThatDoNotChangeIt(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		state      PluginResourceModel
		private    []privateKey
		configured types.String
		destroy    bool
	}{
		"version omitted":             {state: installedBookshelf("12.0.0.0", "12.0.0.0"), configured: types.StringNull()},
		"installed at the version":    {state: installedBookshelf("13.0.0.0", "13.0.0.0"), private: []privateKey{pinnedKey(true)}, configured: types.StringValue("13.0.0.0")},
		"installed below the version": {state: installedBookshelf("14.0.0.0", "14.0.0.0"), private: []privateKey{pinnedKey(true)}, configured: types.StringValue("14.0.0.0")},
		"keyword kept":                {state: installedBookshelf("latest", "13.0.0.0"), private: []privateKey{pinnedKey(true)}, configured: types.StringValue("latest")},
		"version unknown until apply": {state: installedBookshelf("12.0.0.0", "12.0.0.0"), private: []privateKey{pinnedKey(true)}, configured: types.StringUnknown()},
		"destroyed":                   {state: installedBookshelf("12.0.0.0", "12.0.0.0"), private: []privateKey{pinnedKey(true)}, destroy: true},
	}

	for name, tc := range tests {
		for _, scheduleFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/schedule first %t", name, scheduleFirst), func(t *testing.T) {
				t.Parallel()

				r := newFakePluginResource(t, &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("13.0.0.0", "Active")}, pluginUpdateTriggers: updatesOn})
				var diags diag.Diagnostics
				if scheduleFirst {
					diags.Append(planPluginUpdates(t, r.client, updateTriggersOff(t)).Diagnostics...)
				}
				read := readPlugin(t, r, tc.state, tc.private...)
				diags.Append(read.Diagnostics...)
				refreshed := pluginModel(t, read.State)
				var plan *PluginResourceModel
				if !tc.destroy {
					planned := refreshed
					if !tc.configured.IsNull() {
						planned.Version = tc.configured
					}
					plan = &planned
				}
				diags.Append(planPlugin(t, r, &refreshed, plan, tc.configured, privateKeys(t, read.Private)...).Diagnostics...)
				if !scheduleFirst {
					diags.Append(planPluginUpdates(t, r.client, updateTriggersOff(t)).Diagnostics...)
				}
				if len(diags) > 0 {
					t.Errorf("diagnostics %v, want none", diags)
				}
			})
		}
	}
}

// A plan that replaces the plugin for a new name or repository_url replaces
// it under either schedule, so no order of the task and the plugin reports
// anything, even with a pin the installed version exceeds.
func TestUnitPluginPlanIgnoresScheduleChangesForAReplacement(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*PluginResourceModel){
		"a new name": func(m *PluginResourceModel) {
			m.Name, m.Version = types.StringValue("Other"), types.StringValue("1.0.0.0")
		},
		"a new repository_url": func(m *PluginResourceModel) {
			m.RepositoryURL = types.StringValue("https://other.example/manifest.json")
		},
		"a new repository_url unknown until apply": func(m *PluginResourceModel) {
			m.RepositoryURL = types.StringUnknown()
		},
	}
	orders := []string{"schedule first", "schedule between refresh and plan", "schedule last"}

	for name, change := range tests {
		for _, order := range orders {
			for schedule, triggers := range map[string]func(*testing.T) types.List{"off": updateTriggersOff, "unknown": updateTriggersUnknown} {
				t.Run(fmt.Sprintf("%s/%s/%s", name, order, schedule), func(t *testing.T) {
					t.Parallel()

					r := newFakePluginResource(t, &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("13.0.0.0", "Active")}, pluginUpdateTriggers: updatesOn})
					var diags diag.Diagnostics
					if order == orders[0] {
						diags.Append(planPluginUpdates(t, r.client, triggers(t)).Diagnostics...)
					}
					read := readPlugin(t, r, installedBookshelf("12.0.0.0", "12.0.0.0"), pinnedKey(true))
					diags.Append(read.Diagnostics...)
					if order == orders[1] {
						diags.Append(planPluginUpdates(t, r.client, triggers(t)).Diagnostics...)
					}
					refreshed := pluginModel(t, read.State)
					planned := refreshed
					change(&planned)
					config := planned
					config.ID, config.InstalledVersion, config.Status = types.StringNull(), types.StringNull(), types.StringNull()
					diags.Append(planPluginFor(t, r, &refreshed, &planned, &config, privateKeys(t, read.Private)...).Diagnostics...)
					if order == orders[2] {
						diags.Append(planPluginUpdates(t, r.client, triggers(t)).Diagnostics...)
					}
					if len(diags) > 0 {
						t.Errorf("diagnostics %v, want none", diags)
					}
				})
			}
		}
	}
}

// With -refresh=false Read does not run, so the plan uses the schedule of the
// last refresh; that is deterministic, so it only warns.
func TestUnitPluginPlanWarnsWithoutARefresh(t *testing.T) {
	t.Parallel()

	state := installedBookshelf("12.0.0.0", "13.0.0.0")
	private := []privateKey{autoUpdateKey(autoUpdateOn), pinnedKey(true)}
	const stale = "Plugin planned without reading the Update Plugins schedule"

	t.Run("plugin first", func(t *testing.T) {
		t.Parallel()

		r := newFakePluginResource(t, &fakePluginServer{})
		if plan := modifyPluginPlan(t, r, state, state, private...); len(plan.Diagnostics) > 0 {
			t.Fatalf("plugin diagnostics %v, want none", plan.Diagnostics)
		}
		task := planPluginUpdates(t, r.client, updateTriggersOff(t))
		if got, want := described(task.Diagnostics), []string{"warning " + stale + " at triggers"}; !slices.Equal(got, want) {
			t.Errorf("task diagnostics %q, want %q", got, want)
		}
	})
	t.Run("schedule first", func(t *testing.T) {
		t.Parallel()

		r := newFakePluginResource(t, &fakePluginServer{})
		if task := planPluginUpdates(t, r.client, updateTriggersOff(t)); len(task.Diagnostics) > 0 {
			t.Fatalf("task diagnostics %v, want none", task.Diagnostics)
		}
		plan := modifyPluginPlan(t, r, state, state, private...)
		if got, want := described(plan.Diagnostics), []string{"warning " + stale + " at version"}; !slices.Equal(got, want) {
			t.Fatalf("plugin diagnostics %q, want %q", got, want)
		}
		if detail := plan.Diagnostics[0].Detail(); !strings.Contains(detail, "used Jellyfin's automatic plugin updates as on, as they were when Terraform last refreshed or created it, although a jellyfin_scheduled_task in this configuration turns them off") {
			t.Errorf("plugin warning detail %q, want the schedule of the last refresh and the planned one", detail)
		}
		if plan.RequiresReplace.Contains(path.Root("version")) {
			t.Error("plan replaces the plugin, want it planned with updates on as last refreshed")
		}
	})
	// State an earlier provider version wrote records no schedule, which
	// counts as off, and the warning does not claim a refresh recorded it.
	t.Run("no schedule stored", func(t *testing.T) {
		t.Parallel()

		r := newFakePluginResource(t, &fakePluginServer{})
		if plan := modifyPluginPlan(t, r, state, state, pinnedKey(true)); len(plan.Diagnostics) > 0 {
			t.Fatalf("plugin diagnostics %v, want none", plan.Diagnostics)
		}
		task := planPluginUpdates(t, r.client, updateTriggersOn(t))
		if got, want := described(task.Diagnostics), []string{"warning " + stale + " at triggers"}; !slices.Equal(got, want) {
			t.Fatalf("task diagnostics %q, want %q", got, want)
		}
		detail := task.Diagnostics[0].Detail()
		if !strings.Contains(detail, "used Jellyfin's automatic plugin updates as off, because its state, written by an earlier provider version and not refreshed since, records no schedule") || strings.Contains(detail, "last refreshed") {
			t.Errorf("task warning detail %q, want it to say no schedule is stored", detail)
		}
	})
	// A refresh that stored triggers unknown until apply, followed by a plan
	// that knows them, as the apply walk or a later -refresh=false plan does,
	// does not claim the triggers are still unknown.
	t.Run("unknown schedule stored", func(t *testing.T) {
		t.Parallel()

		const wasUnknown = "Plugin planned with an unknown Update Plugins schedule"
		stored := []privateKey{autoUpdateKey(autoUpdateUnknown), pinnedKey(true)}
		want := "were unknown until apply, which counts as on, so Bookshelf is planned with Jellyfin's automatic plugin updates on, although that jellyfin_scheduled_task now turns them off"

		t.Run("plugin first", func(t *testing.T) {
			t.Parallel()

			r := newFakePluginResource(t, &fakePluginServer{})
			if plan := modifyPluginPlan(t, r, state, state, stored...); len(plan.Diagnostics) > 0 {
				t.Fatalf("plugin diagnostics %v, want none", plan.Diagnostics)
			}
			task := planPluginUpdates(t, r.client, updateTriggersOff(t))
			if got, want := described(task.Diagnostics), []string{"warning " + wasUnknown + " at triggers"}; !slices.Equal(got, want) {
				t.Fatalf("task diagnostics %q, want %q", got, want)
			}
			if detail := task.Diagnostics[0].Detail(); !strings.Contains(detail, want) {
				t.Errorf("task warning detail %q, want it to contain %q", detail, want)
			}
		})
		t.Run("schedule first", func(t *testing.T) {
			t.Parallel()

			r := newFakePluginResource(t, &fakePluginServer{})
			if task := planPluginUpdates(t, r.client, updateTriggersOff(t)); len(task.Diagnostics) > 0 {
				t.Fatalf("task diagnostics %v, want none", task.Diagnostics)
			}
			plan := modifyPluginPlan(t, r, state, state, stored...)
			if got, want := described(plan.Diagnostics), []string{"warning " + wasUnknown + " at version"}; !slices.Equal(got, want) {
				t.Fatalf("plugin diagnostics %q, want %q", got, want)
			}
			if detail := plan.Diagnostics[0].Detail(); !strings.Contains(detail, want) {
				t.Errorf("plugin warning detail %q, want it to contain %q", detail, want)
			}
			if plan.RequiresReplace.Contains(path.Root("version")) {
				t.Error("plan replaces the plugin, want it planned with updates on as stored")
			}
		})
	})
}

// Triggers unknown until apply count as on for a plugin planned after the
// task, which never reinstalls a plugin that on keeps, while one planned
// before it uses the server's schedule; either way with a warning where the
// plan depends on them.
func TestUnitPluginPlanWarnsWhileTheScheduleIsUnknown(t *testing.T) {
	t.Parallel()

	const unknown = "Update Plugins schedule unknown until apply"
	state := installedBookshelf("12.0.0.0", "12.0.0.0")
	pin := types.StringValue("12.0.0.0")

	t.Run("schedule first", func(t *testing.T) {
		t.Parallel()

		r := newFakePluginResource(t, &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("13.0.0.0", "Active")}})
		if task := planPluginUpdates(t, r.client, updateTriggersUnknown(t)); len(task.Diagnostics) > 0 {
			t.Fatalf("task diagnostics %v, want none", task.Diagnostics)
		}
		read := readPlugin(t, r, state, pinnedKey(true))
		if read.Diagnostics.HasError() {
			t.Fatalf("Read: %v", read.Diagnostics)
		}
		refreshed := pluginModel(t, read.State)
		if refreshed.Version.ValueString() != "12.0.0.0" {
			t.Errorf("read version %s, want the pin kept", refreshed.Version)
		}
		if got := privateValue(t, read.Private, privateAutoUpdate); got != `"unknown"` {
			t.Errorf("stored auto_update %s, want \"unknown\"", got)
		}
		plan := planPlugin(t, r, &refreshed, &refreshed, pin, privateKeys(t, read.Private)...)
		if got, want := described(plan.Diagnostics), []string{"warning " + unknown + " at version"}; !slices.Equal(got, want) {
			t.Errorf("plugin diagnostics %q, want %q", got, want)
		}
		if detail := plan.Diagnostics[0].Detail(); !strings.Contains(detail, "Bookshelf is planned with updates on") {
			t.Errorf("plugin warning detail %q, want it to say the plan keeps updates on", detail)
		}
	})
	t.Run("plugin first", func(t *testing.T) {
		t.Parallel()

		r := newFakePluginResource(t, &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("13.0.0.0", "Active")}})
		_, plan := refreshAndPlan(t, r, state, pin, pinnedKey(true))
		if len(plan.Diagnostics) > 0 || !plan.RequiresReplace.Contains(path.Root("version")) {
			t.Fatalf("plugin plan: %v, replace %v; want the pin reinstalled without diagnostics", plan.Diagnostics, plan.RequiresReplace)
		}
		task := planPluginUpdates(t, r.client, updateTriggersUnknown(t))
		if got, want := described(task.Diagnostics), []string{"warning " + unknown + " at triggers"}; !slices.Equal(got, want) {
			t.Errorf("task diagnostics %q, want %q", got, want)
		}
	})
}

// Each provider configuration gets its own client, so a task planned through
// one says nothing about plugins read through another.
func TestUnitPluginAutoUpdateIsScopedToTheClient(t *testing.T) {
	t.Parallel()

	r := newFakePluginResource(t, &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("13.0.0.0", "Active")}, pluginUpdateTriggers: updatesOn})
	if task := planPluginUpdates(t, r.client, updateTriggersOff(t)); task.Diagnostics.HasError() {
		t.Fatalf("planning PluginUpdates: %v", task.Diagnostics)
	}
	other := &PluginResource{client: client.NewClient(r.client.BaseURL, r.client.APIKey)}

	resp := readPlugin(t, other, installedBookshelf("12.0.0.0", "12.0.0.0"), pinnedKey(true))
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics)
	}
	if got := privateValue(t, resp.Private, privateAutoUpdate); got != `"on"` {
		t.Errorf("stored auto_update %s, want the server's \"on\"", got)
	}
}

func TestUnitPluginCreateAdoptsANewerVersionWhileUpdatesAreOn(t *testing.T) {
	t.Parallel()

	fake := &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("14.0.0.0", "Active")}, pluginUpdateTriggers: updatesOn}
	r := newFakePluginResource(t, fake)
	resp := createPlugin(t, r, plannedBookshelf("13.0.0.0"))
	if len(resp.Diagnostics) > 0 {
		t.Fatalf("Create: %v", resp.Diagnostics)
	}
	got := pluginModel(t, resp.State)
	if got.Version.ValueString() != "13.0.0.0" || got.InstalledVersion.ValueString() != "14.0.0.0" || got.Status.ValueString() != "Active" {
		t.Errorf("created version %s, installed_version %s, status %s; want 13.0.0.0, 14.0.0.0, Active", got.Version, got.InstalledVersion, got.Status)
	}
	if installs := fake.installed(); len(installs) > 0 {
		t.Errorf("Create installed %q, want the newer version adopted", installs)
	}
	if keys, want := privateKeys(t, resp.Private), []privateKey{autoUpdateKey(autoUpdateOn), pinnedKey(true)}; !slices.Equal(keys, want) {
		t.Errorf("stored %v, want %v", keys, want)
	}
}

func TestUnitPluginCreateInstallsTheConfiguredVersionWhileUpdatesAreOff(t *testing.T) {
	t.Parallel()

	fake := &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("14.0.0.0", "Active")}, installID: bookshelfID}
	r := newFakePluginResource(t, fake)
	resp := createPlugin(t, r, plannedBookshelf("13.0.0.0"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics)
	}
	if installs, want := fake.installed(), []string{"Bookshelf 13.0.0.0"}; !slices.Equal(installs, want) {
		t.Errorf("installs %q, want %q", installs, want)
	}
	if got := pluginModel(t, resp.State); got.InstalledVersion.ValueString() != "13.0.0.0" || got.Status.ValueString() != "Restart" {
		t.Errorf("created installed_version %s, status %s; want 13.0.0.0, Restart", got.InstalledVersion, got.Status)
	}
	if got := privateValue(t, resp.Private, privateAutoUpdate); got != `"off"` {
		t.Errorf("stored auto_update %s, want \"off\"", got)
	}
}

func TestUnitPluginCreateDoesNotAdoptAVersionJellyfinCannotLoad(t *testing.T) {
	t.Parallel()

	fake := &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("14.0.0.0", pluginStatusNotSupported)}, pluginUpdateTriggers: updatesOn, installID: bookshelfID}
	r := newFakePluginResource(t, fake)
	resp := createPlugin(t, r, plannedBookshelf("13.0.0.0"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics)
	}
	if installs, want := fake.installed(), []string{"Bookshelf 13.0.0.0"}; !slices.Equal(installs, want) {
		t.Errorf("installs %q, want %q", installs, want)
	}
	if got := pluginModel(t, resp.State).InstalledVersion.ValueString(); got != "13.0.0.0" {
		t.Errorf("created installed_version %s, want 13.0.0.0", got)
	}
}

// An omitted version holds the installed version, so a create that adopts a
// newer one holds that one.
func TestUnitPluginCreateWithoutVersionTakesTheAdoptedVersion(t *testing.T) {
	t.Parallel()

	fake := &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("14.0.0.0", "Active")}, packages: bookshelfPackage("13.0.0.0"), pluginUpdateTriggers: updatesOn}
	r := newFakePluginResource(t, fake)
	planned := plannedBookshelf("")
	planned.Version = types.StringUnknown()
	resp := createPlugin(t, r, planned)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics)
	}
	if got := pluginModel(t, resp.State); got.Version.ValueString() != "14.0.0.0" || got.InstalledVersion.ValueString() != "14.0.0.0" {
		t.Errorf("created version %s, installed_version %s; want 14.0.0.0 for both", got.Version, got.InstalledVersion)
	}
	if got := privateValue(t, resp.Private, privateVersionPinned); got != "false" {
		t.Errorf("stored version_pinned %s, want false", got)
	}
}

// Create never keeps a version Jellyfin cannot load: it installs the
// configured version again, and fails only when Jellyfin still lists that
// version so afterwards. Terraform then keeps the plugin as tainted, and
// Create does not count the version as created this run, so the destroy that
// follows a create_before_destroy replacement still uninstalls it.
func TestUnitPluginCreateFailsOnAVersionJellyfinCannotLoad(t *testing.T) {
	t.Parallel()

	for _, status := range []string{pluginStatusNotSupported, pluginStatusMalfunctioned} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()

			// Jellyfin answers the install without listing another entry.
			fake := &fakePluginServer{plugins: []client.InstalledPlugin{bookshelfAt("13.0.0.0", status)}, pluginUpdateTriggers: updatesOn}
			r := newFakePluginResource(t, fake)
			resp := createPlugin(t, r, plannedBookshelf("13.0.0.0"))
			if got, want := described(resp.Diagnostics), []string{"error Plugin cannot be loaded"}; !slices.Equal(got, want) {
				t.Fatalf("Create diagnostics %q, want %q", got, want)
			}
			if installs, want := fake.installed(), []string{"Bookshelf 13.0.0.0"}; !slices.Equal(installs, want) {
				t.Errorf("installs %q, want %q", installs, want)
			}
			detail := resp.Diagnostics[0].Detail()
			if !strings.Contains(detail, "Bookshelf 13.0.0.0 with the status "+status) || !strings.Contains(detail, "installs the configured version again, after uninstalling this one unless the resource sets create_before_destroy") {
				t.Errorf("Create error detail %q, want the version, its status and what the next apply does", detail)
			}
			if resp.State.Raw.IsNull() {
				t.Fatal("Create set no state, so Terraform would not keep the plugin as tainted")
			}
			if got := pluginModel(t, resp.State).Status.ValueString(); got != status {
				t.Errorf("created status %s, want %s", got, status)
			}

			replaced := pluginResourceState(t, installedBookshelf("13.0.0.0", "13.0.0.0"))
			deleted := &resource.DeleteResponse{State: replaced}
			r.Delete(t.Context(), resource.DeleteRequest{State: replaced}, deleted)
			if deleted.Diagnostics.HasError() {
				t.Fatalf("Delete: %v", deleted.Diagnostics)
			}
			if left := fake.listed(); len(left) > 0 {
				t.Errorf("plugins left %+v, want the version Jellyfin cannot load uninstalled", left)
			}
		})
	}
}

// installedToKeep picks, from what Jellyfin lists, the entry Create keeps
// instead of installing the configured 13.0.0.0: the exact entry when
// Jellyfin can load it, else the newest newer entry it can load while
// updates are on, else none, and Create installs 13.0.0.0.
func TestUnitPluginCreateChoosesTheVersionToKeep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		listed        []client.InstalledPlugin
		triggers      []json.RawMessage
		installStatus int
		wantDiags     []string
		wantInstalls  []string
		// wantInstalled and wantStatus are those of the created state.
		wantInstalled, wantStatus string
	}{
		{
			name:     "exact NotSupported and a newer Active, updates on",
			listed:   []client.InstalledPlugin{bookshelfAt("13.0.0.0", pluginStatusNotSupported), bookshelfAt("14.0.0.0", "Active")},
			triggers: updatesOn, wantInstalled: "14.0.0.0", wantStatus: "Active",
		},
		{
			name:   "exact NotSupported and a newer Active, updates off",
			listed: []client.InstalledPlugin{bookshelfAt("13.0.0.0", pluginStatusNotSupported), bookshelfAt("14.0.0.0", "Active")},
			// Jellyfin's repositories offer no build of 13.0.0.0 for a server
			// that lists it NotSupported.
			installStatus: http.StatusNotFound, wantDiags: []string{"error Failed to install plugin"},
		},
		{
			name:     "a newer Malfunctioned, updates on",
			listed:   []client.InstalledPlugin{bookshelfAt("14.0.0.0", pluginStatusMalfunctioned)},
			triggers: updatesOn, wantInstalls: []string{"Bookshelf 13.0.0.0"}, wantInstalled: "13.0.0.0", wantStatus: "Restart",
		},
		{
			name:     "a newer Superceded and a newest Restart, updates on",
			listed:   []client.InstalledPlugin{bookshelfAt("14.0.0.0", "Superceded"), bookshelfAt("15.0.0.0", "Restart")},
			triggers: updatesOn, wantInstalled: "15.0.0.0", wantStatus: "Restart",
		},
		{
			name:     "exact only pending deletion",
			listed:   []client.InstalledPlugin{bookshelfAt("13.0.0.0", pluginStatusDeleted)},
			triggers: updatesOn, wantInstalls: []string{"Bookshelf 13.0.0.0"}, wantInstalled: "13.0.0.0", wantStatus: "Restart",
		},
		{
			name:     "exact Malfunctioned, reinstalled",
			listed:   []client.InstalledPlugin{bookshelfAt("13.0.0.0", pluginStatusMalfunctioned)},
			triggers: updatesOn, wantInstalls: []string{"Bookshelf 13.0.0.0"}, wantInstalled: "13.0.0.0", wantStatus: "Restart",
		},
		{
			name:          "exact NotSupported, no newer version",
			listed:        []client.InstalledPlugin{bookshelfAt("13.0.0.0", pluginStatusNotSupported)},
			triggers:      updatesOn,
			installStatus: http.StatusNotFound, wantDiags: []string{"error Failed to install plugin"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakePluginServer{plugins: tc.listed, pluginUpdateTriggers: tc.triggers, installID: bookshelfID, installStatus: tc.installStatus}
			r := newFakePluginResource(t, fake)
			resp := createPlugin(t, r, plannedBookshelf("13.0.0.0"))
			if got := described(resp.Diagnostics); !slices.Equal(got, tc.wantDiags) {
				t.Fatalf("Create diagnostics %q, want %q", got, tc.wantDiags)
			}
			if installs := fake.installed(); !slices.Equal(installs, tc.wantInstalls) {
				t.Errorf("installs %q, want %q", installs, tc.wantInstalls)
			}
			if tc.wantDiags != nil {
				if detail := resp.Diagnostics[0].Detail(); !strings.Contains(detail, "No enabled plugin repository offers Bookshelf 13.0.0.0") {
					t.Errorf("Create error detail %q, want the version reported as not offered", detail)
				}
				return
			}
			got := pluginModel(t, resp.State)
			if got.Version.ValueString() != "13.0.0.0" || got.InstalledVersion.ValueString() != tc.wantInstalled || got.Status.ValueString() != tc.wantStatus {
				t.Errorf("created version %s, installed_version %s, status %s; want 13.0.0.0, %s, %s", got.Version, got.InstalledVersion, got.Status, tc.wantInstalled, tc.wantStatus)
			}
		})
	}
}

// Create always reads the schedule and stores it, so a plan that does not
// refresh the plugin afterwards uses the schedule of its create.
func TestUnitPluginCreateStoresTheSchedule(t *testing.T) {
	t.Parallel()

	listings := map[string][]client.InstalledPlugin{
		"not installed":            nil,
		"installed at the version": {bookshelfAt("13.0.0.0", "Active")},
		"installed below it":       {bookshelfAt("12.0.0.0", "Active")},
		"installed above it":       {bookshelfAt("14.0.0.0", "Active")},
	}
	for name, listed := range listings {
		for _, triggers := range [][]json.RawMessage{updatesOn, nil} {
			want := autoUpdateOff
			if triggers != nil {
				want = autoUpdateOn
			}
			t.Run(name+"/"+string(want), func(t *testing.T) {
				t.Parallel()

				fake := &fakePluginServer{plugins: listed, installID: bookshelfID, pluginUpdateTriggers: triggers}
				resp := createPlugin(t, newFakePluginResource(t, fake), plannedBookshelf("13.0.0.0"))
				if resp.Diagnostics.HasError() {
					t.Fatalf("Create: %v", resp.Diagnostics)
				}
				if n := fake.scheduledTaskRequests(); n != 1 {
					t.Errorf("Create sent %d GET /ScheduledTasks, want 1", n)
				}
				if got := privateValue(t, resp.Private, privateAutoUpdate); got != strconv.Quote(string(want)) {
					t.Errorf("stored auto_update %s, want %q", got, want)
				}
			})
		}
		t.Run(name+"/unreadable", func(t *testing.T) {
			t.Parallel()

			fake := &fakePluginServer{plugins: listed, installID: bookshelfID, tasksStatus: http.StatusInternalServerError}
			resp := createPlugin(t, newFakePluginResource(t, fake), plannedBookshelf("13.0.0.0"))
			if got, want := described(resp.Diagnostics), []string{"error Failed to read the Update Plugins schedule"}; !slices.Equal(got, want) {
				t.Errorf("Create diagnostics %q, want %q", got, want)
			}
			if installs := fake.installed(); len(installs) > 0 {
				t.Errorf("Create installed %q before reading the schedule", installs)
			}
		})
	}
}

func TestUnitPluginLatestNewerThanSupportedWarns(t *testing.T) {
	t.Parallel()

	const newer = "99.0.0.0"
	supported := supportedSecurityPluginVersion()
	packages := []client.PackageInfo{{Name: securityPluginName, Versions: []client.VersionInfo{{Version: newer}, {Version: supported}}}}
	security := func(version, installed string) PluginResourceModel {
		return PluginResourceModel{
			ID:               types.StringValue(normalizeGUID(jellyfinSecurityPluginID)),
			Name:             types.StringValue(securityPluginName),
			Version:          types.StringValue(version),
			InstalledVersion: types.StringValue(installed),
			Status:           types.StringValue("Active"),
			RepositoryURL:    types.StringValue(securityPluginRepoURL),
		}
	}
	const want = "warning Plugin version newer than supported at version"

	t.Run("plan", func(t *testing.T) {
		t.Parallel()

		r := newFakePluginResource(t, &fakePluginServer{packages: packages})
		state := security(pluginVersionSupported, supported)
		resp := modifyPluginPlan(t, r, state, security(pluginVersionLatest, supported))
		if got := described(resp.Diagnostics); !slices.Equal(got, []string{want}) {
			t.Fatalf("ModifyPlan diagnostics %q, want %q", got, want)
		}
		if detail := resp.Diagnostics[0].Detail(); !strings.Contains(detail, "latest resolves Jellyfin Security to "+newer+", which is newer than "+supported) {
			t.Errorf("warning detail %q, want both versions", detail)
		}
	})
	t.Run("create", func(t *testing.T) {
		t.Parallel()

		listed := []client.InstalledPlugin{{ID: jellyfinSecurityPluginID, Name: securityPluginName, Version: newer, Status: "Active"}}
		r := newFakePluginResource(t, &fakePluginServer{packages: packages, plugins: listed})
		planned := security(pluginVersionLatest, "")
		planned.ID, planned.InstalledVersion, planned.Status = types.StringUnknown(), types.StringUnknown(), types.StringUnknown()
		resp := createPlugin(t, r, planned)
		if got := described(resp.Diagnostics); !slices.Equal(got, []string{want}) {
			t.Errorf("Create diagnostics %q, want %q", got, want)
		}
	})
}
