// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

// fakePluginServer stands in for Jellyfin's plugin endpoints and records how
// many install and uninstall requests it serves at once.
type fakePluginServer struct {
	mu          sync.Mutex
	inFlight    int
	maxInFlight int
}

func (f *fakePluginServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/Plugins/"),
		r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/Packages/Installed/"):
		f.mu.Lock()
		f.inFlight++
		f.maxInFlight = max(f.maxInFlight, f.inFlight)
		f.mu.Unlock()

		// Long enough for requests sent without waiting on each other to overlap.
		time.Sleep(20 * time.Millisecond)

		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func TestUnitPluginInstallsAndUninstallsDoNotOverlap(t *testing.T) {
	fake := &fakePluginServer{}
	server := httptest.NewServer(fake)
	defer server.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := range 8 {
		// Separate clients, as separate provider instances in one process have.
		c := client.NewClient(server.URL, "test-key")
		wg.Go(func() {
			var err error
			if i%2 == 0 {
				err = c.UninstallPlugin(ctx, "9c4e63f1031b4f25988b4f7d78a8b53e")
			} else {
				err = c.InstallPlugin(ctx, "Bookshelf", "13.0.0.0", "https://repo.example/manifest.json")
			}
			if err != nil {
				t.Errorf("request %d: %v", i, err)
			}
		})
	}
	wg.Wait()

	if fake.maxInFlight != 1 {
		t.Errorf("Jellyfin served %d plugin changes at once, want 1", fake.maxInFlight)
	}
}
