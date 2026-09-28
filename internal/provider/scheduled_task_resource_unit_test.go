// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

func scheduledTaskSchema(t *testing.T) rschema.Schema {
	t.Helper()

	var resp resource.SchemaResponse
	(&ScheduledTaskResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp.Schema
}

func scheduledTaskTriggerType(t *testing.T) attr.Type {
	t.Helper()

	listType, ok := scheduledTaskSchema(t).Attributes["triggers"].GetType().(types.ListType)
	if !ok {
		t.Fatalf("triggers type = %T, want types.ListType", scheduledTaskSchema(t).Attributes["triggers"].GetType())
	}
	return listType.ElemType
}

func newTrigger(typ string) ScheduledTaskTriggerModel {
	return ScheduledTaskTriggerModel{
		Type:            types.StringValue(typ),
		TimeOfDayTicks:  types.Int64Null(),
		IntervalTicks:   types.Int64Null(),
		DayOfWeek:       types.StringNull(),
		MaxRuntimeTicks: types.Int64Null(),
	}
}

func triggerList(t *testing.T, triggers ...ScheduledTaskTriggerModel) types.List {
	t.Helper()

	list, diags := types.ListValueFrom(context.Background(), scheduledTaskTriggerType(t), triggers)
	if diags.HasError() {
		t.Fatalf("building trigger list: %v", diags)
	}
	return list
}

func TestScheduledTaskTriggerAttributesAreNotComputed(t *testing.T) {
	t.Parallel()

	triggers, ok := scheduledTaskSchema(t).Attributes["triggers"].(rschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("triggers attribute type = %T, want schema.ListNestedAttribute", scheduledTaskSchema(t).Attributes["triggers"])
	}
	for name, a := range triggers.NestedObject.Attributes {
		if a.IsComputed() {
			t.Errorf("triggers.%s is computed; an omitted value would plan as unknown and never resolve", name)
		}
	}
}

func TestScheduledTaskTriggerTickValidators(t *testing.T) {
	t.Parallel()

	triggers, ok := scheduledTaskSchema(t).Attributes["triggers"].(rschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("triggers attribute type = %T, want schema.ListNestedAttribute", scheduledTaskSchema(t).Attributes["triggers"])
	}

	tests := map[string]struct {
		attribute string
		value     int64
		wantError bool
	}{
		"time_of_day_ticks at midnight":             {attribute: "time_of_day_ticks", value: 0},
		"time_of_day_ticks at the last tick of day": {attribute: "time_of_day_ticks", value: 863999999999},
		"time_of_day_ticks of one day":              {attribute: "time_of_day_ticks", value: 864000000000, wantError: true},
		"negative time_of_day_ticks":                {attribute: "time_of_day_ticks", value: -1, wantError: true},
		"zero interval_ticks":                       {attribute: "interval_ticks", value: 0},
		"negative interval_ticks":                   {attribute: "interval_ticks", value: -1, wantError: true},
		"zero max_runtime_ticks":                    {attribute: "max_runtime_ticks", value: 0},
		"negative max_runtime_ticks":                {attribute: "max_runtime_ticks", value: -1, wantError: true},
		"max_runtime_ticks at the limit":            {attribute: "max_runtime_ticks", value: 42949672949999},
		"max_runtime_ticks one tick past the limit": {attribute: "max_runtime_ticks", value: 42949672950000, wantError: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			a, ok := triggers.NestedObject.Attributes[tc.attribute].(rschema.Int64Attribute)
			if !ok {
				t.Fatalf("triggers.%s type = %T, want schema.Int64Attribute", tc.attribute, triggers.NestedObject.Attributes[tc.attribute])
			}

			var diags diag.Diagnostics
			for _, v := range a.Int64Validators() {
				var resp validator.Int64Response
				v.ValidateInt64(context.Background(), validator.Int64Request{
					Path:        path.Root("triggers").AtListIndex(0).AtName(tc.attribute),
					ConfigValue: types.Int64Value(tc.value),
				}, &resp)
				diags.Append(resp.Diagnostics...)
			}
			if diags.HasError() != tc.wantError {
				t.Fatalf("validating %s = %d: diagnostics %v, want error %t", tc.attribute, tc.value, diags, tc.wantError)
			}
		})
	}
}

