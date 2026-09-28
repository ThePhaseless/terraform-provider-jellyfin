// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

var (
	_ resource.Resource                   = &ScheduledTaskResource{}
	_ resource.ResourceWithImportState    = &ScheduledTaskResource{}
	_ resource.ResourceWithValidateConfig = &ScheduledTaskResource{}
	_ resource.ResourceWithModifyPlan     = &ScheduledTaskResource{}
	_ wireBound                           = &ScheduledTaskResource{}
)

const (
	triggerTypeDaily    = "DailyTrigger"
	triggerTypeWeekly   = "WeeklyTrigger"
	triggerTypeInterval = "IntervalTrigger"
	triggerTypeStartup  = "StartupTrigger"

	ticksPerDay = 864_000_000_000

	// CancellationTokenSource.CancelAfter truncates its delay to whole
	// milliseconds and takes at most 4294967294 of them.
	maxRuntimeTicksLimit = 4_294_967_295*10_000 - 1
)

// NewScheduledTaskResource creates a new scheduled task resource.
func NewScheduledTaskResource() resource.Resource {
	return &ScheduledTaskResource{}
}

// ScheduledTaskResource defines the resource implementation.
type ScheduledTaskResource struct {
	client *client.Client
}

// ScheduledTaskResourceModel describes the resource data model.
type ScheduledTaskResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Key      types.String `tfsdk:"key"`
	TaskID   types.String `tfsdk:"task_id"`
	Triggers types.List   `tfsdk:"triggers"`
}

var scheduledTaskWire = sync.OnceValues(func() (*wire.Binding, error) {
	s := schemaOf(&ScheduledTaskResource{})
	triggers, _ := s.Attributes["triggers"].GetType().(types.ListType)
	return wire.Bind(s, "TaskInfo",
		wire.Identity("id", "task_id"),
		// A task without triggers must read as the empty list its
		// configuration holds, whether Jellyfin serves [], null or no key.
		wire.ReadMissingAs("triggers", types.ListValueMust(triggers.ElemType, nil)))
})

func (r *ScheduledTaskResource) Wire() (*wire.Binding, error) { return scheduledTaskWire() }

// ScheduledTaskTriggerModel describes one trigger element.
type ScheduledTaskTriggerModel struct {
	Type            types.String `tfsdk:"type"`
	TimeOfDayTicks  types.Int64  `tfsdk:"time_of_day_ticks"`
	IntervalTicks   types.Int64  `tfsdk:"interval_ticks"`
	DayOfWeek       types.String `tfsdk:"day_of_week"`
	MaxRuntimeTicks types.Int64  `tfsdk:"max_runtime_ticks"`
}

func (r *ScheduledTaskResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scheduled_task"
}

