// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

// NewRestartResource creates a new jellyfin_restart resource.
func NewRestartResource() resource.Resource {
	return &RestartResource{}
}

// RestartResource implements an action resource that restarts the Jellyfin
// server. It is modeled on null_resource/terraform_data: the restart fires on
// create and whenever a value in `triggers` changes (which forces replacement).
type RestartResource struct {
	client *client.Client
}

// RestartResourceModel describes the resource data model.
type RestartResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Triggers    types.Map    `tfsdk:"triggers"`
	Timeout     types.Int64  `tfsdk:"timeout"`
	CompletedAt types.String `tfsdk:"completed_at"`
}

func (r *RestartResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_restart"
}

func (r *RestartResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Restarts the Jellyfin server. The restart fires on create and whenever any value in `triggers` changes (which forces replacement). Use it to load newly installed plugins so a following jellyfin_plugin_configuration can run in the same apply.",
		MarkdownDescription: "Restarts the Jellyfin server. The restart fires on create and whenever any value in `triggers` changes (which forces replacement). Use it to load newly installed plugins so a following `jellyfin_plugin_configuration` can run in the same apply.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "Unique identifier for this restart action.",
				MarkdownDescription: "Unique identifier for this restart action.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"triggers": schema.MapAttribute{
				Description:         "Map of arbitrary string values that, when changed, force a new restart. Tie restarts to upstream changes, e.g. `triggers = { plugin_version = jellyfin_plugin.x.version }`.",
				MarkdownDescription: "Map of arbitrary string values that, when changed, force a new restart. Tie restarts to upstream changes, e.g. `triggers = { plugin_version = jellyfin_plugin.x.version }`.",
				ElementType:         types.StringType,
				Optional:            true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"timeout": schema.Int64Attribute{
				Description:         "Maximum number of seconds to wait for the server to come back up after restart. Defaults to 120.",
				MarkdownDescription: "Maximum number of seconds to wait for the server to come back up after restart. Defaults to 120.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(120),
			},
			"completed_at": schema.StringAttribute{
				Description:         "RFC3339 timestamp marking when the server was ready after the restart.",
				MarkdownDescription: "RFC3339 timestamp marking when the server was ready after the restart.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *RestartResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RestartResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RestartResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RestartServer(ctx); err != nil {
		resp.Diagnostics.AddError("Failed to restart Jellyfin server", err.Error())
		return
	}

	timeout := 120 * time.Second
	if !data.Timeout.IsNull() && !data.Timeout.IsUnknown() {
		timeout = time.Duration(data.Timeout.ValueInt64()) * time.Second
	}
	if err := waitForServerReady(ctx, r.client, timeout); err != nil {
		resp.Diagnostics.AddError("Jellyfin server did not become ready after restart", err.Error())
		return
	}

	id, err := randomID()
	if err != nil {
		resp.Diagnostics.AddError("Failed to generate restart resource id", err.Error())
		return
	}
	data.ID = types.StringValue(id)
	data.CompletedAt = types.StringValue(time.Now().UTC().Format(time.RFC3339))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RestartResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// The restart is a point-in-time action; refresh preserves state as-is.
	var data RestartResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RestartResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// `triggers` forces replacement, so Update is never reached.
	resp.Diagnostics.AddError("Update not supported", "jellyfin_restart is replace-only: change a value in triggers to force a new restart.")
}

func (r *RestartResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Nothing to undo on the server; just drop from state.
}

// waitForServerReady polls /System/Info until the server responds and
// HasPendingRestart is false, indicating the restart completed and plugins are
// loaded. Any HTTP/network/decode error (the expected mid-restart states) is
// treated as "not ready" and retried. Reuses startupStatusDelay from provider.go.
func waitForServerReady(ctx context.Context, c *client.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("server did not report ready (HasPendingRestart=false) within %s", timeout)
		}
		if info, err := c.GetSystemInfo(ctx); err == nil && !info.HasPendingRestart {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(startupStatusDelay):
		}
	}
}

// randomID returns a 16-byte hex string for the resource id.
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
