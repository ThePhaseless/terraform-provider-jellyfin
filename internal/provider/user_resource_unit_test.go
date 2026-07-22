// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnitUserPolicyOverlay(t *testing.T) {
	ctx := context.Background()
	fixture := `{
		"IsHidden": true,
		"EnableMediaPlayback": false,
		"LoginAttemptsBeforeLockout": 3,
		"MaxActiveSessions": 2,
		"SyncPlayAccess": "JoinGroups",
		"AccessSchedules": [
			{"DayOfWeek": "Monday", "StartHour": 9.0, "EndHour": 17.0}
		],
		"EnabledFolders": ["/movies"]
	}`

	m, err := parseJSONObject(fixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	policy := &UserPolicyModel{
		IsHidden:                   types.BoolValue(true),
		EnableMediaPlayback:        types.BoolValue(false),
		LoginAttemptsBeforeLockout: types.Int64Value(3),
		MaxActiveSessions:          types.Int64Value(2),
		SyncPlayAccess:             types.StringValue("JoinGroups"),
		EnabledFolders:             mustStringList([]string{"/movies"}),
		AccessSchedules:            mustAccessScheduleList(ctx, []UserAccessScheduleModel{{DayOfWeek: types.StringValue("Monday"), StartHour: types.Float64Value(9.0), EndHour: types.Float64Value(17.0)}}),
	}

	if d := overlayPolicyIntoJSON(ctx, m, policy); d.HasError() {
		t.Fatalf("overlay: %v", d)
	}

	got := policyFromRaw(ctx, string(mustJSON(m)), nil)
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(policy)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("round-trip mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func mustStringList(values []string) types.List {
	v, _ := types.ListValueFrom(context.Background(), types.StringType, values)
	return v
}

func mustAccessScheduleList(ctx context.Context, values []UserAccessScheduleModel) types.List {
	objType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"day_of_week": types.StringType,
		"start_hour":  types.Float64Type,
		"end_hour":    types.Float64Type,
	}}
	objects := make([]types.Object, len(values))
	for i, v := range values {
		objects[i], _ = types.ObjectValue(objType.AttrTypes, map[string]attr.Value{
			"day_of_week": v.DayOfWeek,
			"start_hour":  v.StartHour,
			"end_hour":    v.EndHour,
		})
	}
	v, _ := types.ListValueFrom(ctx, objType, objects)
	return v
}
