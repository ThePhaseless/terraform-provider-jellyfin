// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

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
		{"2.6.3.1", "2.6.3.1", true},
		{"2.6.3.1", "2.6.3.0", false}, // same release, different server ABI build
		{"2.6.3.1", "2.6.3", false},
	}

	for _, c := range cases {
		if got := samePluginVersion(c.got, c.want); got != c.same {
			t.Errorf("samePluginVersion(%q, %q) = %v, want %v", c.got, c.want, got, c.same)
		}
	}
}

func TestPluginRelease(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"2.6.3.1": "2.6.3",
		"2.6.3.0": "2.6.3",
		"2.6.3":   "2.6.3",
		"10.11":   "10.11",
		"":        "",
	}

	for in, want := range cases {
		if got := pluginRelease(in); got != want {
			t.Errorf("pluginRelease(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPickReleaseBuild(t *testing.T) {
	t.Parallel()

	offered := func(versions ...string) []client.VersionInfo {
		out := make([]client.VersionInfo, len(versions))
		for i, v := range versions {
			out[i] = client.VersionInfo{Version: v}
		}
		return out
	}

	cases := []struct {
		name    string
		offered []client.VersionInfo
		want    string
		pick    string
	}{
		{"pinned build offered", offered("2.6.3.1", "2.6.3.0", "2.6.1.1"), "2.6.3.1", "2.6.3.1"},
		{"only the other ABI build offered", offered("2.6.3.0", "2.6.1.0", "2.5.22.0"), "2.6.3.1", "2.6.3.0"},
		{"highest sibling build wins", offered("2.6.3.0", "2.6.3.2"), "2.6.3.1", "2.6.3.2"},
		{"release not offered", offered("2.6.1.1", "2.6.1.0"), "2.6.3.1", "2.6.3.1"},
		{"nothing offered", nil, "2.6.3.1", "2.6.3.1"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := pickReleaseBuild(c.offered, c.want); got != c.pick {
				t.Errorf("pickReleaseBuild(%v, %q) = %q, want %q", c.offered, c.want, got, c.pick)
			}
		})
	}
}
