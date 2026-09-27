// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

// fakeJellyfin serves before at get until a POST to post, and from then on
// what after makes of the posted body.
type fakeJellyfin struct {
	get, post string
	before    string
	after     func(posted []byte) string

	mu     sync.Mutex
	posted []byte
}

func (f *fakeJellyfin) body() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posted
}

func serves(doc string) func([]byte) string {
	return func([]byte) string { return doc }
}

func (f *fakeJellyfin) client(t *testing.T) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == f.get:
			doc := f.before
			if f.posted != nil {
				doc = f.after(f.posted)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, doc)
		case r.Method == http.MethodPost && r.URL.Path == f.post:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("reading POST %s: %v", r.URL.Path, err)
			}
			f.posted = body
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return client.NewClient(srv.URL, "k")
}

type planValue struct {
	at path.Path
	v  attr.Value
}

// planRead reads doc through r's binding into a plan, as Terraform plans an
// unchanged resource from the state that read produced, and then sets each
// value of set.
func planRead(t *testing.T, r resource.Resource, doc string, set ...planValue) tfsdk.Plan {
	t.Helper()
	ctx := context.Background()
	bound, ok := r.(wireBound)
	if !ok {
		t.Fatalf("%T binds no Jellyfin document", r)
	}
	b, err := bound.Wire()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatalf("parsing %s: %v", doc, err)
	}
	obj, d := b.Flatten(ctx, m, types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatalf("read: %v", d)
	}
	raw, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	plan := tfsdk.Plan{Schema: schemaOf(r), Raw: raw}
	for _, s := range set {
		if d := plan.SetAttribute(ctx, s.at, s.v); d.HasError() {
			t.Fatalf("planning %s: %v", s.at, d)
		}
	}
	return plan
}

func updateAgainst(t *testing.T, r resource.Resource, c *client.Client, plan tfsdk.Plan) resource.UpdateResponse {
	t.Helper()
	ctx := context.Background()
	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatalf("%T takes no client", r)
	}
	var configured resource.ConfigureResponse
	configurable.Configure(ctx, resource.ConfigureRequest{ProviderData: c}, &configured)
	if configured.Diagnostics.HasError() {
		t.Fatalf("configure: %v", configured.Diagnostics)
	}
	resp := resource.UpdateResponse{State: tfsdk.State(plan)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: tfsdk.State(plan)}, &resp)
	return resp
}

func TestUnitApplyKeepsPlannedNullInsideListElementServedEmpty(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		resource  resource.Resource
		get, post string
		doc       string
		set       []planValue
		at        path.Path
		null      attr.Value
	}{
		{
			name: "system list", resource: NewSystemConfigurationResource(),
			get: "/System/Configuration", post: "/System/Configuration",
			doc:  `{"MetadataOptions":[{"ItemType":"Movie","DisabledMetadataSavers":[]}]}`,
			at:   path.Root("metadata_options").AtListIndex(0).AtName("disabled_metadata_savers"),
			null: types.ListNull(types.StringType),
		},
		{
			name: "livetv string", resource: NewLiveTVConfigurationResource(),
			get: "/System/Configuration/livetv", post: "/System/Configuration/livetv",
			doc:  `{"TunerHosts":[{"Url":"http://tuner","Type":"M3U","Source":""}]}`,
			at:   path.Root("tuner_hosts").AtListIndex(0).AtName("source"),
			null: types.StringNull(),
		},
		{
			name: "scheduled task string", resource: NewScheduledTaskResource(),
			get: "/ScheduledTasks/abc", post: "/ScheduledTasks/abc/Triggers",
			doc:  `{"Id":"abc","Triggers":[{"Type":"IntervalTrigger","IntervalTicks":1,"DayOfWeek":""}]}`,
			set:  []planValue{{path.Root("task_id"), types.StringValue("abc")}},
			at:   path.Root("triggers").AtListIndex(0).AtName("day_of_week"),
			null: types.StringNull(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			var served attr.Value
			plan := planRead(t, tc.resource, tc.doc, tc.set...)
			if d := plan.GetAttribute(ctx, tc.at, &served); d.HasError() || served.IsNull() {
				t.Fatalf("%s reads from the served document as %v (%v), want an empty value", tc.at, served, d)
			}
			if d := plan.SetAttribute(ctx, tc.at, tc.null); d.HasError() {
				t.Fatalf("planning %s: %v", tc.at, d)
			}

			srv := &fakeJellyfin{get: tc.get, post: tc.post, before: tc.doc, after: serves(tc.doc)}
			resp := updateAgainst(t, tc.resource, srv.client(t), plan)
			if resp.Diagnostics.HasError() {
				t.Fatalf("apply: %v", resp.Diagnostics)
			}
			var got attr.Value
			if d := resp.State.GetAttribute(ctx, tc.at, &got); d.HasError() {
				t.Fatal(d)
			}
			if !got.IsNull() {
				t.Errorf("%s = %v after apply, want the planned null", tc.at, got)
			}
		})
	}
}

