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

func TestAwaitRestartRejectsServerThatNeverGoesDown(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(systemInfoHandler(func() (bool, bool) { return false, false }))
	defer server.Close()

	c := client.NewClient(server.URL, "k")
	err := awaitRestart(context.Background(), c, 300*time.Millisecond, testPoll)
	if err == nil {
		t.Fatal("a server that keeps answering has not restarted; expected an error")
	}
}

func TestAwaitRestartWaitsForDownThenReady(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(systemInfoHandler(func() (bool, bool) {
		switch n := calls.Add(1); {
		case n <= 2:
			return false, false // still serving the pre-restart process
		case n <= 4:
			return true, false // restarting
		default:
			return false, false
		}
	}))
	defer server.Close()

	c := client.NewClient(server.URL, "k")
	if err := awaitRestart(context.Background(), c, 5*time.Second, testPoll); err != nil {
		t.Fatalf("awaitRestart() error = %v", err)
	}
	if got := calls.Load(); got < 5 {
		t.Fatalf("expected the wait to span the outage, got %d polls", got)
	}
}

func TestAwaitRestartTimesOutWhileRestartStillPending(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(systemInfoHandler(func() (bool, bool) {
		n := calls.Add(1)
		return n <= 2, true
	}))
	defer server.Close()

	c := client.NewClient(server.URL, "k")
	if err := awaitRestart(context.Background(), c, 300*time.Millisecond, testPoll); err == nil {
		t.Fatal("expected a timeout while HasPendingRestart stayed true")
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
