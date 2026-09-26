// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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

func TestMarshalTriggersOmitsNullAttributes(t *testing.T) {
	t.Parallel()

	for name, tc := range triggerSerialisationCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := marshalTriggers(context.Background(), triggerList(t, tc.trigger))
			if err != nil {
				t.Fatalf("marshalTriggers() error = %v", err)
			}

			var gotEntries, wantEntries []map[string]any
			if err := json.Unmarshal([]byte(got), &gotEntries); err != nil {
				t.Fatalf("unmarshal marshalTriggers() output %s: %v", got, err)
			}
			if err := json.Unmarshal([]byte("["+tc.json+"]"), &wantEntries); err != nil {
				t.Fatalf("unmarshal want: %v", err)
			}
			if !reflect.DeepEqual(gotEntries, wantEntries) {
				t.Fatalf("marshalTriggers() = %s, want [%s]", got, tc.json)
			}
		})
	}
}

func TestFlattenTriggersNullsAbsentAttributes(t *testing.T) {
	t.Parallel()

	for name, tc := range triggerSerialisationCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, diags := flattenTriggers(context.Background(), []json.RawMessage{json.RawMessage(tc.json)})
			if diags.HasError() {
				t.Fatalf("flattenTriggers() diagnostics: %v", diags)
			}
			if want := triggerList(t, tc.trigger); !got.Equal(want) {
				t.Fatalf("flattenTriggers() = %v, want %v", got, want)
			}
		})
	}
}

func TestFlattenTriggersTreatsExplicitJSONNullAsNull(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"Type":"IntervalTrigger","IntervalTicks":1,"TimeOfDayTicks":null,"DayOfWeek":null,"MaxRuntimeTicks":null}`)
	got, diags := flattenTriggers(context.Background(), []json.RawMessage{raw})
	if diags.HasError() {
		t.Fatalf("flattenTriggers() diagnostics: %v", diags)
	}

	want := newTrigger(triggerTypeInterval)
	want.IntervalTicks = types.Int64Value(1)
	if !got.Equal(triggerList(t, want)) {
		t.Fatalf("flattenTriggers() = %v, want %v", got, triggerList(t, want))
	}
}

func TestFlattenTriggersReturnsKnownEmptyListForNoTriggers(t *testing.T) {
	t.Parallel()

	got, diags := flattenTriggers(context.Background(), nil)
	if diags.HasError() {
		t.Fatalf("flattenTriggers() diagnostics: %v", diags)
	}
	if got.IsNull() || got.IsUnknown() || len(got.Elements()) != 0 {
		t.Fatalf("flattenTriggers(nil) = %v, want a known empty list", got)
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

func TestScheduledTaskValidateConfigReportsMissingAttributePath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := scheduledTaskSchema(t)

	interval := newTrigger(triggerTypeInterval)
	interval.IntervalTicks = types.Int64Value(1)
	weekly := newTrigger(triggerTypeWeekly)
	weekly.TimeOfDayTicks = types.Int64Value(1)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &ScheduledTaskResourceModel{
		ID:       types.StringNull(),
		TaskID:   types.StringValue("7738148ffcd07979c7ceb148e06b3aed"),
		Triggers: triggerList(t, interval, weekly),
	}); diags.HasError() {
		t.Fatalf("building config: %v", diags)
	}

	var resp resource.ValidateConfigResponse
	(&ScheduledTaskResource{}).ValidateConfig(ctx, resource.ValidateConfigRequest{
		Config: tfsdk.Config{Schema: s, Raw: plan.Raw},
	}, &resp)

	if got := resp.Diagnostics.ErrorsCount(); got != 1 {
		t.Fatalf("ValidateConfig() error count = %d, want 1: %v", got, resp.Diagnostics)
	}
	d, ok := resp.Diagnostics.Errors()[0].(interface{ Path() path.Path })
	if !ok {
		t.Fatalf("ValidateConfig() error has no attribute path: %v", resp.Diagnostics)
	}
	if want := path.Root("triggers").AtListIndex(1).AtName("day_of_week"); !d.Path().Equal(want) {
		t.Fatalf("ValidateConfig() error path = %s, want %s", d.Path(), want)
	}
}