func (r *ScheduledTaskResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages triggers for a Jellyfin scheduled task.",
		MarkdownDescription: "Manages triggers for a Jellyfin scheduled task.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "The resource identifier, matching the scheduled task ID.",
				MarkdownDescription: "The resource identifier, matching the scheduled task ID.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key": schema.StringAttribute{
				Description:         "The task's key, the readable name Jellyfin lists next to its ID in GET /ScheduledTasks, such as RefreshLibrary (Scan Media Library) or PluginUpdates (Update Plugins). It matches exactly, case included, and set without task_id must belong to one task only. Set key, task_id, or both naming the same task; with only task_id set, key reads the task's key. A key no task has fails the plan, terraform destroy included, so once its task is gone, such as after removing the plugin that added it, remove the resource from the configuration or destroy with -refresh=false.",
				MarkdownDescription: "The task's key, the readable name Jellyfin lists next to its ID in `GET /ScheduledTasks`, such as `RefreshLibrary` (*Scan Media Library*) or `PluginUpdates` (*Update Plugins*). It matches exactly, case included, and set without `task_id` must belong to one task only. Set `key`, `task_id`, or both naming the same task; with only `task_id` set, `key` reads the task's key. A key no task has fails the plan, `terraform destroy` included, so once its task is gone, such as after removing the plugin that added it, remove the resource from the configuration or destroy with `-refresh=false`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				// State from releases without key holds it as null, which
				// UseStateForUnknown would plan, although apply reads the key.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"task_id": schema.StringAttribute{
				Description:         "The task's ID, which Jellyfin derives from an MD5 hash of the full name of the .NET type that runs the task and matches ignoring case, such as 7738148ffcd07979c7ceb148e06b3aed for Scan Media Library. Set key, task_id, or both naming the same task; with only key set, task_id reads the ID of the task the key selects.",
				MarkdownDescription: "The task's ID, which Jellyfin derives from an MD5 hash of the full name of the .NET type that runs the task and matches ignoring case, such as `7738148ffcd07979c7ceb148e06b3aed` for *Scan Media Library*. Set `key`, `task_id`, or both naming the same task; with only `key` set, `task_id` reads the ID of the task the key selects.",
				Optional:            true,
				Computed:            true,
				Validators:          requiredIdentifierValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			// The optional trigger attributes are not Computed because Jellyfin stores
			// each trigger exactly as posted and never fills in fields, so an omitted
			// attribute must plan as null rather than unknown.
			"triggers": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Description:         "The trigger type (DailyTrigger, WeeklyTrigger, IntervalTrigger, StartupTrigger).",
							MarkdownDescription: "The trigger type (`DailyTrigger`, `WeeklyTrigger`, `IntervalTrigger`, `StartupTrigger`).",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf(triggerTypeDaily, triggerTypeWeekly, triggerTypeInterval, triggerTypeStartup),
							},
						},
						// Jellyfin adds these ticks to a date unchecked and saves the trigger
						// before its timer takes the due time. Depending on the value and the
						// server clock, a negative value or one of a day or more either
						// schedules runs at another time or fails the request with a 400
						// while the trigger stays on the server.
						"time_of_day_ticks": schema.Int64Attribute{
							Description:         "Time of day the task runs, in ticks (100 ns) after midnight, from 0 to 863999999999. Required for DailyTrigger and WeeklyTrigger.",
							MarkdownDescription: "Time of day the task runs, in ticks (100 ns) after midnight, from `0` to `863999999999`. Required for `DailyTrigger` and `WeeklyTrigger`.",
							Optional:            true,
							Validators: []validator.Int64{
								int64validator.Between(0, ticksPerDay-1),
							},
						},
						"interval_ticks": schema.Int64Attribute{
							Description:         "Interval between runs, in ticks (100 ns). Required for IntervalTrigger.",
							MarkdownDescription: "Interval between runs, in ticks (100 ns). Required for `IntervalTrigger`.",
							Optional:            true,
							Validators: []validator.Int64{
								int64validator.AtLeast(0),
							},
						},
						"day_of_week": schema.StringAttribute{
							Description:         "Day of the week the task runs (Sunday through Saturday). Required for WeeklyTrigger.",
							MarkdownDescription: "Day of the week the task runs (`Sunday` through `Saturday`). Required for `WeeklyTrigger`.",
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"),
							},
						},
						// Jellyfin saves any value but passes it to CancelAfter only when the
						// trigger starts a run, so a value CancelAfter rejects fails every such
						// run before the task begins. The few negative values it takes mean no
						// limit or an immediate cancel, which leaving the attribute unset or
						// setting it below 10000 already say.
						"max_runtime_ticks": schema.Int64Attribute{
							Description:         "Maximum time the task may run before Jellyfin cancels it, in ticks (100 ns), from 0 to 42949672949999 (about 49.7 days). Jellyfin counts whole milliseconds, so a value below 10000 (1 ms), including 0, cancels each run as soon as it starts; leave it unset for no limit.",
							MarkdownDescription: "Maximum time the task may run before Jellyfin cancels it, in ticks (100 ns), from `0` to `42949672949999` (about 49.7 days). Jellyfin counts whole milliseconds, so a value below `10000` (1 ms), including `0`, cancels each run as soon as it starts; leave it unset for no limit.",
							Optional:            true,
							Validators: []validator.Int64{
								int64validator.Between(0, maxRuntimeTicksLimit),
							},
						},
					},
				},
				Description:         "The task triggers. This list replaces all of the task's triggers. Each trigger is sent exactly as configured, so an optional attribute left unset is removed from the server; declare every attribute an existing trigger should keep, such as the max_runtime_ticks some built-in tasks ship with.",
				MarkdownDescription: "The task triggers. This list replaces all of the task's triggers. Each trigger is sent exactly as configured, so an optional attribute left unset is removed from the server; declare every attribute an existing trigger should keep, such as the `max_runtime_ticks` some built-in tasks ship with.",
				Required:            true,
			},
		},
	}
}

