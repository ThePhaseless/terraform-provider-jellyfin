// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &ServerRestartResource{}

// NewServerRestartResource creates a new server restart resource.
func NewServerRestartResource() resource.Resource {
	return &ServerRestartResource{}
}

// ServerRestartResource defines the resource implementation.
type ServerRestartResource struct {
	client *client.Client
}

// ServerRestartResourceModel describes the resource data model.
type ServerRestartResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Triggers      types.Map    `tfsdk:"triggers"`
	LastRestarted types.String `tfsdk:"last_restarted"`
}

func (r *ServerRestartResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_restart"
}

func (r *ServerRestartResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Restarts the Jellyfin server. Use the `triggers` map to force a restart " +
			"when other resources change (similar to the `null_resource` pattern).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A generated identifier for the restart event.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"triggers": schema.MapAttribute{
				MarkdownDescription: "A map of arbitrary strings that, when changed, will trigger a new restart.",
				Optional:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"last_restarted": schema.StringAttribute{
				MarkdownDescription: "The RFC3339 timestamp of the last successful restart.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ServerRestartResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ServerRestartResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ServerRestartResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RestartServer(); err != nil {
		resp.Diagnostics.AddError("Failed to restart server", err.Error())
		return
	}

	if err := r.waitForServer(ctx, 2*time.Minute); err != nil {
		resp.Diagnostics.AddError("Server did not come back online", err.Error())
		return
	}

	id, err := uuid.GenerateUUID()
	if err != nil {
		resp.Diagnostics.AddError("Failed to generate resource ID", err.Error())
		return
	}

	data.ID = types.StringValue(id)
	data.LastRestarted = types.StringValue(time.Now().UTC().Format(time.RFC3339))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ServerRestartResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
	// No-op: there is no remote state to refresh for a one-shot restart.
}

func (r *ServerRestartResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// triggers requires replacement, so Update should never be called.
	resp.Diagnostics.AddError("Update not supported", "Server restart resource updates require replacement.")
}

func (r *ServerRestartResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// No-op: deleting from state does not need to perform a server action.
}

// waitForServer polls the public system info endpoint until the server responds or the timeout expires.
func (r *ServerRestartResource) waitForServer(ctx context.Context, timeout time.Duration) error {
	// Give the server a brief moment to begin shutting down before we start polling.
	time.Sleep(2 * time.Second)

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := r.client.GetPublicSystemInfo(); err == nil {
			return nil
		} else {
			lastErr = err
			tflog.Debug(ctx, "Waiting for Jellyfin server to come back online", map[string]interface{}{"error": err.Error()})
		}
		time.Sleep(2 * time.Second)
	}

	if lastErr != nil {
		return fmt.Errorf("server did not respond within %s: %w", timeout, lastErr)
	}
	return fmt.Errorf("server did not respond within %s", timeout)
}