var triggerSerialisationCases = map[string]struct {
	trigger ScheduledTaskTriggerModel
	json    string
}{
	"interval trigger without day_of_week": {
		trigger: func() ScheduledTaskTriggerModel {
			m := newTrigger(triggerTypeInterval)
			m.IntervalTicks = types.Int64Value(432000000000)
			return m
		}(),
		json: `{"Type":"IntervalTrigger","IntervalTicks":432000000000}`,
	},
	"daily trigger with max runtime": {
		trigger: func() ScheduledTaskTriggerModel {
			m := newTrigger(triggerTypeDaily)
			m.TimeOfDayTicks = types.Int64Value(72000000000)
			m.MaxRuntimeTicks = types.Int64Value(144000000000)
			return m
		}(),
		json: `{"Type":"DailyTrigger","TimeOfDayTicks":72000000000,"MaxRuntimeTicks":144000000000}`,
	},
	"weekly trigger": {
		trigger: func() ScheduledTaskTriggerModel {
			m := newTrigger(triggerTypeWeekly)
			m.TimeOfDayTicks = types.Int64Value(36000000000)
			m.DayOfWeek = types.StringValue("Tuesday")
			return m
		}(),
		json: `{"Type":"WeeklyTrigger","TimeOfDayTicks":36000000000,"DayOfWeek":"Tuesday"}`,
	},
	"startup trigger": {
		trigger: newTrigger(triggerTypeStartup),
		json:    `{"Type":"StartupTrigger"}`,
	},
	"zero ticks stay zero": {
		trigger: func() ScheduledTaskTriggerModel {
			m := newTrigger(triggerTypeInterval)
			m.IntervalTicks = types.Int64Value(0)
			m.MaxRuntimeTicks = types.Int64Value(0)
			return m
		}(),
		json: `{"Type":"IntervalTrigger","IntervalTicks":0,"MaxRuntimeTicks":0}`,
	},
}

func scheduledTaskBinding(t *testing.T) *wire.Binding {
	t.Helper()

	b, err := scheduledTaskWire()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestScheduledTaskApplyPostsTriggersWithoutNullAttributes(t *testing.T) {
	t.Parallel()

	type postCase struct {
		triggers types.List
		body     string
	}
	cases := map[string]postCase{
		"no triggers": {types.ListValueMust(scheduledTaskTriggerType(t), []attr.Value{}), `[]`},
	}
	for name, tc := range triggerSerialisationCases {
		cases[name] = postCase{triggerList(t, tc.trigger), `[` + tc.json + `]`}
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			plan := tfsdk.Plan{Schema: scheduledTaskSchema(t)}
			data := ScheduledTaskResourceModel{ID: types.StringNull(), TaskID: types.StringValue("abc"), Triggers: tc.triggers}
			if d := plan.Set(ctx, &data); d.HasError() {
				t.Fatalf("plan: %v", d)
			}
			srv := &fakeJellyfin{
				get: "/ScheduledTasks/abc", post: "/ScheduledTasks/abc/Triggers",
				before: `{"Id":"abc","Triggers":[{"Type":"StartupTrigger"}]}`,
				after:  func(posted []byte) string { return `{"Id":"abc","Triggers":` + string(posted) + `}` },
			}
			resp := updateAgainst(t, NewScheduledTaskResource(), srv.client(t), plan)
			if resp.Diagnostics.HasError() {
				t.Fatalf("apply: %v", resp.Diagnostics)
			}
			checkSameJSON(t, json.RawMessage(srv.body()), tc.body)
		})
	}
}

func TestScheduledTaskReadNullsAbsentTriggerAttributes(t *testing.T) {
	t.Parallel()

	for name, tc := range triggerSerialisationCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := readWire[ScheduledTaskResourceModel](t, scheduledTaskBinding(t), `{"Triggers":[`+tc.json+`]}`).Triggers
			if want := triggerList(t, tc.trigger); !got.Equal(want) {
				t.Fatalf("triggers = %v, want %v", got, want)
			}
		})
	}
}

