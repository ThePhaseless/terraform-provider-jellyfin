// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import "testing"

func TestSamePluginVersionIgnoresTrailingZeroSegment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		got, want string
		same      bool
	}{
		{"2.5.22.0", "2.5.22", true}, // Jellyfin's assembly version vs the release tag
		{"2.5.22.0", "2.5.22.0", true},
		{"2.5.22.0", "2.5.21", false},
		{"2.5.21.0", "2.5.22.0", false},
		{"2.5.22.0", "", true}, // no version requested
	}

	for _, c := range cases {
		if got := samePluginVersion(c.got, c.want); got != c.same {
			t.Errorf("samePluginVersion(%q, %q) = %v, want %v", c.got, c.want, got, c.same)
		}
	}
}
