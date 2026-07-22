// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
)

//go:embed supported_jellyfin_version.env
var supportedJellyfinVersionEnv string

//go:embed supported_sso_plugin_version.env
var supportedSSOPluginVersionEnv string

// supportedJellyfinVersion returns the tested Jellyfin server version from the
// embedded .env file. It trims whitespace and strips the JELLYFIN_VERSION= prefix.
func supportedJellyfinVersion() string {
	return parseVersionEnv(supportedJellyfinVersionEnv, "JELLYFIN_VERSION")
}

// supportedSSOPluginVersion returns the tested SSO-Auth plugin version from the
// embedded .env file. It trims whitespace and strips the SSO_PLUGIN_VERSION= prefix.
func supportedSSOPluginVersion() string {
	return parseVersionEnv(supportedSSOPluginVersionEnv, "SSO_PLUGIN_VERSION")
}

func parseVersionEnv(content, key string) string {
	content = strings.TrimSpace(content)
	prefix := key + "="
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

// compareDottedVersions compares dotted numeric versions a and b.
// It returns -1 if a < b, 0 if a == b, and 1 if a > b.
// Each segment contributes its leading integer (trailing non-digits ignored);
// missing segments count as 0; no leading digit in the first segment means the
// version is treated as 0.0.0.
func compareDottedVersions(a, b string) int {
	if !hasLeadingDigit(a) || !hasLeadingDigit(b) {
		return 0
	}

	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := range maxLen {
		aSeg := parseSegment(getPart(aParts, i))
		bSeg := parseSegment(getPart(bParts, i))

		if aSeg < bSeg {
			return -1
		}
		if aSeg > bSeg {
			return 1
		}
	}

	return 0
}

func hasLeadingDigit(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	return s[0] >= '0' && s[0] <= '9'
}

func getPart(parts []string, i int) string {
	if i >= len(parts) {
		return ""
	}
	return parts[i]
}

func parseSegment(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}

	n, err := strconv.ParseInt(s[:end], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// versionNewerWarning returns a detail message when installed > supported.
// The ok return value is true when installed is newer and the caller should
// surface the detail as a warning.
func versionNewerWarning(what, installed, supported string) (detail string, ok bool) {
	if compareDottedVersions(installed, supported) <= 0 {
		return "", false
	}

	return fmt.Sprintf(
		"The %s reports version %s, which is newer than the latest version this provider was tested against (%s). "+
			"The provider may behave unexpectedly. Check for a newer provider release, and report issues at "+
			"https://github.com/ThePhaseless/terraform-provider-jellyfin/issues.",
		what, installed, supported,
	), true
}