func TestScheduledTaskReadTreatsExplicitJSONNullAsNull(t *testing.T) {
	t.Parallel()

	got := readWire[ScheduledTaskResourceModel](t, scheduledTaskBinding(t), `{"Triggers":[{"Type":"IntervalTrigger","IntervalTicks":1,"TimeOfDayTicks":null,"DayOfWeek":null,"MaxRuntimeTicks":null}]}`).Triggers

	want := newTrigger(triggerTypeInterval)
	want.IntervalTicks = types.Int64Value(1)
	if !got.Equal(triggerList(t, want)) {
		t.Fatalf("triggers = %v, want %v", got, triggerList(t, want))
	}
}

func TestScheduledTaskReadReturnsKnownEmptyListForNoTriggers(t *testing.T) {
	t.Parallel()

	for _, task := range []string{`{"Triggers":[]}`, `{"Triggers":null}`, `{}`} {
		got := readWire[ScheduledTaskResourceModel](t, scheduledTaskBinding(t), task).Triggers
		if got.IsNull() || got.IsUnknown() || len(got.Elements()) != 0 {
			t.Errorf("triggers read from %s = %v, want a known empty list", task, got)
		}
	}
}

func TestMissingTriggerAttributes(t *testing.T) {
	t.Parallel()

	withTimeOfDay := func(m ScheduledTaskTriggerModel) ScheduledTaskTriggerModel {
		m.TimeOfDayTicks = types.Int64Value(1)
		return m
	}

	tests := map[string]struct {
		trigger ScheduledTaskTriggerModel
		want    []string
	}{
		"daily without time_of_day_ticks": {trigger: newTrigger(triggerTypeDaily), want: []string{"time_of_day_ticks"}},
		"daily with time_of_day_ticks":    {trigger: withTimeOfDay(newTrigger(triggerTypeDaily))},
		"weekly without anything":         {trigger: newTrigger(triggerTypeWeekly), want: []string{"time_of_day_ticks", "day_of_week"}},
		"weekly without day_of_week":      {trigger: withTimeOfDay(newTrigger(triggerTypeWeekly)), want: []string{"day_of_week"}},
		"interval without interval_ticks": {trigger: newTrigger(triggerTypeInterval), want: []string{"interval_ticks"}},
		"startup needs nothing":           {trigger: newTrigger(triggerTypeStartup)},
		"unknown interval_ticks is not missing": {
			trigger: func() ScheduledTaskTriggerModel {
				m := newTrigger(triggerTypeInterval)
				m.IntervalTicks = types.Int64Unknown()
				return m
			}(),
		},
		"unknown type is not checked": {
			trigger: func() ScheduledTaskTriggerModel {
				m := newTrigger(triggerTypeWeekly)
				m.Type = types.StringUnknown()
				return m
			}(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := missingTriggerAttributes(tc.trigger); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("missingTriggerAttributes() = %v, want %v", got, tc.want)
			}
		})
	}
}

func validateScheduledTaskConfig(t *testing.T, triggers ...ScheduledTaskTriggerModel) diag.Diagnostics {
	t.Helper()

	ctx := context.Background()
	s := scheduledTaskSchema(t)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &ScheduledTaskResourceModel{
		ID:       types.StringNull(),
		TaskID:   types.StringValue("7738148ffcd07979c7ceb148e06b3aed"),
		Triggers: triggerList(t, triggers...),
	}); diags.HasError() {
		t.Fatalf("building config: %v", diags)
	}

	var resp resource.ValidateConfigResponse
	(&ScheduledTaskResource{}).ValidateConfig(ctx, resource.ValidateConfigRequest{
		Config: tfsdk.Config{Schema: s, Raw: plan.Raw},
	}, &resp)
	return resp.Diagnostics
}

func errorPaths(t *testing.T, diags diag.Diagnostics) []string {
	t.Helper()

	var paths []string
	for _, d := range diags.Errors() {
		withPath, ok := d.(diag.DiagnosticWithPath)
		if !ok {
			t.Fatalf("diagnostic has no attribute path: %v", d)
		}
		paths = append(paths, withPath.Path().String())
	}
	return paths
}

