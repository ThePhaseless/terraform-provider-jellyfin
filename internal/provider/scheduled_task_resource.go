// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

var (
	_ resource.Resource                = &ScheduledTaskResource{}
	_ resource.ResourceWithImportState = &ScheduledTaskResource{}
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
	TaskID   types.String `tfsdk:"task_id"`
	Triggers types.List   `tfsdk:"triggers"`
}

// ScheduledTaskTriggerModel describes one trigger element.
type ScheduledTaskTriggerModel struct {
	Type          types.String `tfsdk:"type"`
	TimeOfDayTicks types.Int64  `tfsdk:"time_of_day_ticks"`
	IntervalTicks  types.Int64  `tfsdk:"interval_ticks"`
	DayOfWeek     types.String `tfsdk:"day_of_week"`
	MaxRuntimeTicks types.Int64 `tfsdk:"max_runtime_ticks"`
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
			"task_id": schema.StringAttribute{
				Description:         "The unique identifier of the scheduled task.",
				MarkdownDescription: "The unique identifier of the scheduled task.",
				Required:            true,
				Validators:          requiredIdentifierValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"triggers": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Description:         "The trigger type (DailyTrigger, WeeklyTrigger, IntervalTrigger, StartupTrigger).",
							MarkdownDescription: "The trigger type (DailyTrigger, WeeklyTrigger, IntervalTrigger, StartupTrigger).",
							Required:            true,
						},
						"time_of_day_ticks": schema.Int64Attribute{
							Description:         "Time of day ticks.",
							MarkdownDescription: "Time of day ticks.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
						},
						"interval_ticks": schema.Int64Attribute{
							Description:         "Interval ticks.",
							MarkdownDescription: "Interval ticks.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
						},
						"day_of_week": schema.StringAttribute{
							Description:         "Day of week.",
							MarkdownDescription: "Day of week.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"max_runtime_ticks": schema.Int64Attribute{
							Description:         "Maximum runtime ticks.",
							MarkdownDescription: "Maximum runtime ticks.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
						},
					},
				},
				Description:         "The task triggers.",
				MarkdownDescription: "The task triggers.",
				Required:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
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

	triggersJSON, err := marshalTriggers(ctx, data.Triggers)
	if err != nil {
		resp.Diagnostics.AddError("Failed to serialize triggers", err.Error())
		return
	}

	if err := r.client.UpdateScheduledTaskTriggers(ctx, data.TaskID.ValueString(), triggersJSON); err != nil {
		resp.Diagnostics.AddError("Failed to update scheduled task triggers", err.Error())
		return
	}

	data.ID = data.TaskID
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ScheduledTaskResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ScheduledTaskResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
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

	triggers, diags := flattenTriggers(ctx, task.Triggers)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Triggers = triggers
	data.ID = types.StringValue(task.ID)
	data.TaskID = types.StringValue(task.ID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ScheduledTaskResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ScheduledTaskResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	triggersJSON, err := marshalTriggers(ctx, data.Triggers)
	if err != nil {
		resp.Diagnostics.AddError("Failed to serialize triggers", err.Error())
		return
	}

	if err := r.client.UpdateScheduledTaskTriggers(ctx, data.TaskID.ValueString(), triggersJSON); err != nil {
		resp.Diagnostics.AddError("Failed to update scheduled task triggers", err.Error())
		return
	}

	task, err := r.client.GetScheduledTask(ctx, data.TaskID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read scheduled task after update", err.Error())
		return
	}

	triggers, diags := flattenTriggers(ctx, task.Triggers)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Triggers = triggers
	data.ID = types.StringValue(task.ID)
	data.TaskID = types.StringValue(task.ID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ScheduledTaskResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Cannot delete a scheduled task - we just remove from state.
}

func (r *ScheduledTaskResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("task_id"), req, resp)
}

func marshalTriggers(ctx context.Context, list types.List) (string, error) {
	var triggers []ScheduledTaskTriggerModel
	if diags := list.ElementsAs(ctx, &triggers, false); diags.HasError() {
		return "", fmt.Errorf("extracting triggers: %v", diags)
	}

	rawEntries := make([]map[string]json.RawMessage, len(triggers))
	for i, t := range triggers {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "Type", t.Type)
		putJSONInt64(entry, "TimeOfDayTicks", t.TimeOfDayTicks)
		putJSONInt64(entry, "IntervalTicks", t.IntervalTicks)
		putJSONString(entry, "DayOfWeek", t.DayOfWeek)
		putJSONInt64(entry, "MaxRuntimeTicks", t.MaxRuntimeTicks)
		rawEntries[i] = entry
	}

	b, err := json.Marshal(rawEntries)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func flattenTriggers(ctx context.Context, raw []json.RawMessage) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	objType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"type":              types.StringType,
		"time_of_day_ticks": types.Int64Type,
		"interval_ticks":    types.Int64Type,
		"day_of_week":       types.StringType,
		"max_runtime_ticks": types.Int64Type,
	}}

	objects := make([]attr.Value, len(raw))
	for i, r := range raw {
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(r, &entry); err != nil {
			return types.ListNull(objType), append(diags, diag.NewErrorDiagnostic("Failed to parse trigger", err.Error()))
		}

		attrs := map[string]attr.Value{
			"type":              getJSONString(entry, "Type"),
			"time_of_day_ticks": getJSONInt64(entry, "TimeOfDayTicks"),
			"interval_ticks":    getJSONInt64(entry, "IntervalTicks"),
			"day_of_week":       getJSONString(entry, "DayOfWeek"),
			"max_runtime_ticks": getJSONInt64(entry, "MaxRuntimeTicks"),
		}
		obj, d := types.ObjectValue(objType.AttrTypes, attrs)
		if d.HasError() {
			return types.ListNull(objType), append(diags, d...)
		}
		objects[i] = obj
	}

	list, d := types.ListValue(objType, objects)
	if d.HasError() {
		return types.ListNull(objType), append(diags, d...)
	}
	return list, diags
}