func (r *ScheduledTaskResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ScheduledTaskResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var key, taskID types.String
	var triggers types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("key"), &key)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("task_id"), &taskID)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("triggers"), &triggers)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if key.IsNull() && taskID.IsNull() {
		resp.Diagnostics.AddError("Missing scheduled task attribute",
			`Set key, such as "RefreshLibrary", or task_id to select the scheduled task.`)
	}

	if triggers.IsNull() || triggers.IsUnknown() {
		return
	}

	for i, elem := range triggers.Elements() {
		obj, ok := elem.(types.Object)
		if !ok || obj.IsNull() || obj.IsUnknown() {
			continue
		}

		var trigger ScheduledTaskTriggerModel
		diags := obj.As(ctx, &trigger, basetypes.ObjectAsOptions{})
		resp.Diagnostics.Append(diags...)
		if diags.HasError() {
			return
		}

		for _, name := range missingTriggerAttributes(trigger) {
			resp.Diagnostics.AddAttributeError(
				path.Root("triggers").AtListIndex(i).AtName(name),
				"Missing trigger attribute",
				fmt.Sprintf("%s is required when type is %q.", name, trigger.Type.ValueString()),
			)
		}
	}
}

func (r *ScheduledTaskResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ScheduledTaskResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Verify the task exists.
	if _, err := r.client.GetScheduledTask(ctx, data.TaskID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to find scheduled task", err.Error())
		return
	}

	r.writeTriggers(ctx, &data, "create", &resp.Diagnostics, &resp.State)
}