func TestScheduledTaskValidateConfigReportsMissingAttributePath(t *testing.T) {
	t.Parallel()

	interval := newTrigger(triggerTypeInterval)
	interval.IntervalTicks = types.Int64Value(1)
	weekly := newTrigger(triggerTypeWeekly)
	weekly.TimeOfDayTicks = types.Int64Value(1)

	got := errorPaths(t, validateScheduledTaskConfig(t, interval, weekly))
	want := []string{path.Root("triggers").AtListIndex(1).AtName("day_of_week").String()}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ValidateConfig() error paths = %v, want %v", got, want)
	}
}

func TestScheduledTaskValidateConfigReportsEveryInvalidTrigger(t *testing.T) {
	t.Parallel()

	got := errorPaths(t, validateScheduledTaskConfig(t, newTrigger(triggerTypeDaily), newTrigger(triggerTypeWeekly)))
	want := []string{
		path.Root("triggers").AtListIndex(0).AtName("time_of_day_ticks").String(),
		path.Root("triggers").AtListIndex(1).AtName("time_of_day_ticks").String(),
		path.Root("triggers").AtListIndex(1).AtName("day_of_week").String(),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ValidateConfig() error paths = %v, want %v", got, want)
	}
}

func validateScheduledTaskSelectors(t *testing.T, key, taskID types.String) diag.Diagnostics {
	t.Helper()

	ctx := context.Background()
	s := scheduledTaskSchema(t)
	var resp resource.ValidateConfigResponse
	(&ScheduledTaskResource{}).ValidateConfig(ctx, resource.ValidateConfigRequest{
		Config: tfsdk.Config{Schema: s, Raw: scheduledTaskValue(t, &ScheduledTaskResourceModel{
			ID:     types.StringNull(),
			Key:    key,
			TaskID: taskID,
		})},
	}, &resp)
	return resp.Diagnostics
}

func TestScheduledTaskValidateConfigWantsKeyOrTaskID(t *testing.T) {
	t.Parallel()

	key, taskID := types.StringValue("RefreshLibrary"), types.StringValue(scanMediaLibraryID)
	tests := map[string]struct {
		key, taskID types.String
		wantError   string
	}{
		"key alone":                     {key: key, taskID: types.StringNull()},
		"task_id alone":                 {key: types.StringNull(), taskID: taskID},
		"both":                          {key: key, taskID: taskID},
		"neither":                       {key: types.StringNull(), taskID: types.StringNull(), wantError: "Missing scheduled task attribute"},
		"unknown key with task_id":      {key: types.StringUnknown(), taskID: taskID},
		"key with unknown task_id":      {key: key, taskID: types.StringUnknown()},
		"unknown key without task_id":   {key: types.StringUnknown(), taskID: types.StringNull()},
		"unknown task_id without a key": {key: types.StringNull(), taskID: types.StringUnknown()},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			diags := validateScheduledTaskSelectors(t, tc.key, tc.taskID)
			switch {
			case tc.wantError == "" && diags.HasError():
				t.Fatalf("ValidateConfig() = %v, want no error", diags)
			case tc.wantError != "" && (len(diags.Errors()) != 1 || diags.Errors()[0].Summary() != tc.wantError):
				t.Fatalf("ValidateConfig() = %v, want one error %q", diags, tc.wantError)
			}
		})
	}
}

const (
	scanMediaLibraryID = "7738148ffcd07979c7ceb148e06b3aed"
	updatePluginsID    = "f9b057c054e9e6daee4a88ffd146a403"
	refreshChannelsID  = "0c9ee3a88fc15547c6852205480da1fd"
	cleanLogFilesID    = "1c8ede62c521bea0bf851344f5b8ca40"
)

func testTasks() []client.ScheduledTask {
	return []client.ScheduledTask{
		{ID: scanMediaLibraryID, Key: "RefreshLibrary"},
		{ID: updatePluginsID, Key: "PluginUpdates"},
		{ID: refreshChannelsID, Key: "RefreshInternetChannels", IsHidden: true},
	}
}

