// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
	ID           types.String         `tfsdk:"id"`
	TaskID       types.String         `tfsdk:"task_id"`
	TriggersJSON jsontypes.Normalized `tfsdk:"triggers_json"`
}

func (r *ScheduledTaskResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scheduled_task"
}

func (r *ScheduledTaskResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages triggers for a Jellyfin scheduled task. " +
			"Allows configuring when scheduled tasks run (e.g., library scans, trickplay generation).",
		MarkdownDescription: "Manages triggers for a Jellyfin scheduled task. " +
			"Allows configuring when scheduled tasks run (e.g., library scans, trickplay generation).",
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
			"triggers_json": schema.StringAttribute{
				Description: "The task triggers as a JSON array string. Each trigger object can have " +
					"Type (DailyTrigger, IntervalTrigger, StartupTrigger, WeeklyTrigger), " +
					"TimeOfDayTicks, IntervalTicks, DayOfWeek, MaxRuntimeTicks.",
				MarkdownDescription: "The task triggers as a JSON array string. Each trigger object can have " +
					"Type (DailyTrigger, IntervalTrigger, StartupTrigger, WeeklyTrigger), " +
					"TimeOfDayTicks, IntervalTicks, DayOfWeek, MaxRuntimeTicks.",
				Required:   true,
				CustomType: jsontypes.NormalizedType{},
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
	_, err := r.client.GetScheduledTask(ctx, data.TaskID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to find scheduled task", err.Error())
		return
	}

	if err := r.client.UpdateScheduledTaskTriggers(ctx, data.TaskID.ValueString(), data.TriggersJSON.ValueString()); err != nil {
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

	rawTriggers, err := json.Marshal(task.Triggers)
	if err != nil {
		resp.Diagnostics.AddError("Failed to serialize task triggers", err.Error())
		return
	}

	normalizedTriggers, err := normalizeJSON(string(rawTriggers))
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize task triggers", err.Error())
		return
	}

	data.TriggersJSON = jsontypes.NewNormalizedValue(normalizedTriggers)
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

	if err := r.client.UpdateScheduledTaskTriggers(ctx, data.TaskID.ValueString(), data.TriggersJSON.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to update scheduled task triggers", err.Error())
		return
	}

	task, err := r.client.GetScheduledTask(ctx, data.TaskID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read scheduled task after update", err.Error())
		return
	}
	rawTriggers, err := json.Marshal(task.Triggers)
	if err != nil {
		resp.Diagnostics.AddError("Failed to serialize task triggers", err.Error())
		return
	}
	normalizedTriggers, err := normalizeJSON(string(rawTriggers))
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize task triggers", err.Error())
		return
	}
	data.ID = types.StringValue(task.ID)
	data.TaskID = types.StringValue(task.ID)
	data.TriggersJSON = jsontypes.NewNormalizedValue(normalizedTriggers)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ScheduledTaskResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Cannot delete a scheduled task - we just remove from state.
	// The task will keep its current triggers.
}

func (r *ScheduledTaskResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	triggersJSON := jsontypes.NewNormalizedNull()
	if r.client != nil {
		task, err := r.client.GetScheduledTask(ctx, req.ID)
		if err != nil {
			resp.Diagnostics.AddError("Failed to read scheduled task during import", err.Error())
			return
		}
		rawTriggers, err := json.Marshal(task.Triggers)
		if err != nil {
			resp.Diagnostics.AddError("Failed to serialize task triggers during import", err.Error())
			return
		}
		normalizedTriggers, err := normalizeJSON(string(rawTriggers))
		if err != nil {
			resp.Diagnostics.AddError("Failed to normalize task triggers during import", err.Error())
			return
		}
		triggersJSON = jsontypes.NewNormalizedValue(normalizedTriggers)
	}

	data := ScheduledTaskResourceModel{
		ID:           types.StringValue(req.ID),
		TaskID:       types.StringValue(req.ID),
		TriggersJSON: triggersJSON,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
