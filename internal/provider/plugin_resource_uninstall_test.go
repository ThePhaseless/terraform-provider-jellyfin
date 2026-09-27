// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

const bookshelfID = "9c4e63f1031b4f25988b4f7d78a8b53e"

// fakePluginServer stands in for Jellyfin's plugin endpoints. DELETE
// /Plugins/{id} removes the last listed version of the plugin, and answers 400
// while another install or uninstall is in flight, as Jellyfin's unsynchronised
// plugin list can.
type fakePluginServer struct {
	mu          sync.Mutex
	plugins     []client.InstalledPlugin
	inFlight    int
	maxInFlight int
	deletes     int
	// keep answers DELETE without removing anything, as Jellyfin does for a
	// plugin it does not let users uninstall.
	keep bool
	// status, when set, replaces the 204 DELETE answers with.
	status int
}

func (f *fakePluginServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/Plugins":
		f.mu.Lock()
		defer f.mu.Unlock()
		if err := json.NewEncoder(w).Encode(f.plugins); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/Plugins/"),
		r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/Packages/Installed/"):
		f.mu.Lock()
		f.inFlight++
		f.maxInFlight = max(f.maxInFlight, f.inFlight)
		f.mu.Unlock()

		// Long enough for requests sent without waiting on each other to overlap.
		time.Sleep(20 * time.Millisecond)

		f.mu.Lock()
		defer f.mu.Unlock()
		status := http.StatusNoContent
		if r.Method == http.MethodDelete {
			status = f.uninstall(strings.TrimPrefix(r.URL.Path, "/Plugins/"))
		}
		if f.inFlight > 1 {
			status = http.StatusBadRequest
		}
		f.inFlight--
		w.WriteHeader(status)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakePluginServer) uninstall(id string) int {
	f.deletes++
	for i := len(f.plugins) - 1; i >= 0; i-- {
		if f.plugins[i].ID != id {
			continue
		}
		if !f.keep {
			f.plugins = append(f.plugins[:i], f.plugins[i+1:]...)
		}
		if f.status != 0 {
			return f.status
		}
		return http.StatusNoContent
	}
	return http.StatusNotFound
}

func (f *fakePluginServer) listed() []client.InstalledPlugin {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]client.InstalledPlugin(nil), f.plugins...)
}

func newFakePluginResource(t *testing.T, fake *fakePluginServer) *PluginResource {
	t.Helper()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return &PluginResource{client: client.NewClient(server.URL, "test-key")}
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
				err = c.UninstallPlugin(ctx, bookshelfID)
			} else {
				err = c.InstallPlugin(ctx, "Bookshelf", "13.0.0.0", "https://repo.example/manifest.json")
			}
			if err != nil && !client.IsNotFound(err) {
				t.Errorf("request %d: %v", i, err)
			}
		})
	}
	wg.Wait()

	if fake.maxInFlight != 1 {
		t.Errorf("Jellyfin served %d plugin changes at once, want 1", fake.maxInFlight)
	}
}

func TestUnitPluginUninstallRemovesEveryListedVersion(t *testing.T) {
	fake := &fakePluginServer{plugins: []client.InstalledPlugin{
		{ID: bookshelfID, Name: "Bookshelf", Version: "12.0.0.0", Status: "Superseded"},
		{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: "Restart"},
		{ID: "170a157fac6c437aabddca9c25cebd39", Name: "Fanart", Version: "15.0.0.0", Status: "Active"},
	}}
	r := newFakePluginResource(t, fake)

	if err := r.uninstall(context.Background(), bookshelfID); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if got := fake.listed(); len(got) != 1 || got[0].Name != "Fanart" {
		t.Errorf("plugins left = %+v, want only Fanart", got)
	}
}

func TestUnitPluginUninstallSkipsVersionPendingDeletion(t *testing.T) {
	fake := &fakePluginServer{plugins: []client.InstalledPlugin{
		{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: pluginStatusDeleted},
	}}
	r := newFakePluginResource(t, fake)

	if err := r.uninstall(context.Background(), bookshelfID); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if fake.deletes != 0 {
		t.Errorf("DELETE requests = %d, want 0", fake.deletes)
	}
}

func TestUnitPluginUninstallAcceptsFailureOncePluginIsGone(t *testing.T) {
	fake := &fakePluginServer{
		plugins: []client.InstalledPlugin{{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: "Restart"}},
		status:  http.StatusBadRequest,
	}
	r := newFakePluginResource(t, fake)

	if err := r.uninstall(context.Background(), bookshelfID); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
}

func TestUnitPluginUninstallReportsFailureWhilePluginIsListed(t *testing.T) {
	fake := &fakePluginServer{
		plugins: []client.InstalledPlugin{{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: "Restart"}},
		keep:    true,
		status:  http.StatusBadRequest,
	}
	r := newFakePluginResource(t, fake)

	err := r.uninstall(context.Background(), bookshelfID)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("uninstall error = %v, want the 400 Jellyfin answered", err)
	}
}

func TestUnitPluginUninstallStopsWhenJellyfinKeepsPlugin(t *testing.T) {
	fake := &fakePluginServer{
		plugins: []client.InstalledPlugin{{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: "Active"}},
		keep:    true,
	}
	r := newFakePluginResource(t, fake)

	err := r.uninstall(context.Background(), bookshelfID)
	if err == nil || !strings.Contains(err.Error(), "13.0.0.0") {
		t.Fatalf("uninstall error = %v, want one naming the version still listed", err)
	}
	if fake.deletes != 1 {
		t.Errorf("DELETE requests = %d, want 1", fake.deletes)
	}
}

func TestUnitPluginConcurrentUninstallsOfOnePluginSucceed(t *testing.T) {
	fake := &fakePluginServer{plugins: []client.InstalledPlugin{
		{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: "Restart"},
	}}
	server := httptest.NewServer(fake)
	defer server.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := range 4 {
		r := &PluginResource{client: client.NewClient(server.URL, "test-key")}
		wg.Go(func() {
			if err := r.uninstall(ctx, bookshelfID); err != nil {
				t.Errorf("uninstall %d: %v", i, err)
			}
		})
	}
	wg.Wait()

	if got := fake.listed(); len(got) != 0 {
		t.Errorf("plugins left = %+v, want none", got)
	}
}
