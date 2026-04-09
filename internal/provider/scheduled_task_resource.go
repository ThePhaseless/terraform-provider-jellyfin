// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
	TriggersJSON jsontypes.Normalized `tfsdk:"triggers_json"`
}

func (r *ScheduledTaskResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scheduled_task"
}

func (r *ScheduledTaskResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages triggers for a Jellyfin scheduled task. " +
			"Allows configuring when scheduled tasks run (e.g., library scans, trickplay generation).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The unique identifier of the scheduled task.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"triggers_json": schema.StringAttribute{
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
	_, err := r.client.GetScheduledTask(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to find scheduled task", err.Error())
		return
	}

	if err := r.client.UpdateScheduledTaskTriggers(data.ID.ValueString(), data.TriggersJSON.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to update scheduled task triggers", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ScheduledTaskResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ScheduledTaskResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	task, err := r.client.GetScheduledTask(data.ID.ValueString())
	if err != nil {
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

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ScheduledTaskResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ScheduledTaskResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateScheduledTaskTriggers(data.ID.ValueString(), data.TriggersJSON.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to update scheduled task triggers", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ScheduledTaskResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Cannot delete a scheduled task - we just remove from state.
	// The task will keep its current triggers.
}

func (r *ScheduledTaskResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
