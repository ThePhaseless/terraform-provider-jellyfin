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

func TestWaitForServerReadyReturnsImmediatelyWhenNotPending(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"Id": "s", "HasPendingRestart": false})
	}))
	defer server.Close()

	c := client.NewClient(server.URL, "k")
	if err := waitForServerReady(context.Background(), c, 10*time.Second); err != nil {
		t.Fatalf("waitForServerReady() error = %v", err)
	}
}

func TestWaitForServerReadyPollsUntilNotPending(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pending := calls.Add(1) < 2 // pending on the first call, clear on the second
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"Id": "s", "HasPendingRestart": pending})
	}))
	defer server.Close()

	c := client.NewClient(server.URL, "k")
	if err := waitForServerReady(context.Background(), c, 10*time.Second); err != nil {
		t.Fatalf("waitForServerReady() error = %v", err)
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("expected at least 2 polls, got %d", got)
	}
}

func TestWaitForServerReadyTimesOut(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"Id": "s", "HasPendingRestart": true})
	}))
	defer server.Close()

	c := client.NewClient(server.URL, "k")
	err := waitForServerReady(context.Background(), c, 500*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}