func TestFindTask(t *testing.T) {
	t.Parallel()

	shared := append(testTasks(), client.ScheduledTask{ID: "aaaa", Key: "Shared"}, client.ScheduledTask{ID: "bbbb", Key: "Shared"})
	tests := map[string]struct {
		tasks      []client.ScheduledTask
		ref        string
		byID       bool
		want       string
		wantDetail string
	}{
		"key": {
			ref: "PluginUpdates", want: updatePluginsID,
		},
		"key of a hidden task": {
			ref: "RefreshInternetChannels", want: refreshChannelsID,
		},
		"key in another case": {
			ref:        "refreshlibrary",
			wantDetail: `No scheduled task has the key "refreshlibrary". Keys match exactly, case included. The server's tasks have these keys: PluginUpdates, RefreshInternetChannels, RefreshLibrary.`,
		},
		"ID given as a key": {
			ref:        scanMediaLibraryID,
			wantDetail: `No scheduled task has the key "` + scanMediaLibraryID + `".`,
		},
		"key several tasks have": {
			tasks: shared, ref: "Shared",
			wantDetail: `Several tasks have the key "Shared" (IDs aaaa, bbbb); set task_id to the ID of the one to manage.`,
		},
		"import by ID keeps its spelling": {
			ref: "7738148FFCD07979C7CEB148E06B3AED", byID: true, want: "7738148FFCD07979C7CEB148E06B3AED",
		},
		"import by key": {
			ref: "RefreshLibrary", byID: true, want: scanMediaLibraryID,
		},
		"import of neither": {
			ref: "Nothing", byID: true,
			wantDetail: `No scheduled task has the ID or key "Nothing".`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tasks := tc.tasks
			if tasks == nil {
				tasks = testTasks()
			}
			got, d := findTask(tasks, tc.ref, tc.byID)
			if tc.wantDetail == "" {
				if d != nil || got != tc.want {
					t.Fatalf("findTask(%q) = %q, %v; want %q", tc.ref, got, d, tc.want)
				}
				return
			}
			if d == nil || !strings.HasPrefix(d.Detail(), tc.wantDetail) {
				t.Fatalf("findTask(%q) = %q, %v; want an error starting %q", tc.ref, got, d, tc.wantDetail)
			}
		})
	}
}

// fakeTaskServer serves tasks as Jellyfin does: the list, and each task by
// its ID ignoring case.
type fakeTaskServer struct {
	tasks []client.ScheduledTask

	mu       sync.Mutex
	requests int
}

func (f *fakeTaskServer) client(t *testing.T) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests++
		if r.Method != http.MethodGet {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/ScheduledTasks" {
			_ = json.NewEncoder(w).Encode(f.tasks)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/ScheduledTasks/")
		i := slices.IndexFunc(f.tasks, func(task client.ScheduledTask) bool { return strings.EqualFold(task.ID, id) })
		if i < 0 {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": f.tasks[i].ID, "Key": f.tasks[i].Key, "Triggers": []any{}})
	}))
	t.Cleanup(srv.Close)
	return client.NewClient(srv.URL, "k")
}

func (f *fakeTaskServer) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

// scheduledTaskValue is m as a Terraform value, with no triggers when m has
// none, or the null object when m is nil.
func scheduledTaskValue(t *testing.T, m *ScheduledTaskResourceModel) tftypes.Value {
	t.Helper()

	ctx := context.Background()
	s := scheduledTaskSchema(t)
	v := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if m == nil {
		return v.Raw
	}
	withTriggers := *m
	if withTriggers.Triggers.IsNull() {
		withTriggers.Triggers = triggerList(t)
	}
	if d := v.Set(ctx, &withTriggers); d.HasError() {
		t.Fatalf("building %+v: %v", m, d)
	}
	return v.Raw
}

