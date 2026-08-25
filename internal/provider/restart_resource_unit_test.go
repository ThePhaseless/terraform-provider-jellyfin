// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"Id": "s", "HasPendingRestart": hasPending})
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
	if err := awaitRestart(context.Background(), c, 5*time.Second, testPoll); err != nil {
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
	if err := awaitRestart(context.Background(), c, 400*time.Millisecond, testPoll); err == nil {
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
	if err := awaitRestart(context.Background(), c, 5*time.Second, testPoll); err != nil {
		t.Fatalf("awaitRestart() error = %v", err)
	}
}

func TestAwaitRestartHonoursCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(systemInfoHandler(func() (bool, bool) { return true, false }))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := client.NewClient(server.URL, "k")
	if err := awaitRestart(ctx, c, 5*time.Second, testPoll); err == nil {
		t.Fatal("expected the cancelled context to abort the wait")
	}
}
