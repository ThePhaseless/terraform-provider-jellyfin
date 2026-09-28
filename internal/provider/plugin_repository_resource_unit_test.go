// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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

type repositoryServer struct {
	delay time.Duration

	mu     sync.Mutex
	repos  []client.PluginRepository
	posted []string
}

func (f *repositoryServer) client(t *testing.T) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Repositories" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			f.mu.Lock()
			served := slices.Clone(f.repos)
			f.mu.Unlock()
			time.Sleep(f.delay)
			_ = json.NewEncoder(w).Encode(served)
		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			var posted []client.PluginRepository
			if err == nil {
				err = json.Unmarshal(body, &posted)
			}
			if err != nil {
				t.Error(err)
			}
			f.mu.Lock()
			f.repos = posted
			f.posted = append(f.posted, string(body))
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	return client.NewClient(srv.URL, "k")
}

func repositoryPlan(t *testing.T, repo client.PluginRepository) tfsdk.Plan {
	t.Helper()
	ctx := t.Context()
	s := schemaOf(&PluginRepositoryResource{})
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if d := plan.Set(ctx, &PluginRepositoryResourceModel{
		ID:      types.StringValue(repo.Name),
		Name:    types.StringValue(repo.Name),
		URL:     types.StringValue(repo.URL),
		Enabled: types.BoolValue(repo.Enabled),
	}); d.HasError() {
		t.Fatal(d)
	}
	return plan
}

func repositoriesJSON(t *testing.T, repos ...client.PluginRepository) string {
	t.Helper()
	body, err := json.Marshal(append([]client.PluginRepository{}, repos...))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// Jellyfin replaces the whole repository list on each write, so repositories
// created at the same time must not drop each other.
func TestUnitPluginRepositoryCreatesKeepEachOther(t *testing.T) {
	// Answer slowly, so a create that does not wait for the other reads the
	// list before the other writes it.
	srv := &repositoryServer{delay: 20 * time.Millisecond}
	c := srv.client(t)

	var wg sync.WaitGroup
	for i := range 3 {
		name := fmt.Sprintf("repo-%d", i)
		plan := repositoryPlan(t, client.PluginRepository{Name: name, URL: "https://" + name, Enabled: true})
		wg.Go(func() {
			r := &PluginRepositoryResource{client: c}
			resp := resource.CreateResponse{State: tfsdk.State(plan)}
			r.Create(t.Context(), resource.CreateRequest{Plan: plan}, &resp)
			if resp.Diagnostics.HasError() {
				t.Error(resp.Diagnostics)
			}
		})
	}
	wg.Wait()

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.repos) != 3 {
		t.Errorf("the server holds %v, want the three repositories", srv.repos)
	}
}

func TestUnitPluginRepositoryWritesTheServedList(t *testing.T) {
	a := client.PluginRepository{Name: "a", URL: "https://a", Enabled: true}
	b := client.PluginRepository{Name: "b", URL: "https://b", Enabled: true}
	c := client.PluginRepository{Name: "c", URL: "https://c"}
	moved := client.PluginRepository{Name: "b", URL: "https://b2"}
	renamed := client.PluginRepository{Name: "d", URL: "https://b", Enabled: true}
	taken := client.PluginRepository{Name: "a", URL: "https://b", Enabled: true}
	const exists = "Plugin repository already exists"

	for _, test := range []struct {
		name        string
		served      []client.PluginRepository
		op          string
		prior, plan client.PluginRepository
		wantError   string
		wantPosted  []string
	}{
		{name: "create appends", served: []client.PluginRepository{a}, op: "create", plan: b, wantPosted: []string{repositoriesJSON(t, a, b)}},
		{name: "create refuses a taken name", served: []client.PluginRepository{a, b}, op: "create", plan: taken, wantError: exists},
		{name: "update keeps its name", served: []client.PluginRepository{a, b, c}, op: "update", prior: b, plan: moved, wantPosted: []string{repositoriesJSON(t, a, moved, c)}},
		{name: "update renames", served: []client.PluginRepository{a, b, c}, op: "update", prior: b, plan: renamed, wantPosted: []string{repositoriesJSON(t, a, renamed, c)}},
		{name: "update refuses a taken name", served: []client.PluginRepository{a, b, c}, op: "update", prior: b, plan: taken, wantError: exists},
		{name: "delete removes only its entry", served: []client.PluginRepository{a, b, c}, op: "delete", prior: b, wantPosted: []string{repositoriesJSON(t, a, c)}},
		{name: "delete of the last entry posts an empty list", served: []client.PluginRepository{a}, op: "delete", prior: a, wantPosted: []string{`[]`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			srv := &repositoryServer{repos: test.served}
			r := &PluginRepositoryResource{client: srv.client(t)}
			prior := tfsdk.State(repositoryPlan(t, test.prior))
			var diags diag.Diagnostics
			var state tfsdk.State
			switch test.op {
			case "create":
				plan := repositoryPlan(t, test.plan)
				resp := resource.CreateResponse{State: tfsdk.State(plan)}
				r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
				diags, state = resp.Diagnostics, resp.State
			case "update":
				plan := repositoryPlan(t, test.plan)
				resp := resource.UpdateResponse{State: prior}
				r.Update(ctx, resource.UpdateRequest{Plan: plan, State: prior}, &resp)
				diags, state = resp.Diagnostics, resp.State
			case "delete":
				resp := resource.DeleteResponse{State: prior}
				r.Delete(ctx, resource.DeleteRequest{State: prior}, &resp)
				diags = resp.Diagnostics
			}

			if test.wantError == "" && diags.HasError() {
				t.Fatalf("%s: %v", test.op, diags)
			}
			if test.wantError != "" && (diags.ErrorsCount() != 1 || diags.Errors()[0].Summary() != test.wantError) {
				t.Fatalf("%s: %v, want the error %q", test.op, diags, test.wantError)
			}
			if !slices.Equal(srv.posted, test.wantPosted) {
				t.Errorf("posted %q, want %q", srv.posted, test.wantPosted)
			}
			if test.op == "delete" || test.wantError != "" {
				return
			}
			var got, want PluginRepositoryResourceModel
			if d := state.Get(ctx, &got); d.HasError() {
				t.Fatal(d)
			}
			if d := repositoryPlan(t, test.plan).Get(ctx, &want); d.HasError() {
				t.Fatal(d)
			}
			if got != want {
				t.Errorf("state %+v, want %+v", got, want)
			}
		})
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
