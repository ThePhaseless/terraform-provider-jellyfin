// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnitLiveTVConfigurationOverlay(t *testing.T) {
	ctx := context.Background()
	fixture := `{"GuideDays":14,"RecordingPath":"/recordings","TunerHosts":[{"Id":"host1","Url":"http://tv/"}],"ListingProviders":[{"Id":"prov1","Type":"SchedulesDirect","Username":"u","Password":"p","EnabledTuners":["host1"],"ChannelMappings":[{"Name":"c1","Value":"d1"}]}],"PrePaddingSeconds":30,"PostPaddingSeconds":30}`

	var data LiveTVConfigurationResourceModel
	data.GuideDays = types.Int64Value(14)
	data.TunerHosts = types.ListNull(tunerHostObjectType())
	data.ListingProviders = types.ListNull(listingProviderObjectType())
	data.PrePaddingSeconds = types.Int64Value(30)
	data.PostPaddingSeconds = types.Int64Value(30)

	m, err := parseJSONObject(fixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if d := overlayLiveTVConfiguration(ctx, m, &data); d.HasError() {
		t.Fatalf("overlay: %v", d)
	}

	var got LiveTVConfigurationResourceModel
	flattenLiveTVConfiguration(ctx, string(mustJSON(m)), &got, nil)

	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(data)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("round-trip mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
