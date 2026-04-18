// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ScheduledTasksDataSource{}

// NewScheduledTasksDataSource creates a new scheduled tasks list data source.
func NewScheduledTasksDataSource() datasource.DataSource {
	return &ScheduledTasksDataSource{}
}

// ScheduledTasksDataSource defines the data source implementation.
type ScheduledTasksDataSource struct {
	client *client.Client
}

// ScheduledTasksDataSourceTaskModel describes a single scheduled task element.
type ScheduledTasksDataSourceTaskModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Category    types.String `tfsdk:"category"`
	State       types.String `tfsdk:"state"`
	Key         types.String `tfsdk:"key"`
}

// ScheduledTasksDataSourceModel describes the data source data model.
type ScheduledTasksDataSourceModel struct {
	Tasks []ScheduledTasksDataSourceTaskModel `tfsdk:"tasks"`
}

func (d *ScheduledTasksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scheduled_tasks"
}

func (d *ScheduledTasksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all Jellyfin scheduled tasks. Task IDs are server-specific GUIDs and may be used as the `id` " +
			"attribute of a `jellyfin_scheduled_task` resource.",
		Attributes: map[string]schema.Attribute{
			"tasks": schema.ListNestedAttribute{
				MarkdownDescription: "The list of scheduled tasks.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The task ID.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The task name.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "The task description.",
							Computed:            true,
						},
						"category": schema.StringAttribute{
							MarkdownDescription: "The task category.",
							Computed:            true,
						},
						"state": schema.StringAttribute{
							MarkdownDescription: "The task state (e.g., `Idle`, `Running`).",
							Computed:            true,
						},
						"key": schema.StringAttribute{
							MarkdownDescription: "The task key (stable identifier).",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *ScheduledTasksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}

	d.client = c
}

func (d *ScheduledTasksDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	tasks, err := d.client.GetScheduledTasks()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get scheduled tasks", err.Error())
		return
	}

	data := ScheduledTasksDataSourceModel{
		Tasks: make([]ScheduledTasksDataSourceTaskModel, 0, len(tasks)),
	}
	for _, t := range tasks {
		data.Tasks = append(data.Tasks, ScheduledTasksDataSourceTaskModel{
			ID:          types.StringValue(t.Id),
			Name:        types.StringValue(t.Name),
			Description: types.StringValue(t.Description),
			Category:    types.StringValue(t.Category),
			State:       types.StringValue(t.State),
			Key:         types.StringValue(t.Key),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
