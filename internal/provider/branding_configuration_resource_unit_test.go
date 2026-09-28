// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnitBrandingConfigurationRoundTrip(t *testing.T) {
	checkRoundTrip[BrandingConfigurationResourceModel](t, mustWire(t, brandingWire), `{"LoginDisclaimer":"LoginDisclaimer","CustomCss":"CustomCSS","SplashscreenEnabled":true}`)
}

func TestUnitBrandingSplashscreenLocationIsNotComputedReadOrWritten(t *testing.T) {
	ctx := t.Context()

	if schemaOf(&BrandingConfigurationResource{}).Attributes["splashscreen_location"].IsComputed() {
		t.Error("splashscreen_location is computed, so create plans show it as known after apply although it always reads as null")
	}

	b := mustWire(t, brandingWire)
	data := BrandingConfigurationResourceModel{SplashscreenLocation: types.StringValue("/config/splashscreen.png")}
	if d := b.FlattenInto(ctx, `{"SplashscreenEnabled":true,"SplashscreenLocation":"/config/splashscreen.png"}`, &data); d.HasError() {
		t.Fatalf("read: %v", d)
	}
	if !data.SplashscreenLocation.IsNull() {
		t.Errorf("splashscreen_location read as %s, want null", data.SplashscreenLocation)
	}

	data.SplashscreenLocation = types.StringValue("/config/splashscreen.png")
	if v, ok := writeWire(t, b, &data)["SplashscreenLocation"]; ok {
		t.Errorf("overlay wrote SplashscreenLocation = %s, want no key", v)
	}
}
