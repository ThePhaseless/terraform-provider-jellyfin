// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	_ "embed"
	"fmt"

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

// versionNewerWarning returns a detail message when installed > supported.
// The ok return value is true when installed is newer and the caller should
// surface the detail as a warning.
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
