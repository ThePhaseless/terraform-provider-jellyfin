// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

// PluginUpdatesTaskKey is the key of Jellyfin's Update Plugins scheduled
// task, whose triggers decide whether jellyfin_plugin reads version as a
// minimum or as exact.
const PluginUpdatesTaskKey = "PluginUpdates"

// autoUpdate is whether Jellyfin updates plugins automatically, as the
// triggers of its PluginUpdates task say.
type autoUpdate string

const (
	autoUpdateOn  autoUpdate = "on"
	autoUpdateOff autoUpdate = "off"
	// autoUpdateUnknown is the schedule of triggers unknown until apply.
	autoUpdateUnknown autoUpdate = "unknown"
)

// minimum reports whether version is a minimum under a. An unknown schedule
// counts as on, which keeps a plugin that off would reinstall instead of
// reinstalling one that on would keep.
func (a autoUpdate) minimum() bool { return a == autoUpdateOn || a == autoUpdateUnknown }

func (a autoUpdate) known() bool { return a == autoUpdateOn || a == autoUpdateOff }

// Keys of jellyfin_plugin's private state.
const (
	// privateAutoUpdate holds the schedule the last refresh read version
	// against: "on", "off" or "unknown".
	privateAutoUpdate = "auto_update"
	// privateVersionPinned holds whether the configuration sets version, which
	// Read cannot see: true or false.
	privateVersionPinned = "version_pinned"
)

// privateGetter and privateSetter are the methods of the private state that
// the framework's requests and responses carry.
type privateGetter interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
}

