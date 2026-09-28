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

func TestUnitBrandingConfigurationRoundTrip(t *testing.T) {
	ctx := context.Background()
	b, err := brandingWire()
	if err != nil {
		t.Fatal(err)
	}
	fixture := `{"LoginDisclaimer":"LoginDisclaimer","CustomCss":"CustomCSS","SplashscreenEnabled":true}`

	data := BrandingConfigurationResourceModel{ID: types.StringValue("branding")}
	if d := b.FlattenInto(ctx, fixture, &data); d.HasError() {
		t.Fatalf("read: %v", d)
	}

	base := map[string]json.RawMessage{}
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("write: %v", d)
	}

	checkSameJSON(t, base, fixture)
}

func TestUnitBrandingSplashscreenLocationIsNotComputedReadOrWritten(t *testing.T) {
	ctx := context.Background()

	var resp resource.SchemaResponse
	(&BrandingConfigurationResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Schema.Attributes["splashscreen_location"].IsComputed() {
		t.Error("splashscreen_location is computed, so create plans show it as known after apply although it always reads as null")
	}

	b, err := brandingWire()
	if err != nil {
		t.Fatal(err)
	}
	data := BrandingConfigurationResourceModel{SplashscreenLocation: types.StringValue("/config/splashscreen.png")}
	if d := b.FlattenInto(ctx, `{"SplashscreenEnabled":true,"SplashscreenLocation":"/config/splashscreen.png"}`, &data); d.HasError() {
		t.Fatalf("read: %v", d)
	}
	if !data.SplashscreenLocation.IsNull() {
		t.Errorf("splashscreen_location read as %s, want null", data.SplashscreenLocation)
	}

	base := map[string]json.RawMessage{}
	data.SplashscreenLocation = types.StringValue("/config/splashscreen.png")
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("write: %v", d)
	}
	if v, ok := base["SplashscreenLocation"]; ok {
		t.Errorf("overlay wrote SplashscreenLocation = %s, want no key", v)
	}
}
