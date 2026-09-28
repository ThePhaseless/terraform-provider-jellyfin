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

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

const (
	bookshelfID = "9c4e63f1031b4f25988b4f7d78a8b53e"
	tmdbID      = "b8715ed16c4745289ad3f72deb539cd4"
)

const requestOverlapWindow = 20 * time.Millisecond

// fakePluginServer fakes Jellyfin's plugin endpoints: versioned DELETE (204
// no-op for bundled, 404 unlisted), 400 on overlap.
type fakePluginServer struct {
	mu          sync.Mutex
	plugins     []client.InstalledPlugin
	packages    []client.PackageInfo
	inFlight    int
	maxInFlight int
	deletes     int
	// keep answers DELETE without removing even a version users may
	// uninstall.
	keep bool
	// status, when set, replaces the 204 DELETE answers with.
	status int
	// packagesStatus, when set, is what GET /Packages answers instead of the
	// packages.
	packagesStatus int
	// installStatus, when set, replaces the 204 an install answers with.
	installStatus int
}

func (f *fakePluginServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/Plugins":
		f.mu.Lock()
		defer f.mu.Unlock()
		if err := json.NewEncoder(w).Encode(f.plugins); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/Packages":
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.packagesStatus != 0 {
			w.WriteHeader(f.packagesStatus)
			return
		}
		if err := json.NewEncoder(w).Encode(f.packages); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/Plugins/"),
		r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/Packages/Installed/"):
		f.mu.Lock()
		f.inFlight++
		f.maxInFlight = max(f.maxInFlight, f.inFlight)
		f.mu.Unlock()

		time.Sleep(requestOverlapWindow)

		f.mu.Lock()
		defer f.mu.Unlock()
		status := http.StatusNoContent
		switch {
		case r.Method == http.MethodDelete:
			status = f.uninstall(strings.TrimPrefix(r.URL.Path, "/Plugins/"))
		case f.installStatus != 0:
			status = f.installStatus
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

func (f *fakePluginServer) uninstall(idAndVersion string) int {
	f.deletes++
	id, version, _ := strings.Cut(idAndVersion, "/")
	for i, p := range f.plugins {
		if normalizeGUID(p.ID) != normalizeGUID(id) || p.Version != version {
			continue
		}
		if !p.CanUninstall {
			return http.StatusNoContent
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

func pluginResourceNullState(ctx context.Context) tfsdk.State {
	s := schemaOf(NewPluginResource())
	return tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
}

func pluginResourceState(t *testing.T, m PluginResourceModel) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: schemaOf(NewPluginResource())}
	if diags := state.Set(t.Context(), &m); diags.HasError() {
		t.Fatalf("state: %v", diags.Errors())
	}
	return state
}

func TestUnitPluginInstallsAndUninstallsDoNotOverlap(t *testing.T) {
	fake := &fakePluginServer{}
	server := httptest.NewServer(fake)
	defer server.Close()

	ctx := t.Context()
	var wg sync.WaitGroup
	for i := range 8 {
		c := client.NewClient(server.URL, "test-key")
		wg.Go(func() {
			var err error
			if i%2 == 0 {
				err = c.UninstallPluginVersion(ctx, bookshelfID, "13.0.0.0")
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
		{ID: bookshelfID, Name: "Bookshelf", Version: "12.0.0.0", Status: "Superseded", CanUninstall: true},
		{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: "Restart", CanUninstall: true},
		{ID: "170a157fac6c437aabddca9c25cebd39", Name: "Fanart", Version: "15.0.0.0", Status: "Active", CanUninstall: true},
	}}
	r := newFakePluginResource(t, fake)

	if _, err := r.uninstall(t.Context(), bookshelfID); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if got := fake.listed(); len(got) != 1 || got[0].Name != "Fanart" {
		t.Errorf("plugins left = %+v, want only Fanart", got)
	}
}

func TestUnitPluginUninstallSkipsVersionPendingDeletion(t *testing.T) {
	fake := &fakePluginServer{plugins: []client.InstalledPlugin{
		{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: pluginStatusDeleted, CanUninstall: true},
	}}
	r := newFakePluginResource(t, fake)

	if _, err := r.uninstall(t.Context(), bookshelfID); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if fake.deletes != 0 {
		t.Errorf("DELETE requests = %d, want 0", fake.deletes)
	}
}

func TestUnitPluginUninstallAcceptsFailureOncePluginIsGone(t *testing.T) {
	fake := &fakePluginServer{
		plugins: []client.InstalledPlugin{{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: "Restart", CanUninstall: true}},
		status:  http.StatusBadRequest,
	}
	r := newFakePluginResource(t, fake)

	if _, err := r.uninstall(t.Context(), bookshelfID); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
}

func TestUnitPluginUninstallReportsFailureWhilePluginIsListed(t *testing.T) {
	fake := &fakePluginServer{
		plugins: []client.InstalledPlugin{{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: "Restart", CanUninstall: true}},
		keep:    true,
		status:  http.StatusBadRequest,
	}
	r := newFakePluginResource(t, fake)

	_, err := r.uninstall(t.Context(), bookshelfID)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("uninstall error = %v, want the 400 Jellyfin answered", err)
	}
}

// A 404 while the plugin is still listed is a request that failed, such as
// one that raced an uninstall from another process and found meta.json gone.
func TestUnitPluginUninstallReportsNotFoundWhilePluginIsListed(t *testing.T) {
	fake := &fakePluginServer{
		plugins: []client.InstalledPlugin{{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: "Active", CanUninstall: true}},
		keep:    true,
		status:  http.StatusNotFound,
	}
	r := newFakePluginResource(t, fake)

	_, err := r.uninstall(t.Context(), bookshelfID)
	if !client.IsNotFound(err) {
		t.Fatalf("uninstall error = %v, want the 404 Jellyfin answered", err)
	}
}

func TestUnitPluginUninstallLeavesBundledPlugin(t *testing.T) {
	fake := &fakePluginServer{plugins: []client.InstalledPlugin{
		{ID: tmdbID, Name: "TMDb", Version: "12.1.0.0", Status: "Active"},
	}}
	r := newFakePluginResource(t, fake)

	kept, err := r.uninstall(t.Context(), tmdbID)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(kept) != 1 || kept[0].Version != "12.1.0.0" {
		t.Errorf("kept = %+v, want TMDb 12.1.0.0", kept)
	}
	if fake.deletes != 0 {
		t.Errorf("DELETE requests = %d, want 0", fake.deletes)
	}
}

func TestUnitPluginUninstallRemovesVersionInstalledOverBundledPlugin(t *testing.T) {
	fake := &fakePluginServer{plugins: []client.InstalledPlugin{
		{ID: tmdbID, Name: "TMDb", Version: "12.1.0.0", Status: "Superseded"},
		{ID: tmdbID, Name: "TMDb", Version: "12.2.0.0", Status: "Restart", CanUninstall: true},
	}}
	r := newFakePluginResource(t, fake)

	kept, err := r.uninstall(t.Context(), tmdbID)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(kept) != 1 || kept[0].Version != "12.1.0.0" {
		t.Errorf("kept = %+v, want TMDb 12.1.0.0", kept)
	}
	if got := fake.listed(); len(got) != 1 || got[0].Version != "12.1.0.0" {
		t.Errorf("plugins left = %+v, want only TMDb 12.1.0.0", got)
	}
}

func TestUnitPluginDeleteWarnsWhenJellyfinKeepsBundledPlugin(t *testing.T) {
	fake := &fakePluginServer{plugins: []client.InstalledPlugin{
		{ID: tmdbID, Name: "TMDb", Version: "12.1.0.0", Status: "Active"},
	}}
	r := newFakePluginResource(t, fake)
	ctx := t.Context()

	state := pluginResourceState(t, PluginResourceModel{
		ID:               types.StringValue(tmdbID),
		Name:             types.StringValue("TMDb"),
		Version:          types.StringValue("12.1.0.0"),
		InstalledVersion: types.StringValue("12.1.0.0"),
		RepositoryURL:    types.StringNull(),
	})
	resp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", resp.Diagnostics.Errors())
	}
	warnings := resp.Diagnostics.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0].Detail(), "TMDb 12.1.0.0") {
		t.Errorf("warnings = %v, want one naming TMDb 12.1.0.0", warnings)
	}
}

func TestUnitPluginUninstallStopsWhenJellyfinKeepsPlugin(t *testing.T) {
	fake := &fakePluginServer{
		plugins: []client.InstalledPlugin{{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: "Active", CanUninstall: true}},
		keep:    true,
	}
	r := newFakePluginResource(t, fake)

	_, err := r.uninstall(t.Context(), bookshelfID)
	if err == nil || !strings.Contains(err.Error(), "13.0.0.0") {
		t.Fatalf("uninstall error = %v, want one naming the version still listed", err)
	}
	if fake.deletes != 1 {
		t.Errorf("DELETE requests = %d, want 1", fake.deletes)
	}
}

func TestUnitPluginConcurrentUninstallsOfOnePluginSucceed(t *testing.T) {
	fake := &fakePluginServer{plugins: []client.InstalledPlugin{
		{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: "Restart", CanUninstall: true},
	}}
	server := httptest.NewServer(fake)
	defer server.Close()

	ctx := t.Context()
	var wg sync.WaitGroup
	for i := range 4 {
		r := &PluginResource{client: client.NewClient(server.URL, "test-key")}
		wg.Go(func() {
			if _, err := r.uninstall(ctx, bookshelfID); err != nil {
				t.Errorf("uninstall %d: %v", i, err)
			}
		})
	}
	wg.Wait()

	if got := fake.listed(); len(got) != 0 {
		t.Errorf("plugins left = %+v, want none", got)
	}
}

// replacePluginVersionFirst creates Bookshelf 13 via creator, then destroys 12
// via destroyer (create_before_destroy order).
func replacePluginVersionFirst(t *testing.T, creator, destroyer *PluginResource) {
	t.Helper()
	ctx := t.Context()

	createResp := &resource.CreateResponse{State: pluginResourceNullState(ctx)}
	creator.Create(ctx, resource.CreateRequest{Plan: pluginResourcePlan(t, plannedBookshelf("13.0.0.0"))}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}

	replaced := pluginResourceState(t, installedBookshelf("12.0.0.0", "12.0.0.0"))
	deleteResp := &resource.DeleteResponse{State: replaced}
	destroyer.Delete(ctx, resource.DeleteRequest{State: replaced}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}
}

// bookshelfAfterUpdate lists Bookshelf as Jellyfin does once 13.0.0.0 is
// installed over a loaded 12.0.0.0.
func bookshelfAfterUpdate() *fakePluginServer {
	return &fakePluginServer{plugins: []client.InstalledPlugin{
		{ID: bookshelfID, Name: "Bookshelf", Version: "12.0.0.0", Status: "Superseded", CanUninstall: true},
		{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: "Restart", CanUninstall: true},
	}}
}

func TestUnitPluginDeleteLeavesVersionCreatedThroughSameClient(t *testing.T) {
	fake := bookshelfAfterUpdate()
	r := newFakePluginResource(t, fake)

	replacePluginVersionFirst(t, r, &PluginResource{client: r.client})

	if got := fake.listed(); len(got) != 1 || got[0].Version != "13.0.0.0" {
		t.Errorf("plugins left = %+v, want only Bookshelf 13.0.0.0", got)
	}
}

func TestUnitPluginDeleteThroughAnotherClientRemovesCreatedVersion(t *testing.T) {
	fake := bookshelfAfterUpdate()
	r := newFakePluginResource(t, fake)
	other := &PluginResource{client: client.NewClient(r.client.BaseURL, r.client.APIKey)}

	replacePluginVersionFirst(t, r, other)

	if got := fake.listed(); len(got) != 0 {
		t.Errorf("plugins left = %+v, want none", got)
	}
}

func TestUnitPluginWaitIgnoresVersionPendingDeletion(t *testing.T) {
	fake := &fakePluginServer{plugins: []client.InstalledPlugin{
		{ID: bookshelfID, Name: "Bookshelf", Version: "13.0.0.0", Status: pluginStatusDeleted, CanUninstall: true},
	}}
	r := newFakePluginResource(t, fake)

	if p, err := r.waitForPlugin(t.Context(), "Bookshelf", "13.0.0.0", time.Millisecond); err == nil {
		t.Fatalf("waitForPlugin returned %+v, want an error while only a version pending deletion is listed", p)
	}
}

// Jellyfin answers an install with 404 when no enabled repository offers the
// package at that version for the server, which Create reports at once
// instead of waiting for a plugin that never appears.
func TestUnitPluginCreateReportsAVersionNoRepositoryOffers(t *testing.T) {
	fake := &fakePluginServer{installStatus: http.StatusNotFound}
	r := newFakePluginResource(t, fake)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	resp := &resource.CreateResponse{State: pluginResourceNullState(ctx)}
	r.Create(ctx, resource.CreateRequest{Plan: pluginResourcePlan(t, plannedBookshelf("13.0.0"))}, resp)
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "No enabled plugin repository offers Bookshelf 13.0.0") {
		t.Errorf("Create diagnostics = %v, want the version reported as not offered", resp.Diagnostics)
	}
	if ctx.Err() != nil {
		t.Error("Create waited for the plugin instead of reporting the 404")
	}
}

func TestUnitPluginWaitStopsWhenCancelled(t *testing.T) {
	r := newFakePluginResource(t, &fakePluginServer{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	start := time.Now()
	if _, err := r.waitForPlugin(ctx, "Bookshelf", "13.0.0.0", time.Minute); err == nil {
		t.Fatal("waitForPlugin found a plugin nothing lists")
	}
	if elapsed := time.Since(start); elapsed >= pluginPollInterval {
		t.Errorf("waitForPlugin returned after %s, not as soon as the context was cancelled", elapsed)
	}
}