type privateSetter interface {
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

// storedAutoUpdate returns the schedule private holds and true, or off and
// false when it holds none, as in state an earlier provider version wrote,
// when version was always exact.
func storedAutoUpdate(ctx context.Context, private privateGetter) (autoUpdate, bool, diag.Diagnostics) {
	raw, diags := private.GetKey(ctx, privateAutoUpdate)
	var a autoUpdate
	if diags.HasError() || json.Unmarshal(raw, &a) != nil || (!a.known() && a != autoUpdateUnknown) {
		return autoUpdateOff, false, diags
	}
	return a, true, diags
}

func storeAutoUpdate(ctx context.Context, private privateSetter, a autoUpdate) diag.Diagnostics {
	return private.SetKey(ctx, privateAutoUpdate, []byte(strconv.Quote(string(a))))
}

// storedVersionPinned returns whether private says the configuration sets
// version. When it says nothing, as for an imported plugin or in state an
// earlier provider version wrote, a concrete version in state counts as set:
// jellyfin-import writes the installed version into the configuration, and
// a configuration that omits version moves it to the installed one in place
// once, which stores false.
func storedVersionPinned(ctx context.Context, private privateGetter, version types.String) (bool, diag.Diagnostics) {
	raw, diags := private.GetKey(ctx, privateVersionPinned)
	if diags.HasError() {
		return false, diags
	}
	var pinned bool
	if json.Unmarshal(raw, &pinned) != nil {
		v := version.ValueString()
		return v != "" && !isPluginVersionKeyword(v), diags
	}
	return pinned, diags
}

func storeVersionPinned(ctx context.Context, private privateSetter, pinned bool) diag.Diagnostics {
	return private.SetKey(ctx, privateVersionPinned, []byte(strconv.FormatBool(pinned)))
}

// pluginUpdatesOn reports whether Jellyfin updates plugins automatically:
// whether GET /ScheduledTasks lists its PluginUpdates task with a trigger. The
// per-plugin autoUpdate flag in meta.json is not read.
func pluginUpdatesOn(ctx context.Context, c *client.Client) (autoUpdate, error) {
	tasks, err := c.GetScheduledTasks(ctx)
	if err != nil {
		return "", err
	}
	for _, task := range tasks {
		if task.Key == PluginUpdatesTaskKey && len(task.Triggers) > 0 {
			return autoUpdateOn, nil
		}
	}
	return autoUpdateOff, nil
}

func scheduleReadError(diags *diag.Diagnostics, err error) {
	diags.AddError("Failed to read the Update Plugins schedule",
		err.Error()+"\n\njellyfin_plugin reads the triggers of Jellyfin's PluginUpdates task to decide whether version is a minimum or exact.")
}

// autoUpdateWalks holds an *autoUpdateWalk for each client, which the provider
// configures anew for each Terraform walk.
var autoUpdateWalks sync.Map

// autoUpdateWalk connects, within one walk, the Update Plugins schedule a
// jellyfin_scheduled_task plans with the jellyfin_plugin plans that depend on
// it: Terraform orders the two only with depends_on, so whichever plans second
// reports a plugin planned against another schedule than the one planned.
type autoUpdateWalk struct {
	mu sync.Mutex
	// planned is the schedule a PluginUpdates task planned in this walk, or ""
	// until one plans a change.
	planned autoUpdate
	// plugins holds each plugin refreshed or planned in this walk, keyed by
	// normalizeGUID of its ID.
	plugins map[string]*pluginAutoUpdate
}

type pluginAutoUpdate struct {
	name string
	// used is the schedule the plugin's plan is based on.
	used autoUpdate
	// recorded is whether the plugin's private state held used, which is off
	// by default when it holds none.
	recorded bool
	// refreshed is whether the plugin's Read ran in this walk.
	refreshed bool
	// sensitive is whether the plugin's last plan differs between updates on
	// and off.
	sensitive bool
	// installed and required are the versions the messages name.
	installed, required string
}

// autoUpdateWalkFor returns the walk c serves, or nil for a nil client; every
// method of a nil walk records nothing and reports nothing.
func autoUpdateWalkFor(c *client.Client) *autoUpdateWalk {
	if c == nil {
		return nil
	}
	v, _ := autoUpdateWalks.LoadOrStore(c, &autoUpdateWalk{plugins: map[string]*pluginAutoUpdate{}})
	w, _ := v.(*autoUpdateWalk)
	return w
}

// entry returns the plugin's entry, adding one no Read made. The caller holds
// w.mu.
func (w *autoUpdateWalk) entry(id, name string) *pluginAutoUpdate {
	e, ok := w.plugins[id]
	if !ok {
		e = &pluginAutoUpdate{}
		w.plugins[id] = e
	}
	e.name = name
	return e
}

// plannedFor returns the schedule a PluginUpdates task planned in this walk,
// if one did, and records that the plugin's Read used it.
func (w *autoUpdateWalk) plannedFor(id, name string) (autoUpdate, bool) {
	if w == nil {
		return "", false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.planned == "" {
		return "", false
	}
	e := w.entry(id, name)
	e.used, e.refreshed = w.planned, true
	return w.planned, true
}

// decided records that the plugin's Read used server, the schedule it read,
// unless a PluginUpdates task planned one while it did, and returns the one
// used.
func (w *autoUpdateWalk) decided(id, name string, server autoUpdate) autoUpdate {
	if w == nil {
		return server
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	e := w.entry(id, name)
	e.used, e.refreshed = cmp.Or(w.planned, server), true
	return e.used
}

// checkPlugin records plan, the plugin's plan in this walk: the schedule it
// is based on, whether it depends on the schedule, and the versions it
// compares. It reports the plan when a PluginUpdates task in this walk
// planned another schedule, as an error if the plugin was refreshed against
// the old one.
func (w *autoUpdateWalk) checkPlugin(id string, plan pluginAutoUpdate) diag.Diagnostics {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	e := w.entry(id, plan.name)
	e.used, e.recorded, e.sensitive, e.installed, e.required = plan.used, plan.recorded, plan.sensitive, plan.installed, plan.required
	p, planned := *e, w.planned
	w.mu.Unlock()

	var diags diag.Diagnostics
	at := path.Root("version")
	switch {
	case !p.sensitive || planned == "" || (planned == p.used && p.used.known()):
	case !planned.known() || (!p.used.known() && p.refreshed):
		diags.AddAttributeWarning(at, scheduleUnknownSummary, scheduleUnknownDetail(p))
	case !p.used.known():
		diags.AddAttributeWarning(at, scheduleWasUnknownSummary, scheduleWasUnknownDetail(p, planned))
	case p.refreshed:
		diags.AddAttributeError(at, "Plugin planned before the Update Plugins schedule", fmt.Sprintf(
			"Terraform refreshed %[1]s while Jellyfin's automatic plugin updates were %[2]s, and then planned a jellyfin_scheduled_task that turns them %[3]s. "+
				"The plan for %[1]s depends on which: with updates on, installed version %[4]s satisfies version %[5]s and is kept, and with them off it is reinstalled at %[5]s. "+
				"Add depends_on = [jellyfin_scheduled_task.<name>], naming the resource that manages the PluginUpdates task, to this jellyfin_plugin, so Terraform plans and applies the schedule first. "+
				"If that resource depends on this plugin, manage PluginUpdates in a jellyfin_scheduled_task of its own.",
			p.name, p.used, planned, p.installed, p.required))
	default:
		diags.AddAttributeWarning(at, scheduleStaleSummary, scheduleStaleDetail(p, planned))
	}
	return diags
}

// planSchedule records the schedule a PluginUpdates task plans in this walk
// and reports each plugin whose plan in this walk it contradicts, as an error
// for the plugins refreshed against another schedule. A second task that
// plans the other known schedule is an error, and a known schedule planned
// next to an unknown one makes it unknown.
func (w *autoUpdateWalk) planSchedule(schedule autoUpdate) diag.Diagnostics {
	if w == nil {
		return nil
	}
	var diags diag.Diagnostics
	at := path.Root("triggers")

	w.mu.Lock()
	prior := w.planned
	if prior.known() && schedule.known() && prior != schedule {
		w.mu.Unlock()
		diags.AddAttributeError(at, "PluginUpdates task scheduled twice", fmt.Sprintf(
			"Another jellyfin_scheduled_task in this configuration also manages the PluginUpdates task and turns Jellyfin's automatic plugin updates %s, while this one turns them %s. "+
				"Manage each scheduled task in one resource only.", prior, schedule))
		return diags
	}
	if prior != "" && prior != schedule {
		schedule = autoUpdateUnknown
	}
	w.planned = schedule
	var affected []pluginAutoUpdate
	for _, e := range w.plugins {
		if e.sensitive && (e.used != schedule || !schedule.known()) {
			affected = append(affected, *e)
		}
	}
	w.mu.Unlock()

	slices.SortFunc(affected, func(a, b pluginAutoUpdate) int { return strings.Compare(a.name, b.name) })
	var conflicts []string
	var was autoUpdate
	for _, p := range affected {
		switch {
		case !schedule.known() || (!p.used.known() && p.refreshed):
			diags.AddAttributeWarning(at, scheduleUnknownSummary, scheduleUnknownDetail(p))
		case !p.used.known():
			diags.AddAttributeWarning(at, scheduleWasUnknownSummary, scheduleWasUnknownDetail(p, schedule))
		case p.refreshed:
			conflicts = append(conflicts, fmt.Sprintf("%s (installed %s, required %s)", p.name, p.installed, p.required))
			was = p.used
		default:
			diags.AddAttributeWarning(at, scheduleStaleSummary, scheduleStaleDetail(p, schedule))
		}
	}
	if len(conflicts) > 0 {
		diags.AddAttributeError(at, "Update Plugins schedule planned after the plugins it affects", fmt.Sprintf(
			"These triggers turn Jellyfin's automatic plugin updates %s, but Terraform refreshed and planned these jellyfin_plugin resources while they were %s, and their plans depend on which: %s. "+
				"With updates on, an installed version at least as new as the required one is kept, and with them off it is reinstalled at the required version. "+
				"Add depends_on on this jellyfin_scheduled_task to each of them, so Terraform plans and applies the schedule first. "+
				"If this resource depends on one of them, manage PluginUpdates in a jellyfin_scheduled_task of its own.",
			schedule, was, strings.Join(conflicts, ", ")))
	}
	return diags
}

const (
	scheduleStaleSummary      = "Plugin planned without reading the Update Plugins schedule"
	scheduleUnknownSummary    = "Update Plugins schedule unknown until apply"
	scheduleWasUnknownSummary = "Plugin planned with an unknown Update Plugins schedule"
)

// scheduleWasUnknownDetail explains a plan based on the unknown schedule a
// refresh in an earlier walk stored, which the configuration now sets: the
// apply walk after a plan with unknown triggers, or a plan without refresh.
func scheduleWasUnknownDetail(p pluginAutoUpdate, planned autoUpdate) string {
	return fmt.Sprintf(
		"When Terraform last refreshed %[1]s, the triggers of the jellyfin_scheduled_task that manages PluginUpdates were unknown until apply, which counts as on, "+
			"so %[1]s is planned with Jellyfin's automatic plugin updates on, although that jellyfin_scheduled_task now turns them %[2]s, and the plan for %[1]s depends on which: "+
			"installed version %[3]s satisfies version %[4]s with updates on, but is reinstalled at %[4]s with them off. "+
			"The next plan with refresh plans against the schedule the apply sets.",
		p.name, planned, p.installed, p.required)
}

func scheduleStaleDetail(p pluginAutoUpdate, planned autoUpdate) string {
	used := fmt.Sprintf("as %s, as they were when Terraform last refreshed or created it", p.used)
	if !p.recorded {
		used = "as off, because its state, written by an earlier provider version and not refreshed since, records no schedule"
	}
	return fmt.Sprintf(
		"Terraform planned %[1]s without refreshing it, as with -refresh=false, so it used Jellyfin's automatic plugin updates %[2]s, "+
			"although a jellyfin_scheduled_task in this configuration turns them %[3]s, and the plan for %[1]s depends on which: "+
			"installed version %[4]s satisfies version %[5]s with updates on, but is reinstalled at %[5]s with them off. "+
			"Plan again with refresh, after this apply, to plan against the new schedule.",
		p.name, used, planned, p.installed, p.required)
}

func scheduleUnknownDetail(p pluginAutoUpdate) string {
	planned := autoUpdateOff
	if p.used.minimum() {
		planned = autoUpdateOn
	}
	return fmt.Sprintf(
		"The triggers of the jellyfin_scheduled_task that manages PluginUpdates are unknown until apply, and the plan for %[1]s depends on whether they turn Jellyfin's automatic plugin updates on: "+
			"with them on, installed version %[2]s satisfies version %[3]s and is kept, and with them off it is reinstalled at %[3]s. "+
			"%[1]s is planned with updates %[4]s; if the apply sets them otherwise, the next plan corrects it. "+
			"Set the triggers to values known at plan time to plan against the schedule they set.",
		p.name, p.installed, p.required, planned)
}