func (r *ScheduledTaskResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ScheduledTaskResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	b := wireBinding(&resp.Diagnostics, scheduledTaskWire)
	if b == nil {
		return
	}

	// Only an import leaves task_id unset, with id holding the ID or key it
	// was given.
	if data.TaskID.IsNull() {
		id, d := r.lookUpTask(ctx, data.ID.ValueString(), true)
		if d != nil {
			resp.Diagnostics.Append(d)
			return
		}
		data.TaskID = types.StringValue(id)
	}

	task, err := r.client.GetScheduledTask(ctx, data.TaskID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read scheduled task", err.Error())
		return
	}

	resp.Diagnostics.Append(b.FlattenInto(ctx, task.RawJSON, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// task_id keeps the configured spelling: Jellyfin matches task IDs
	// case-insensitively but returns them in lowercase, so copying task.ID would
	// force a replacement on every plan for an uppercase task_id.
	data.ID = data.TaskID

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ScheduledTaskResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ScheduledTaskResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.writeTriggers(ctx, &data, "update", &resp.Diagnostics, &resp.State)
}

// ModifyPlan gates each configured field on the Jellyfin version it needs, so
// a field a later pin adds is checked without a change here, and checks a
// configured key against the server's tasks.
func (r *ScheduledTaskResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	if b := wireBinding(&resp.Diagnostics, scheduledTaskWire); b != nil {
		resp.Diagnostics.Append(checkServerHasFields(ctx, r.client, b, req.Config)...)
	}
	resp.Diagnostics.Append(r.planTaskForKey(ctx, req, resp)...)
}

// planTaskForKey plans task_id as the ID of the task a key configured alone
// selects, and a replacement when that is not the task in the state. With
// task_id also configured, task_id selects the task and key must be its key;
// both may be set because terraform plan -generate-config-out writes both
// from an imported state.
//
// Terraform plans each resource again in the refresh that precedes a destroy,
// as a create when the refresh found its task gone, so a key that no task has
// any more, such as that of a task a plugin or a Jellyfin upgrade removed,
// also fails terraform destroy unless it runs with -refresh=false.
func (r *ScheduledTaskResource) planTaskForKey(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) diag.Diagnostics {
	var diags diag.Diagnostics
	var key, taskID types.String
	diags.Append(req.Config.GetAttribute(ctx, path.Root("key"), &key)...)
	diags.Append(req.Config.GetAttribute(ctx, path.Root("task_id"), &taskID)...)
	switch {
	case diags.HasError() || key.IsNull():
		return diags
	case key.IsUnknown() && taskID.IsNull():
		// Terraform plans again at apply, once the key is known, and rejects
		// that plan if it replaces what this one updates.
		if !req.State.Raw.IsNull() {
			resp.RequiresReplace.Append(path.Root("key"))
		}
		return diags
	case key.IsUnknown() || taskID.IsUnknown() || r.client == nil:
		// The plan Terraform makes at apply checks what is unknown now.
		return diags
	case !taskID.IsNull():
		task, err := r.client.GetScheduledTask(ctx, taskID.ValueString())
		switch {
		case client.IsNotFound(err):
			id, d := r.lookUpTask(ctx, key.ValueString(), false)
			if d != nil {
				return append(diags, diag.WithPath(path.Root("key"), d))
			}
			diags.AddAttributeError(path.Root("task_id"), "Conflicting scheduled task attributes", fmt.Sprintf(
				"No scheduled task has the ID %q, and the key %q names the task %s. Set key or task_id alone, or both to the same task.",
				taskID.ValueString(), key.ValueString(), id))
		case err != nil:
			diags.AddError("Failed to read scheduled task", err.Error())
		case task.Key != key.ValueString():
			diags.AddAttributeError(path.Root("key"), "Conflicting scheduled task attributes", fmt.Sprintf(
				"task_id %q names the task with the key %q, not %q. Set key or task_id alone, or both to the same task.",
				taskID.ValueString(), task.Key, key.ValueString()))
		}
		return diags
	}

	id, d := r.lookUpTask(ctx, key.ValueString(), false)
	if d != nil {
		return append(diags, diag.WithPath(path.Root("key"), d))
	}
	if !req.State.Raw.IsNull() {
		var stored types.String
		diags.Append(req.State.GetAttribute(ctx, path.Root("task_id"), &stored)...)
		if diags.HasError() || strings.EqualFold(stored.ValueString(), id) {
			return diags
		}
		// Terraform replaces only for a path whose value the plan changes, and
		// the key keeps its value when its task gets a new ID, such as after
		// the .NET type that runs the task is renamed.
		resp.RequiresReplace.Append(path.Root("task_id"))
	}
	return append(diags, resp.Plan.SetAttribute(ctx, path.Root("task_id"), id)...)
}

func (r *ScheduledTaskResource) lookUpTask(ctx context.Context, ref string, byID bool) (string, diag.Diagnostic) {
	tasks, err := r.client.GetScheduledTasks(ctx)
	if err != nil {
		return "", diag.NewErrorDiagnostic("Failed to list scheduled tasks", err.Error())
	}
	return findTask(tasks, ref, byID)
}

// findTask returns the ID of the one task ref selects, or an error saying why
// none does. With byID, a task's ID matches ignoring case, as Jellyfin
// matches it, and the ID returned keeps ref's spelling. A key matches exactly,
// as the Jellyfin web client matches it; no Jellyfin endpoint takes one.
func findTask(tasks []client.ScheduledTask, ref string, byID bool) (string, diag.Diagnostic) {
	var ids, keys []string
	for _, t := range tasks {
		if byID && strings.EqualFold(t.ID, ref) {
			return ref, nil
		}
		if t.Key == ref {
			ids = append(ids, t.ID)
		}
		if t.Key != "" {
			keys = append(keys, t.Key)
		}
	}
	switch len(ids) {
	case 1:
		return ids[0], nil
	case 0:
		slices.Sort(keys)
		what, gone := "key", " If a plugin or a Jellyfin upgrade removed the task, remove the resource from the configuration, or destroy it with -refresh=false."
		if byID {
			what, gone = "ID or key", ""
		}
		return "", diag.NewErrorDiagnostic("Scheduled task not found", fmt.Sprintf(
			"No scheduled task has the %s %q. Keys match exactly, case included. The server's tasks have these keys: %s.%s",
			what, ref, strings.Join(keys, ", "), gone))
	}
	choose := "set task_id to the ID of the one to manage"
	if byID {
		choose = "import the one to manage by its ID"
	}
	return "", diag.NewErrorDiagnostic("Ambiguous scheduled task key", fmt.Sprintf(
		"Several tasks have the key %q (IDs %s); %s.", ref, strings.Join(ids, ", "), choose))
}

func (r *ScheduledTaskResource) writeTriggers(ctx context.Context, data *ScheduledTaskResourceModel, operation string, diags *diag.Diagnostics, state *tfsdk.State) {
	b := wireBinding(diags, scheduledTaskWire)
	if b == nil {
		return
	}

	body, d := triggersBody(ctx, b, data)
	diags.Append(d...)
	if diags.HasError() {
		return
	}

	if err := r.client.UpdateScheduledTaskTriggers(ctx, data.TaskID.ValueString(), body); err != nil {
		diags.AddError("Failed to update scheduled task triggers", err.Error())
		return
	}

	updated, err := r.client.GetScheduledTask(ctx, data.TaskID.ValueString())
	if err != nil {
		diags.AddError("Failed to read scheduled task after "+operation, err.Error())
		return
	}

	diags.Append(b.FlattenAfterApply(ctx, updated.RawJSON, data)...)
	data.ID = data.TaskID
	diags.Append(state.Set(ctx, data)...)
}

// triggersBody writes the task's triggers through b and returns the list on
// its own, which is what the triggers endpoint takes.
func triggersBody(ctx context.Context, b *wire.Binding, data *ScheduledTaskResourceModel) (string, diag.Diagnostics) {
	task := map[string]json.RawMessage{}
	diags := b.OverlayModel(ctx, task, data)
	if diags.HasError() {
		return "", diags
	}
	for _, f := range b.Fields {
		if f.Path != "triggers" || len(f.KeyPath) != 1 {
			continue
		}
		if raw, ok := task[f.KeyPath[0]]; ok {
			return string(raw), diags
		}
	}
	diags.AddError("Failed to serialize triggers", "The task binding wrote no top-level triggers list.\n\nThis is a bug in the provider.")
	return "", diags
}

func (r *ScheduledTaskResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Cannot delete a scheduled task - we just remove from state.
}

func (r *ScheduledTaskResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// missingTriggerAttributes returns the attributes the trigger's type requires that are null.
// Jellyfin rejects a trigger without them with a bare "Error processing request." 400.
func missingTriggerAttributes(t ScheduledTaskTriggerModel) []string {
	var missing []string
	switch t.Type.ValueString() {
	case triggerTypeDaily:
		if t.TimeOfDayTicks.IsNull() {
			missing = append(missing, "time_of_day_ticks")
		}
	case triggerTypeWeekly:
		if t.TimeOfDayTicks.IsNull() {
			missing = append(missing, "time_of_day_ticks")
		}
		if t.DayOfWeek.IsNull() {
			missing = append(missing, "day_of_week")
		}
	case triggerTypeInterval:
		if t.IntervalTicks.IsNull() {
			missing = append(missing, "interval_ticks")
		}
	}
	return missing
}