// planScheduledTask runs ModifyPlan on plan, what the framework plans from
// config and state before it, where a nil state plans a create.
func planScheduledTask(t *testing.T, c *client.Client, config ScheduledTaskResourceModel, state *ScheduledTaskResourceModel, plan ScheduledTaskResourceModel) (ScheduledTaskResourceModel, resource.ModifyPlanResponse) {
	t.Helper()

	ctx := context.Background()
	s := scheduledTaskSchema(t)
	resp := resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: s, Raw: scheduledTaskValue(t, &plan)}}
	(&ScheduledTaskResource{client: c}).ModifyPlan(ctx, resource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: s, Raw: scheduledTaskValue(t, &config)},
		State:  tfsdk.State{Schema: s, Raw: scheduledTaskValue(t, state)},
		Plan:   resp.Plan,
	}, &resp)
	var got ScheduledTaskResourceModel
	if !resp.Diagnostics.HasError() {
		if d := resp.Plan.Get(ctx, &got); d.HasError() {
			t.Fatal(d)
		}
	}
	return got, resp
}

func keyConfig(key string) ScheduledTaskResourceModel {
	return ScheduledTaskResourceModel{ID: types.StringNull(), Key: types.StringValue(key), TaskID: types.StringNull()}
}

func keyCreatePlan(key string) ScheduledTaskResourceModel {
	return ScheduledTaskResourceModel{ID: types.StringUnknown(), Key: types.StringValue(key), TaskID: types.StringUnknown()}
}

func storedTask(taskID string, key types.String) *ScheduledTaskResourceModel {
	return &ScheduledTaskResourceModel{ID: types.StringValue(taskID), Key: key, TaskID: types.StringValue(taskID)}
}

func TestScheduledTaskPlanResolvesKeyOnCreate(t *testing.T) {
	t.Parallel()

	srv := &fakeTaskServer{tasks: testTasks()}
	got, resp := planScheduledTask(t, srv.client(t), keyConfig("RefreshLibrary"), nil, keyCreatePlan("RefreshLibrary"))
	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan() = %v", resp.Diagnostics)
	}
	if got.TaskID.ValueString() != scanMediaLibraryID {
		t.Errorf("planned task_id = %v, want %s", got.TaskID, scanMediaLibraryID)
	}
}

func TestScheduledTaskPlanRejectsKeyNoTaskHas(t *testing.T) {
	t.Parallel()

	srv := &fakeTaskServer{tasks: testTasks()}
	_, resp := planScheduledTask(t, srv.client(t), keyConfig("NoSuchTask"), nil, keyCreatePlan("NoSuchTask"))
	if got, want := errorPaths(t, resp.Diagnostics), []string{path.Root("key").String()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ModifyPlan() error paths = %v (%v), want %v", got, resp.Diagnostics, want)
	}
}

func TestScheduledTaskPlanKeepsTheStoredTaskItsKeySelects(t *testing.T) {
	t.Parallel()

	srv := &fakeTaskServer{tasks: testTasks()}
	state := storedTask(scanMediaLibraryID, types.StringValue("RefreshLibrary"))
	got, resp := planScheduledTask(t, srv.client(t), keyConfig("RefreshLibrary"), state, *state)
	if resp.Diagnostics.HasError() || len(resp.RequiresReplace) != 0 {
		t.Fatalf("ModifyPlan() = %v, replace %v; want neither", resp.Diagnostics, resp.RequiresReplace)
	}
	if got.TaskID.ValueString() != scanMediaLibraryID {
		t.Errorf("planned task_id = %v, want %s", got.TaskID, scanMediaLibraryID)
	}
}

// The refresh reads the key of the task task_id names, so the state holds a
// key that other tasks may share once the configuration switches to it.
func TestScheduledTaskPlanRejectsASharedKeyTheStateHolds(t *testing.T) {
	t.Parallel()

	srv := &fakeTaskServer{tasks: append(testTasks(), client.ScheduledTask{ID: "aaaa", Key: "Shared"}, client.ScheduledTask{ID: "bbbb", Key: "Shared"})}
	state := storedTask("aaaa", types.StringValue("Shared"))
	_, resp := planScheduledTask(t, srv.client(t), keyConfig("Shared"), state, *state)
	if got, want := errorPaths(t, resp.Diagnostics), []string{path.Root("key").String()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ModifyPlan() error paths = %v (%v), want %v", got, resp.Diagnostics, want)
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Ambiguous scheduled task key" {
		t.Errorf("ModifyPlan() error = %q, want the ambiguous key error", got)
	}
}

