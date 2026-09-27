// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
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
		MaxParentalRating:          types.Int64Value(13),
		MaxParentalSubRating:       types.Int64Value(1),
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
	fields := []struct {
		name      string
		got, want attr.Value
	}{
		{"IsHidden", got.IsHidden, policy.IsHidden},
		{"EnableMediaPlayback", got.EnableMediaPlayback, policy.EnableMediaPlayback},
		{"MaxParentalRating", got.MaxParentalRating, policy.MaxParentalRating},
		{"MaxParentalSubRating", got.MaxParentalSubRating, policy.MaxParentalSubRating},
		{"LoginAttemptsBeforeLockout", got.LoginAttemptsBeforeLockout, policy.LoginAttemptsBeforeLockout},
		{"MaxActiveSessions", got.MaxActiveSessions, policy.MaxActiveSessions},
		{"SyncPlayAccess", got.SyncPlayAccess, policy.SyncPlayAccess},
		{"EnabledFolders", got.EnabledFolders, policy.EnabledFolders},
		{"AccessSchedules", got.AccessSchedules, policy.AccessSchedules},
	}
	for _, f := range fields {
		if !f.got.Equal(f.want) {
			t.Errorf("%s = %s, want %s", f.name, f.got, f.want)
		}
	}
}

func TestUnitUserPolicyOverlayWritesNullParentalRatings(t *testing.T) {
	ctx := context.Background()
	m, err := parseJSONObject(`{"MaxParentalRating": 10, "MaxParentalSubRating": 2, "IsHidden": true}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	policy := &UserPolicyModel{
		MaxParentalRating:    types.Int64Null(),
		MaxParentalSubRating: types.Int64Null(),
		IsHidden:             types.BoolNull(),
	}

	if d := overlayPolicyIntoJSON(ctx, m, policy); d.HasError() {
		t.Fatalf("overlay: %v", d)
	}

	if got := string(m["MaxParentalRating"]); got != "null" {
		t.Errorf("MaxParentalRating = %s, want null", got)
	}
	if got := string(m["MaxParentalSubRating"]); got != "null" {
		t.Errorf("MaxParentalSubRating = %s, want null", got)
	}
	if got := string(m["IsHidden"]); got != "true" {
		t.Errorf("IsHidden = %s, want the server value true kept", got)
	}
}

func TestUnitUserRenameKeepsConfiguration(t *testing.T) {
	t.Parallel()

	var posted map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users/user-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Id":"user-1","Name":"old","Configuration":{"SubtitleLanguagePreference":"fre"},"Policy":{"IsHidden":true}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/Users" && r.URL.Query().Get("userId") == "user-1":
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Errorf("decoding posted user: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	r := &UserResource{client: client.NewClient(server.URL, "k")}
	if err := r.renameUser(context.Background(), "user-1", "New"); err != nil {
		t.Fatalf("renameUser() error = %v", err)
	}

	if got := string(posted["Name"]); got != `"New"` {
		t.Errorf("Name = %s, want \"New\"", got)
	}
	if got := string(posted["Configuration"]); got != `{"SubtitleLanguagePreference":"fre"}` {
		t.Errorf("Configuration = %s, want the one read from the server", got)
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
