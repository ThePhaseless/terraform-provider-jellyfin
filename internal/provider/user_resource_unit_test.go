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
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func TestUnitUserPolicyRoundTrip(t *testing.T) {
	ctx := context.Background()
	b, err := userPolicyWire()
	if err != nil {
		t.Fatal(err)
	}
	fixture := `{"IsAdministrator":false,"IsDisabled":false,"EnableAllFolders":true,` +
		`"IsHidden":true,"EnableMediaPlayback":false,"MaxParentalRating":13,"MaxParentalSubRating":1,` +
		`"LoginAttemptsBeforeLockout":3,"MaxActiveSessions":2,"SyncPlayAccess":"JoinGroups",` +
		`"AccessSchedules":[{"DayOfWeek":"Monday","StartHour":9.5,"EndHour":17}],"EnabledFolders":["/movies"]}`

	data := UserResourceModel{ID: types.StringValue("user-1"), Name: types.StringValue("alice")}
	if d := b.FlattenInto(ctx, fixture, &data); d.HasError() {
		t.Fatalf("read: %v", d)
	}
	if data.Policy == nil || data.Policy.MaxParentalSubRating.ValueInt64() != 1 || !data.EnableAllFolders.ValueBool() {
		t.Fatalf("read policy %+v, enable_all_folders %v", data.Policy, data.EnableAllFolders)
	}
	if !data.Name.Equal(types.StringValue("alice")) {
		t.Errorf("name = %v, want the prior value: the policy document has no name", data.Name)
	}

	base := map[string]json.RawMessage{}
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("write: %v", d)
	}
	testUnitAssertJSONEqual(t, mustJSON(base), fixture)
}

func TestUnitUserPolicyWriteSendsNullParentalRatings(t *testing.T) {
	ctx := context.Background()
	b, err := userPolicyWire()
	if err != nil {
		t.Fatal(err)
	}
	var data UserResourceModel
	if d := b.FlattenInto(ctx, `{}`, &data); d.HasError() {
		t.Fatalf("read: %v", d)
	}

	m, err := parseJSONObject(`{"MaxParentalRating": 10, "MaxParentalSubRating": 2, "IsHidden": true}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d := b.OverlayModel(ctx, m, &data); d.HasError() {
		t.Fatalf("write: %v", d)
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

// Jellyfin refuses to disable a user that is an administrator before the
// write, so demoting and disabling an administrator at once takes two posts.
func TestUnitUserPolicyDemotesBeforeDisabling(t *testing.T) {
	t.Parallel()

	policy := map[string]any{"IsAdministrator": true, "IsDisabled": false, "EnableAllFolders": true}
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users/user-1":
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": "user-1", "Name": "n", "Policy": policy})
		case r.Method == http.MethodPost && r.URL.Path == "/Users/user-1/Policy":
			posts++
			var posted map[string]any
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Errorf("decoding posted policy: %v", err)
			}
			if posted["IsDisabled"] == true && policy["IsAdministrator"] == true {
				http.Error(w, "Administrators cannot be disabled.", http.StatusForbidden)
				return
			}
			policy = posted
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	r := &UserResource{client: client.NewClient(server.URL, "k")}
	data := UserResourceModel{
		IsAdministrator:  types.BoolValue(false),
		IsDisabled:       types.BoolValue(true),
		EnableAllFolders: types.BoolValue(true),
	}
	var diags diag.Diagnostics
	if err := r.applyPolicy(context.Background(), &data, "user-1", &diags); err != nil || diags.HasError() {
		t.Fatalf("applyPolicy() error = %v, %v", err, diags)
	}
	if policy["IsAdministrator"] != false || policy["IsDisabled"] != true || posts != 2 {
		t.Errorf("after %d posts the policy is %v, want a disabled user that is no administrator", posts, policy)
	}
}

// Jellyfin lists IDs as 32 lowercase hex digits, whatever spelling it read.
func TestUnitUserPolicyTakesIDsAsJellyfinListsThem(t *testing.T) {
	s := schemaOf(&UserResource{})
	policy, ok := s.Attributes["policy"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatal("policy is not a nested object")
	}
	for _, name := range []string{"enabled_folders", "blocked_media_folders", "enabled_channels", "blocked_channels"} {
		a, ok := policy.Attributes[name].(schema.ListAttribute)
		if !ok {
			t.Fatalf("%s is not a list", name)
		}
		for id, want := range map[string]bool{
			"f137a2dd21bbc1b99aa5c0f6bf02a805":     false,
			"F137A2DD21BBC1B99AA5C0F6BF02A805":     true,
			"f137a2dd-21bb-c1b9-9aa5-c0f6bf02a805": true,
		} {
			resp := validator.ListResponse{}
			for _, v := range a.Validators {
				v.ValidateList(context.Background(), validator.ListRequest{Path: path.Root(name), ConfigValue: types.ListValueMust(types.StringType, []attr.Value{types.StringValue(id)})}, &resp)
			}
			if resp.Diagnostics.HasError() != want {
				t.Errorf("%s = [%q]: error %t, want %t", name, id, resp.Diagnostics.HasError(), want)
			}
		}
	}
}

// Removing a schedule must not plan the next one with its hours: an hour a
// schedule leaves unset comes from the prior schedule with the same day.
func TestUnitUserAccessSchedulesPlanByDay(t *testing.T) {
	ctx := context.Background()
	s := schemaOf(&UserResource{})
	policy, ok := s.Attributes["policy"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatal("policy is not a nested object")
	}
	schedules, ok := policy.Attributes["access_schedules"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatal("access_schedules is not a nested list")
	}
	elemType, ok := schedules.NestedObject.Type().(types.ObjectType)
	if !ok {
		t.Fatal("access_schedules holds no objects")
	}
	schedule := func(day string, start, end attr.Value) attr.Value {
		return types.ObjectValueMust(elemType.AttrTypes, map[string]attr.Value{"day_of_week": types.StringValue(day), "start_hour": start, "end_hour": end})
	}
	state := types.ListValueMust(elemType, []attr.Value{
		schedule("Monday", types.Float64Value(8), types.Float64Value(12)),
		schedule("Tuesday", types.Float64Value(9), types.Float64Value(17)),
	})
	config := types.ListValueMust(elemType, []attr.Value{schedule("Tuesday", types.Float64Value(9), types.Float64Null())})
	plan := types.ListValueMust(elemType, []attr.Value{schedule("Tuesday", types.Float64Value(9), types.Float64Unknown())})

	existing := tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{})
	resp := planmodifier.ListResponse{PlanValue: plan}
	for _, m := range schedules.PlanModifiers {
		m.PlanModifyList(ctx, planmodifier.ListRequest{
			Path:        path.Root("policy").AtName("access_schedules"),
			State:       tfsdk.State{Raw: existing},
			Plan:        tfsdk.Plan{Raw: existing},
			ConfigValue: config,
			PlanValue:   resp.PlanValue,
			StateValue:  state,
		}, &resp)
	}
	got, ok := resp.PlanValue.Elements()[0].(types.Object)
	if !ok || !got.Attributes()["end_hour"].Equal(types.Float64Value(17)) {
		t.Errorf("planned %s, want Tuesday's end hour 17", resp.PlanValue)
	}
}
