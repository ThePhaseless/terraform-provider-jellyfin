// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

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

func TestVersionDescribes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		version   types.String
		installed string
		describes bool
	}{
		{types.StringValue("latest"), "15.0.0.0", true},
		{types.StringValue("supported"), "2.6.3.1", true},
		{types.StringValue("13.0.0.0"), "13.0.0.0", true},
		{types.StringValue("2.5.22"), "2.5.22.0", true},
		{types.StringValue("12.0.0.0"), "13.0.0.0", false}, // Jellyfin updated the plugin
		{types.StringValue(""), "13.0.0.0", true},          // configured empty, installed as if omitted
		{types.StringNull(), "13.0.0.0", false},            // imported
	}

	for _, c := range cases {
		if got := versionDescribes(c.version, c.installed); got != c.describes {
			t.Errorf("versionDescribes(%s, %q) = %t, want %t", c.version, c.installed, got, c.describes)
		}
	}
}

func TestSelectInstalledPlugin(t *testing.T) {
	t.Parallel()

	const id = "9c4e63f1031b4f25988b4f7d78a8b53e"
	superseded := client.InstalledPlugin{ID: id, Name: "Bookshelf", Version: "12.0.0.0", Status: "Superseded"}
	pending := client.InstalledPlugin{ID: id, Name: "Bookshelf", Version: "13.0.0.0", Status: "Restart"}
	deleted := client.InstalledPlugin{ID: id, Name: "Bookshelf", Version: "14.0.0.0", Status: pluginStatusDeleted}
	other := client.InstalledPlugin{ID: "170a157fac6c437aabddca9c25cebd39", Name: "Fanart", Version: "15.0.0.0", Status: "Active"}

	cases := []struct {
		name    string
		plugins []client.InstalledPlugin
		id      string
		plugin  string
		version string
		want    string
		found   bool
	}{
		{"version in state while an update is pending", []client.InstalledPlugin{other, superseded, pending}, id, "Bookshelf", "12.0.0.0", "12.0.0.0", true},
		{"newest when the version in state is gone", []client.InstalledPlugin{pending, superseded}, id, "Bookshelf", "11.0.0.0", "13.0.0.0", true},
		{"newest without a version", []client.InstalledPlugin{superseded, pending}, id, "Bookshelf", "", "13.0.0.0", true},
		{"version pending deletion skipped", []client.InstalledPlugin{superseded, deleted}, id, "Bookshelf", "14.0.0.0", "12.0.0.0", true},
		{"dashed id as imported", []client.InstalledPlugin{other, pending}, "9C4E63F1-031B-4F25-988B-4F7D78A8B53E", "9C4E63F1-031B-4F25-988B-4F7D78A8B53E", "", "13.0.0.0", true},
		{"name as imported", []client.InstalledPlugin{other, pending}, "Bookshelf", "Bookshelf", "", "13.0.0.0", true},
		{"only a version pending deletion", []client.InstalledPlugin{other, deleted}, id, "Bookshelf", "", "", false},
		{"not listed", []client.InstalledPlugin{other}, id, "Bookshelf", "", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, found := selectInstalledPlugin(c.plugins, c.id, c.plugin, c.version)
			if found != c.found || got.Version != c.want {
				t.Errorf("selectInstalledPlugin() = %q, %t, want %q, %t", got.Version, found, c.want, c.found)
			}
		})
	}
}