func TestScheduledTaskPlanForKeyUnknownUntilApply(t *testing.T) {
	t.Parallel()

	stored := storedTask(scanMediaLibraryID, types.StringValue("RefreshLibrary"))
	tests := map[string]struct {
		state       *ScheduledTaskResourceModel
		taskID      types.String
		wantReplace bool
	}{
		"replaces a stored task":            {state: stored, taskID: types.StringNull(), wantReplace: true},
		"plans a create":                    {taskID: types.StringNull()},
		"leaves a configured task_id to it": {state: stored, taskID: types.StringValue(scanMediaLibraryID)},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv := &fakeTaskServer{tasks: testTasks()}
			config := ScheduledTaskResourceModel{ID: types.StringNull(), Key: types.StringUnknown(), TaskID: tc.taskID}
			plan := keyCreatePlan("")
			if tc.state != nil {
				plan = *tc.state
			}
			plan.Key = types.StringUnknown()
			_, resp := planScheduledTask(t, srv.client(t), config, tc.state, plan)
			if resp.Diagnostics.HasError() {
				t.Fatalf("ModifyPlan() = %v", resp.Diagnostics)
			}
			var want path.Paths
			if tc.wantReplace {
				want = path.Paths{path.Root("key")}
			}
			if !reflect.DeepEqual(resp.RequiresReplace, want) {
				t.Errorf("RequiresReplace = %v, want %v", resp.RequiresReplace, want)
			}
			if n := srv.requestCount(); n != 0 {
				t.Errorf("ModifyPlan() made %d requests, want none", n)
			}
		})
	}
}

// terraform plan -generate-config-out writes both from an imported state.
func TestScheduledTaskPlanWithKeyAndTaskID(t *testing.T) {
	t.Parallel()

	key := types.StringValue("RefreshLibrary")
	tests := map[string]struct {
		tasks       []client.ScheduledTask
		key, taskID types.String
		wantError   bool
	}{
		"naming the same task":                 {key: key, taskID: types.StringValue(scanMediaLibraryID)},
		"naming the same task in another case": {key: key, taskID: types.StringValue("7738148FFCD07979C7CEB148E06B3AED")},
		"naming different tasks":               {key: key, taskID: types.StringValue(updatePluginsID), wantError: true},
		"with a key in another case":           {key: types.StringValue("refreshlibrary"), taskID: types.StringValue(scanMediaLibraryID), wantError: true},
		"with a task_id no task has":           {key: key, taskID: types.StringValue(cleanLogFilesID)},
		"with a task_id unknown until apply":   {key: key, taskID: types.StringUnknown()},
		"naming one of the tasks sharing a key": {
			tasks: append(testTasks(), client.ScheduledTask{ID: "aaaa", Key: "Shared"}, client.ScheduledTask{ID: "bbbb", Key: "Shared"}),
			key:   types.StringValue("Shared"), taskID: types.StringValue("aaaa"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tasks := tc.tasks
			if tasks == nil {
				tasks = testTasks()
			}
			srv := &fakeTaskServer{tasks: tasks}
			config := ScheduledTaskResourceModel{ID: types.StringNull(), Key: tc.key, TaskID: tc.taskID}
			plan := config
			plan.ID = types.StringUnknown()
			got, resp := planScheduledTask(t, srv.client(t), config, nil, plan)
			if tc.wantError {
				if gotPaths, want := errorPaths(t, resp.Diagnostics), []string{path.Root("key").String()}; !reflect.DeepEqual(gotPaths, want) {
					t.Fatalf("ModifyPlan() error paths = %v (%v), want %v", gotPaths, resp.Diagnostics, want)
				}
				if summary := resp.Diagnostics.Errors()[0].Summary(); summary != "Conflicting scheduled task attributes" {
					t.Errorf("ModifyPlan() error = %q, want the conflicting attributes error", summary)
				}
				return
			}
			if resp.Diagnostics.HasError() || len(resp.RequiresReplace) != 0 {
				t.Fatalf("ModifyPlan() = %v, replace %v; want neither", resp.Diagnostics, resp.RequiresReplace)
			}
			if !got.TaskID.Equal(tc.taskID) {
				t.Errorf("planned task_id = %v, want the configured %v", got.TaskID, tc.taskID)
			}
		})
	}
}

