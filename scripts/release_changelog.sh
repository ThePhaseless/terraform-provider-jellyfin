#!/usr/bin/env bash
# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Turns the Unreleased section of CHANGELOG.md into the section of version,
# dated date, and adds entry to its Changed list: under the Changed heading the
# section already has, or under a new one at its end.
#
#   scripts/release_changelog.sh 0.5.0 2026-10-06 "- Supports only Jellyfin 12.2."
set -euo pipefail

cd "$(dirname "$0")/.."

version="$1" date="$2" entry="$3"

awk -v ver="$version" -v date="$date" -v entry="$entry" '
  !done && /^## \[Unreleased\]$/ {
    print; print ""; print "## [" ver "] - " date
    in_section = 1
    next
  }
  in_section && /^### Changed$/ && !added {
    print; print ""; print entry
    added = 1
    if ((getline next_line) > 0 && next_line != "") print next_line
    next
  }
  in_section && /^## \[/ {
    if (!added) { print "### Changed"; print ""; print entry; print "" }
    in_section = 0; done = 1
  }
  { print }
' CHANGELOG.md > CHANGELOG.md.new
mv CHANGELOG.md.new CHANGELOG.md
