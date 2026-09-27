// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnitBrandingConfigurationOverlay(t *testing.T) {
	ctx := context.Background()
	fixture := `{"LoginDisclaimer":"LoginDisclaimer","CustomCss":"CustomCSS","SplashscreenEnabled":true}`

	var data BrandingConfigurationResourceModel
	flattenBrandingConfiguration(ctx, fixture, &data, nil)

	base := map[string]json.RawMessage{}
	overlayBrandingConfiguration(ctx, base, &data)

	result, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	var want map[string]interface{}
	if err := json.Unmarshal([]byte(fixture), &want); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("round-trip mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestUnitBrandingSplashscreenLocationIsNotComputedReadOrWritten(t *testing.T) {
	ctx := context.Background()

	var resp resource.SchemaResponse
	(&BrandingConfigurationResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Schema.Attributes["splashscreen_location"].IsComputed() {
		t.Error("splashscreen_location is computed, so create plans show it as known after apply although it always reads as null")
	}

	var data BrandingConfigurationResourceModel
	flattenBrandingConfiguration(ctx, `{"SplashscreenEnabled":true,"SplashscreenLocation":"/config/splashscreen.png"}`, &data, nil)
	if !data.SplashscreenLocation.IsNull() {
		t.Errorf("splashscreen_location read as %s, want null", data.SplashscreenLocation)
	}

	base := map[string]json.RawMessage{}
	data.SplashscreenLocation = types.StringValue("/config/splashscreen.png")
	overlayBrandingConfiguration(ctx, base, &data)
	if v, ok := base["SplashscreenLocation"]; ok {
		t.Errorf("overlay wrote SplashscreenLocation = %s, want no key", v)
	}
}
