// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cmd

import "fmt"

var (
	version = "dev"
	commit  = "none"
)

// GetVersion returns a version string corresponding to the current release.
func GetVersion() string {
	return fmt.Sprintf("%v-%v", version, commit)
}