func TestScheduledTaskPlanReplacesWhenKeySelectsAnotherTask(t *testing.T) {
	t.Parallel()

	srv := &fakeTaskServer{tasks: testTasks()}
	state := storedTask(scanMediaLibraryID, types.StringValue("RefreshLibrary"))
	plan := *state
	plan.Key = types.StringValue("PluginUpdates")
	got, resp := planScheduledTask(t, srv.client(t), keyConfig("PluginUpdates"), state, plan)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan() = %v", resp.Diagnostics)
	}
	if want := (path.Paths{path.Root("key")}); !reflect.DeepEqual(resp.RequiresReplace, want) {
		t.Errorf("RequiresReplace = %v, want %v", resp.RequiresReplace, want)
	}
	if got.TaskID.ValueString() != updatePluginsID {
		t.Errorf("planned task_id = %v, want %s", got.TaskID, updatePluginsID)
	}
}

// A state saved before the key attribute existed has no key until a refresh
// reads one, which -refresh=false skips.
func TestScheduledTaskPlanKeepsStoredTaskIDWhenKeySelectsThatTask(t *testing.T) {
	t.Parallel()

	srv := &fakeTaskServer{tasks: testTasks()}
	state := storedTask("7738148FFCD07979C7CEB148E06B3AED", types.StringNull())
	plan := *state
	plan.Key = types.StringValue("RefreshLibrary")
	got, resp := planScheduledTask(t, srv.client(t), keyConfig("RefreshLibrary"), state, plan)
	if resp.Diagnostics.HasError() || len(resp.RequiresReplace) != 0 {
		t.Fatalf("ModifyPlan() = %v, replace %v; want neither", resp.Diagnostics, resp.RequiresReplace)
	}
	if !got.TaskID.Equal(state.TaskID) {
		t.Errorf("planned task_id = %v, want the stored %v", got.TaskID, state.TaskID)
	}
}

func TestScheduledTaskReadAfterImport(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		importID    string
		wantTaskID  string
		wantKey     string
		wantSummary string
	}{
		"by key":                     {importID: "PluginUpdates", wantTaskID: updatePluginsID, wantKey: "PluginUpdates"},
		"by ID":                      {importID: scanMediaLibraryID, wantTaskID: scanMediaLibraryID, wantKey: "RefreshLibrary"},
		"by ID in another case":      {importID: "7738148FFCD07979C7CEB148E06B3AED", wantTaskID: "7738148FFCD07979C7CEB148E06B3AED", wantKey: "RefreshLibrary"},
		"by neither an ID nor a key": {importID: "refreshlibrary", wantSummary: "Scheduled task not found"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			srv := &fakeTaskServer{tasks: testTasks()}
			s := scheduledTaskSchema(t)
			imported := resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: scheduledTaskValue(t, nil)}}
			(&ScheduledTaskResource{}).ImportState(ctx, resource.ImportStateRequest{ID: tc.importID}, &imported)
			if imported.Diagnostics.HasError() {
				t.Fatalf("ImportState() = %v", imported.Diagnostics)
			}

			resp := readAgainst(t, NewScheduledTaskResource(), srv.client(t), imported.State)
			if tc.wantSummary != "" {
				if len(resp.Diagnostics.Errors()) != 1 || resp.Diagnostics.Errors()[0].Summary() != tc.wantSummary {
					t.Fatalf("Read() = %v, want one error %q", resp.Diagnostics, tc.wantSummary)
				}
				return
			}
			if resp.Diagnostics.HasError() {
				t.Fatalf("Read() = %v", resp.Diagnostics)
			}
			var got ScheduledTaskResourceModel
			if d := resp.State.Get(ctx, &got); d.HasError() {
				t.Fatal(d)
			}
			if got.TaskID.ValueString() != tc.wantTaskID || got.ID.ValueString() != tc.wantTaskID || got.Key.ValueString() != tc.wantKey {
				t.Errorf("read id %v, task_id %v, key %v; want id and task_id %s, key %s", got.ID, got.TaskID, got.Key, tc.wantTaskID, tc.wantKey)
			}
		})
	}
}
