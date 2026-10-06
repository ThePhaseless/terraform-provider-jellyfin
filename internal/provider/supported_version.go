// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/release"
)

//go:embed supported_jellyfin_version.env
var supportedJellyfinVersionEnv string

//go:embed supported_security_plugin_version.env
var supportedSecurityPluginVersionEnv string

// supportedJellyfinVersion returns the tested Jellyfin server version from the
// embedded .env file.
func supportedJellyfinVersion() string {
	return release.FromEnv(supportedJellyfinVersionEnv, "JELLYFIN_VERSION")
}

// supportedSecurityPluginVersion returns the tested JellyfinSecurity plugin
// version from the embedded .env file.
func supportedSecurityPluginVersion() string {
	return release.FromEnv(supportedSecurityPluginVersionEnv, "SECURITY_PLUGIN_VERSION")
}

// versionNewerWarning returns the detail to surface as a warning, and ok true,
// when installed is newer than supported.
func versionNewerWarning(what, installed, supported string) (detail string, ok bool) {
	if release.Compare(installed, supported) <= 0 {
		return "", false
	}

	return fmt.Sprintf(
		"The %s reports version %s, which is newer than the latest version this provider was tested against (%s). "+
			"The provider may behave unexpectedly. Check for a newer provider release, and report issues at "+
			"https://github.com/ThePhaseless/terraform-provider-jellyfin/issues.",
		what, installed, supported,
	), true
}

// jellyfinVersionWarning returns a warning's summary and detail, and ok true,
// when the server's Jellyfin release differs from the one this provider
// supports, older or newer. Only the segments supported names count, so a
// server on 12.2.1 runs the supported 12.2.
func jellyfinVersionWarning(installed, supported string) (summary, detail string, ok bool) {
	segments := strings.Split(strings.TrimSpace(installed), ".")
	if n := len(strings.Split(supported, ".")); len(segments) > n {
		segments = segments[:n]
	}
	switch release.Compare(strings.Join(segments, "."), supported) {
	case 1:
		summary = "Jellyfin version newer than supported"
	case -1:
		summary = "Jellyfin version older than supported"
	default:
		return "", "", false
	}
	return summary, fmt.Sprintf(
		"The Jellyfin server reports version %s, but this provider release supports only Jellyfin %s. "+
			"Settings the two releases do not share may be ignored or rejected by the server. "+
			"Use the provider release that supports your Jellyfin version, and report issues at "+
			"https://github.com/ThePhaseless/terraform-provider-jellyfin/issues.",
		installed, supported,
	), true
}
