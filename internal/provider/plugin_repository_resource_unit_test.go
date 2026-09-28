// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func TestFindPluginRepositoryIndex(t *testing.T) {
	t.Parallel()

	repos := []client.PluginRepository{
		{Name: "stable", URL: "https://stable.example/manifest.json", Enabled: true},
		{Name: "testing", URL: "https://testing.example/manifest.json", Enabled: true},
		{Name: "stable", URL: "https://mirror.example/manifest.json", Enabled: true},
	}

	tests := []struct {
		name      string
		repos     []client.PluginRepository
		repoName  string
		repoURL   string
		wantIndex int
		wantErr   bool
	}{
		{
			name:      "unique name matches directly",
			repos:     repos[:2],
			repoName:  "testing",
			wantIndex: 1,
		},
		{
			name:      "duplicate names can be disambiguated by URL",
			repos:     repos,
			repoName:  "stable",
			repoURL:   "https://mirror.example/manifest.json",
			wantIndex: 2,
		},
		{
			name:     "duplicate names without URL are ambiguous",
			repos:    repos,
			repoName: "stable",
			wantErr:  true,
		},
		{
			name:      "missing repository returns sentinel index",
			repos:     repos[:2],
			repoName:  "missing",
			wantIndex: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			index, err := findPluginRepositoryIndex(tt.repos, tt.repoName, tt.repoURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if index != tt.wantIndex {
				t.Fatalf("expected index %d, got %d", tt.wantIndex, index)
			}
		})
	}
}

// Jellyfin replaces the whole repository list on each write, so repositories
// created at the same time must not drop each other.
func TestUnitPluginRepositoryCreatesKeepEachOther(t *testing.T) {
	var mu sync.Mutex
	repos := []client.PluginRepository{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			mu.Lock()
			served := append([]client.PluginRepository{}, repos...)
			mu.Unlock()
			// Answer slowly, so a create that does not wait for the other
			// reads the list before the other writes it.
			time.Sleep(20 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(served)
		case http.MethodPost:
			var posted []client.PluginRepository
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Error(err)
			}
			mu.Lock()
			repos = posted
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	s := schemaOf(&PluginRepositoryResource{})
	var wg sync.WaitGroup
	for i := range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := &PluginRepositoryResource{client: client.NewClient(srv.URL, "k")}
			plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
			name := fmt.Sprintf("repo-%d", i)
			if d := plan.Set(ctx, &PluginRepositoryResourceModel{ID: types.StringValue(name), Name: types.StringValue(name), URL: types.StringValue("https://" + name), Enabled: types.BoolValue(true)}); d.HasError() {
				t.Error(d)
				return
			}
			resp := resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: plan.Raw}}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
			if resp.Diagnostics.HasError() {
				t.Error(resp.Diagnostics)
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(repos) != 3 {
		t.Errorf("the server holds %v, want the three repositories", repos)
	}
}

// A rename plans the id Update stores, the new name.
func TestUnitPluginRepositoryIDFollowsName(t *testing.T) {
	ctx := context.Background()
	s := schemaOf(&PluginRepositoryResource{})
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if d := plan.Set(ctx, &PluginRepositoryResourceModel{ID: types.StringUnknown(), Name: types.StringValue("new"), URL: types.StringValue("u"), Enabled: types.BoolValue(true)}); d.HasError() {
		t.Fatal(d)
	}
	resp := planmodifier.StringResponse{PlanValue: types.StringUnknown()}
	idFollowsName{}.PlanModifyString(ctx, planmodifier.StringRequest{Path: path.Root("id"), Plan: plan, PlanValue: types.StringUnknown(), StateValue: types.StringValue("old")}, &resp)
	if resp.Diagnostics.HasError() || resp.PlanValue.ValueString() != "new" {
		t.Errorf("planned id = %s (%v), want the new name", resp.PlanValue, resp.Diagnostics)
	}
}
