// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

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