func TestUnitApplyReportsPlannedValueReadBackAsNull(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		resource      resource.Resource
		get, post     string
		before, after string
		set           []planValue
		at            path.Path
	}{
		{
			name: "encoding", resource: NewEncodingConfigurationResource(),
			get: "/System/Configuration/encoding", post: "/System/Configuration/encoding",
			before: `{"EnableSubtitleExtraction":true,"SubtitleExtractionTimeoutMinutes":45}`,
			after:  `{"EnableSubtitleExtraction":true}`,
			at:     path.Root("subtitle_extraction_timeout_minutes"),
		},
		{
			name: "networking", resource: NewNetworkingConfigurationResource(),
			get: "/System/Configuration/network", post: "/System/Configuration/network",
			before: `{"EnableHttps":false,"BaseUrl":"/jellyfin"}`,
			after:  `{"EnableHttps":false,"BaseUrl":null}`,
			at:     path.Root("base_url"),
		},
		{
			name: "system", resource: NewSystemConfigurationResource(),
			get: "/System/Configuration", post: "/System/Configuration",
			before: `{"EnableMetrics":false,"PathSubstitutions":[{"From":"/a","To":"/b"}]}`,
			after:  `{"EnableMetrics":false,"PathSubstitutions":[{"From":"/a"}]}`,
			at:     path.Root("path_substitutions").AtListIndex(0).AtName("to"),
		},
		{
			name: "livetv", resource: NewLiveTVConfigurationResource(),
			get: "/System/Configuration/livetv", post: "/System/Configuration/livetv",
			before: `{"TunerHosts":[{"Url":"http://tuner","Type":"M3U","FriendlyName":"Tuner"}]}`,
			after:  `{"TunerHosts":[{"Url":"http://tuner","Type":"M3U","FriendlyName":null}]}`,
			at:     path.Root("tuner_hosts").AtListIndex(0).AtName("friendly_name"),
		},
		{
			name: "scheduled task", resource: NewScheduledTaskResource(),
			get: "/ScheduledTasks/abc", post: "/ScheduledTasks/abc/Triggers",
			before: `{"Id":"abc","Triggers":[{"Type":"DailyTrigger","TimeOfDayTicks":1,"MaxRuntimeTicks":2}]}`,
			after:  `{"Id":"abc","Triggers":[{"Type":"DailyTrigger","TimeOfDayTicks":1}]}`,
			set:    []planValue{{path.Root("task_id"), types.StringValue("abc")}},
			at:     path.Root("triggers").AtListIndex(0).AtName("max_runtime_ticks"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := &fakeJellyfin{get: tc.get, post: tc.post, before: tc.before, after: serves(tc.after)}
			resp := updateAgainst(t, tc.resource, srv.client(t), planRead(t, tc.resource, tc.before, tc.set...))

			errs := resp.Diagnostics.Errors()
			if len(errs) != 1 {
				t.Fatalf("apply reported %v, want one error at %s", resp.Diagnostics, tc.at)
			}
			withPath, ok := errs[0].(diag.DiagnosticWithPath)
			if !ok || !withPath.Path().Equal(tc.at) || errs[0].Summary() != "Value not kept by Jellyfin" {
				t.Errorf("apply reported %v, want %q at %s", errs[0], "Value not kept by Jellyfin", tc.at)
			}
		})
	}
}
