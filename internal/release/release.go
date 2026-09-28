// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

// Package release orders the dotted versions of Jellyfin releases and plugin
// builds, and reads them from the .env files that pin them.
package release

import (
	"strconv"
	"strings"
)

// Compare returns -1, 0 or 1 as a is older than, the same as or newer than b.
// Each dotted segment counts by its leading integer, with trailing non-digits
// ignored, and a missing segment counts as 0. A version without a leading
// digit compares equal to any other.
func Compare(a, b string) int {
	if !HasLeadingDigit(a) || !HasLeadingDigit(b) {
		return 0
	}
	as, bs := strings.Split(strings.TrimSpace(a), "."), strings.Split(strings.TrimSpace(b), ".")
	for i := range max(len(as), len(bs)) {
		x, y := segment(as, i), segment(bs, i)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}
	return 0
}

// HasLeadingDigit reports whether s, without surrounding whitespace, starts
// with a digit, as a version Compare orders does.
func HasLeadingDigit(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && s[0] >= '0' && s[0] <= '9'
}

func segment(parts []string, i int) int64 {
	if i >= len(parts) {
		return 0
	}
	s := strings.TrimSpace(parts[i])
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, err := strconv.ParseInt(s[:end], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// FromEnv returns the value of key in content, the lines of an .env file, or
// "" when no line sets it.
func FromEnv(content, key string) string {
	for _, line := range strings.Split(content, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), key+"="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
