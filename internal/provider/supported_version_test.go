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
