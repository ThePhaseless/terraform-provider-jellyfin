// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

const testPoll = 10 * time.Millisecond

func systemInfoHandler(pending func() (down bool, hasPendingRestart bool)) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		down, hasPending := pending()
		if down {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": "s", "HasPendingRestart": hasPending})
	}
}

func TestRandomIDIsUniqueAndHex(t *testing.T) {
	t.Parallel()

	a, err := randomID()
	if err != nil {
		t.Fatalf("randomID() error = %v", err)
	}
	b, err := randomID()
	if err != nil {
		t.Fatalf("randomID() error = %v", err)
	}
	if a == b {
		t.Fatalf("expected distinct ids, got %q twice", a)
	}
	if len(a) != 32 {
		t.Fatalf("expected 32-char hex id, got %q (len %d)", a, len(a))
	}
}

func TestAwaitRestartWaitsOutTheOutgoingHost(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	start := time.Now()
	server := httptest.NewServer(systemInfoHandler(func() (bool, bool) {
		return calls.Add(1) <= 2, false // the restart takes the server away briefly
	}))
	defer server.Close()

	c := client.NewClient(server.URL, "k")
	if err := awaitRestart(t.Context(), c, 5*time.Second, testPoll); err != nil {
		t.Fatalf("awaitRestart() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed < restartSettleReads*testPoll {
		t.Errorf("returned in %s, before the settle delay had elapsed", elapsed)
	}
}

func TestAwaitRestartRequiresConsecutiveHealthyReads(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(systemInfoHandler(func() (bool, bool) {
		return calls.Add(1)%2 == 0, false // flapping: never healthy twice running
	}))
	defer server.Close()

	c := client.NewClient(server.URL, "k")
	if err := awaitRestart(t.Context(), c, 400*time.Millisecond, testPoll); err == nil {
		t.Fatal("a server that never answers consecutively is not back; expected an error")
	}
}

func TestAwaitRestartIgnoresPendingRestart(t *testing.T) {
	t.Parallel()

	// Background plugin auto-updates keep HasPendingRestart true; that must not
	// stop a responsive server from counting as back.
	server := httptest.NewServer(systemInfoHandler(func() (bool, bool) { return false, true }))
	defer server.Close()

	c := client.NewClient(server.URL, "k")
	if err := awaitRestart(t.Context(), c, 5*time.Second, testPoll); err != nil {
		t.Fatalf("awaitRestart() error = %v", err)
	}
}

func TestAwaitRestartHonoursCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(systemInfoHandler(func() (bool, bool) { return true, false }))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// A server that never comes back would also end the wait, but only once
	// the timeout elapses and with another error.
	start := time.Now()
	c := client.NewClient(server.URL, "k")
	if err := awaitRestart(ctx, c, 5*time.Second, testPoll); !errors.Is(err, context.Canceled) {
		t.Fatalf("awaitRestart() error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Errorf("returned after %s, not as soon as the context was cancelled", elapsed)
	}
}

func TestRestartUpdateChangesTimeoutWithoutRestarting(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// No client: an Update that restarted the server would panic.
	r := &RestartResource{}
	s := schemaOf(r)
	prior := RestartResourceModel{
		ID:          types.StringValue("restart"),
		Triggers:    types.MapNull(types.StringType),
		Timeout:     types.Int64Value(120),
		CompletedAt: types.StringValue("2030-01-01T00:00:00Z"),
	}
	planned := prior
	planned.Timeout = types.Int64Value(300)

	null := tftypes.NewValue(s.Type().TerraformType(ctx), nil)
	state := tfsdk.State{Schema: s, Raw: null}
	plan := tfsdk.Plan{Schema: s, Raw: null}
	if d := state.Set(ctx, &prior); d.HasError() {
		t.Fatal(d)
	}
	if d := plan.Set(ctx, &planned); d.HasError() {
		t.Fatal(d)
	}

	resp := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics = %v", resp.Diagnostics)
	}
	var got RestartResourceModel
	if d := resp.State.Get(ctx, &got); d.HasError() {
		t.Fatal(d)
	}
	if !got.Timeout.Equal(planned.Timeout) || !got.ID.Equal(prior.ID) || !got.CompletedAt.Equal(prior.CompletedAt) {
		t.Errorf("state after Update = %+v, want the plan %+v", got, planned)
	}
}
