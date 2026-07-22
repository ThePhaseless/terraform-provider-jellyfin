// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"strings"
	"testing"
)

func TestSupportedVersionParsers(t *testing.T) {
	t.Parallel()

	if got := supportedJellyfinVersion(); got == "" {
		t.Errorf("supportedJellyfinVersion() returned empty string")
	}
	if got := supportedSSOPluginVersion(); got == "" {
		t.Errorf("supportedSSOPluginVersion() returned empty string")
	}
}

func TestCompareDottedVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{"newer patch", "10.11.12", "10.11.11", 1},
		{"equal", "10.11.11", "10.11.11", 0},
		{"older minor", "10.9.0", "10.11.11", -1},
		{"newer major vs older very new minor", "10.12.0", "10.11.99", 1},
		{"equal with extra zero segment", "10.11.11.0", "10.11.11", 0},
		{"garbage treated as 0.0.0", "not-a-version", "10.11.11", 0},
		{"two garbage", "foo", "bar", 0},
		{"sso newer", "4.0.0.5", "4.0.0.4", 1},
		{"sso equal", "4.0.0.4", "4.0.0.4", 0},
		{"sso older", "4.0.0.3", "4.0.0.4", -1},
		{"shorter version equal", "10.11", "10.11.0", 0},
		{"trailing non-digit ignored", "10.11.11-rc1", "10.11.11", 0},
		{"newer because rc suffix ignored", "10.11.12-rc1", "10.11.11", 1},
		{"leading whitespace", " 10.11.12 ", "10.11.11", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := compareDottedVersions(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareDottedVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestVersionNewerWarning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		installed string
		supported string
		wantOk    bool
	}{
		{"newer", "10.11.12", "10.11.11", true},
		{"equal", "10.11.11", "10.11.11", false},
		{"older", "10.10.0", "10.11.11", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			detail, ok := versionNewerWarning("Jellyfin server", tt.installed, tt.supported)
			if ok != tt.wantOk {
				t.Errorf("versionNewerWarning() ok = %v, want %v", ok, tt.wantOk)
			}
			if tt.wantOk {
				if detail == "" {
					t.Errorf("versionNewerWarning() returned empty detail but ok=true")
				}
				if !strings.Contains(detail, tt.installed) {
					t.Errorf("versionNewerWarning() detail missing installed version %q: %s", tt.installed, detail)
				}
				if !strings.Contains(detail, tt.supported) {
					t.Errorf("versionNewerWarning() detail missing supported version %q: %s", tt.supported, detail)
				}
				if !strings.Contains(detail, "https://github.com/ThePhaseless/terraform-provider-jellyfin/issues") {
					t.Errorf("versionNewerWarning() detail missing issue URL: %s", detail)
				}
			} else {
				if detail != "" {
					t.Errorf("versionNewerWarning() returned non-empty detail when ok=false: %s", detail)
				}
			}
		})
	}
}
