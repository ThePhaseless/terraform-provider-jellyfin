// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"strings"
	"testing"
)

func TestUnitParseAPIGolden(t *testing.T) {
	ix := parseAPIGolden(`op GET /Thing | resp=#Thing
schema Day: string enum(Friday|Monday)
schema Empty: {object}
schema Thing.Count: integer:int32
schema Thing.Days: []#Day
schema Thing.Kids: []#Kid
schema Thing.Name: string
schema Kid.Ratio: number:double
`)
	for _, c := range []struct {
		object, key string
		want        Prop
	}{
		{"Thing", "Count", Prop{Key: "Count", Scalar: "integer", Format: "int32", Sig: "integer:int32"}},
		{"Thing", "Days", Prop{Key: "Days", List: true, Scalar: "string", Sig: "[]#Day"}},
		{"Thing", "Kids", Prop{Key: "Kids", List: true, Ref: "Kid", Sig: "[]#Kid"}},
		{"Kid", "Ratio", Prop{Key: "Ratio", Scalar: "number", Format: "double", Sig: "number:double"}},
	} {
		if got := ix[c.object][c.key]; got != c.want {
			t.Errorf("%s.%s = %+v, want %+v", c.object, c.key, got, c.want)
		}
	}
	if _, ok := ix["Empty"]; !ok {
		t.Error("an object without properties is missing")
	}
	if _, ok := ix["Day"]; ok {
		t.Error("an enum was read as an object")
	}
}

func TestUnitParseSecurityGolden(t *testing.T) {
	ix := parseSecurityGolden("Root", `Enabled: boolean
Port: number
Cidrs: []string
Providers: []object
Providers[].Id: string
Providers[].Maps: []object
Providers[].Maps[].Role: string
`)
	for _, c := range []struct {
		object, key string
		want        Prop
	}{
		{"Root", "Port", Prop{Key: "Port", Scalar: "number", Sig: "number"}},
		{"Root", "Cidrs", Prop{Key: "Cidrs", List: true, Scalar: "string", Sig: "[]string"}},
		{"Root", "Providers", Prop{Key: "Providers", List: true, Ref: "Root.Providers[]", Sig: "[]object"}},
		{"Root.Providers[]", "Maps", Prop{Key: "Maps", List: true, Ref: "Root.Providers[].Maps[]", Sig: "[]object"}},
		{"Root.Providers[].Maps[]", "Role", Prop{Key: "Role", Scalar: "string", Sig: "string"}},
	} {
		if got := ix[c.object][c.key]; got != c.want {
			t.Errorf("%s.%s = %+v, want %+v", c.object, c.key, got, c.want)
		}
	}
}

func TestUnitResolveIgnoresCaseAndUnderscores(t *testing.T) {
	ix := parseAPIGolden(`schema Net.EnableUPnP: boolean
schema Net.AllowHWTranscoding: boolean
schema Net.H264Crf: integer:int32
schema Dup.FooBar: string
schema Dup.Foo_Bar: string
`)
	for attr, want := range map[string]string{"enable_upnp": "EnableUPnP", "allow_hw_transcoding": "AllowHWTranscoding", "h264_crf": "H264Crf"} {
		p, err := ix.Resolve("Net", attr)
		if err != nil || p.Key != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", attr, p.Key, err, want)
		}
	}
	if _, err := ix.Resolve("Net", "enable_upnq"); err == nil || !strings.Contains(err.Error(), "nearest EnableUPnP") {
		t.Errorf("a typo resolves or names no near key: %v", err)
	}
	if _, err := ix.Resolve("Dup", "foo_bar"); err == nil || !strings.Contains(err.Error(), "several") {
		t.Errorf("an ambiguous name resolves: %v", err)
	}
	if _, err := ix.Resolve("Missing", "x"); err == nil {
		t.Error("an unknown object resolves")
	}
}

func TestUnitEmbeddedCatalog(t *testing.T) {
	pinned, floor := Pinned(), Floor()
	if _, ok := pinned["BrandingOptionsDto"]["LoginDisclaimer"]; !ok {
		t.Error("the pinned golden lacks BrandingOptionsDto.LoginDisclaimer")
	}
	if _, ok := pinned[SecurityPluginRoot+".OidcProviders[]"]["CreatedAt"]; !ok {
		t.Error("the security plugin golden is not part of the pinned objects")
	}
	if _, ok := floor["EncodingOptions"]["EncodingThreadCount"]; !ok {
		t.Error("the floor golden lacks EncodingOptions.EncodingThreadCount")
	}
	if _, ok := floor["EncodingOptions"]["HlsAudioSeekStrategy"]; ok {
		t.Error("the floor golden has HlsAudioSeekStrategy, which Jellyfin 12.0 added")
	}
	if !hasLeadingDigit(FloorVersion()) || compareVersions(SinceVersion(), FloorVersion()) <= 0 {
		t.Errorf("floor.env: version %q, next %q; want a version and a later one", FloorVersion(), SinceVersion())
	}
}

func TestUnitCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"10.11.11", "12.0", -1},
		{"12.0.0", "12.0", 0},
		{"12.1.0", "12.0", 1},
		{"10.10", "10.9.11", 1},
		{"10.10.0-rc1", "10.10", 0},
	} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
