// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"testing"
)

func TestHCLString(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
		want  string
	}{
		"plain":                 {"hello", `"hello"`},
		"empty":                 {"", `""`},
		"quotes":                {`say "hi"`, `"say \"hi\""`},
		"backslash":             {`back\slash`, `"back\\slash"`},
		"template introducers":  {"${a} %{b}", `"$${a} %%{b}"`},
		"lone markers":          {"$c %d $ % {e}", `"$c %d $ % {e}"`},
		"escaped introducers":   {"$${a} %%{b}", `"$$${a} %%%{b}"`},
		"unterminated":          {"cost ${", `"cost $${"`},
		"control characters":    {"line\nnext\r\t\x01\x1f", `"line\nnext\r\t\u0001\u001f"`},
		"non-ASCII is kept":     {"café ☕", `"café ☕"`},
		"introducer after rune": {"é${x}", `"é$${x}"`},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := hclString(tc.input); got != tc.want {
				t.Errorf("hclString(%q) = %s, want %s", tc.input, got, tc.want)
			}
		})
	}
}
