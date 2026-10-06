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
	if got := supportedSecurityPluginVersion(); got == "" {
		t.Errorf("supportedSecurityPluginVersion() returned empty string")
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
			if !tt.wantOk {
				if detail != "" {
					t.Errorf("versionNewerWarning() returned non-empty detail when ok=false: %s", detail)
				}
				return
			}
			for _, want := range []string{tt.installed, tt.supported, "https://github.com/ThePhaseless/terraform-provider-jellyfin/issues"} {
				if !strings.Contains(detail, want) {
					t.Errorf("versionNewerWarning() detail missing %q: %s", want, detail)
				}
			}
		})
	}
}

func TestUnitJellyfinVersionWarning(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		installed, supported, summary string
	}{
		{"12.2.0", "12.2", ""},
		{"12.2.3", "12.2", ""},
		{"12.3.0", "12.2", "Jellyfin version newer than supported"},
		{"13.0.0", "12.2", "Jellyfin version newer than supported"},
		{"12.1.4", "12.2", "Jellyfin version older than supported"},
		{"10.11.11", "12.2", "Jellyfin version older than supported"},
		{"unknown", "12.2", ""},
	} {
		summary, detail, ok := jellyfinVersionWarning(tt.installed, tt.supported)
		if summary != tt.summary || ok != (tt.summary != "") {
			t.Errorf("jellyfinVersionWarning(%q, %q) = %q, %t; want %q", tt.installed, tt.supported, summary, ok, tt.summary)
		}
		if ok && (!strings.Contains(detail, tt.installed) || !strings.Contains(detail, tt.supported)) {
			t.Errorf("jellyfinVersionWarning(%q, %q) detail names neither version: %s", tt.installed, tt.supported, detail)
		}
	}
}
