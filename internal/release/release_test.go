// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package release

import "testing"

func TestCompare(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		a, b string
		want int
	}{
		{"newer patch", "10.11.12", "10.11.11", 1},
		{"equal", "10.11.11", "10.11.11", 0},
		{"older minor", "10.9.0", "10.11.11", -1},
		{"newer minor", "10.10", "10.9.11", 1},
		{"newer major vs older very new minor", "10.12.0", "10.11.99", 1},
		{"older major", "10.11.11", "12.0", -1},
		{"equal with extra zero segment", "10.11.11.0", "10.11.11", 0},
		{"shorter version equal", "10.11", "10.11.0", 0},
		{"garbage compares equal", "not-a-version", "10.11.11", 0},
		{"two garbage", "foo", "bar", 0},
		{"plugin newer", "4.0.0.5", "4.0.0.4", 1},
		{"plugin older", "4.0.0.3", "4.0.0.4", -1},
		{"trailing non-digit ignored", "10.11.11-rc1", "10.11.11", 0},
		{"newer because rc suffix ignored", "10.11.12-rc1", "10.11.11", 1},
		{"surrounding whitespace", " 10.11.12 ", "10.11.11", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Compare(tt.a, tt.b); got != tt.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestFromEnv(t *testing.T) {
	t.Parallel()

	const env = "# comment\n JELLYFIN_VERSION=10.11.11 \nNEXT_JELLYFIN_VERSION=12.0\n"
	for key, want := range map[string]string{
		"JELLYFIN_VERSION":      "10.11.11",
		"NEXT_JELLYFIN_VERSION": "12.0",
		"MISSING":               "",
	} {
		if got := FromEnv(env, key); got != want {
			t.Errorf("FromEnv(%q) = %q, want %q", key, got, want)
		}
	}
}
