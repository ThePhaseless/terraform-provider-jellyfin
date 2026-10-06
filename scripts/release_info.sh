#!/usr/bin/env bash
# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Prints, as KEY=value lines for $GITHUB_ENV, what a release says about the
# versions it supports: JELLYFIN_VERSION, the one Jellyfin release it
# supports, and RELEASE_NOTES, a Markdown header listing that release and each
# plugin build from internal/provider/supported_*_plugin_version.env, followed
# by the CHANGELOG.md section of the version given as the first argument.
#
# Each plugin file names its plugin in PLUGIN_NAME and its build in one
# *_VERSION key, so supporting another plugin takes only a file of its own.
set -euo pipefail

cd "$(dirname "$0")/.."

version="${1:-}"
version="${version#v}"

env_value() { grep -oP "^$1=\K\S+" "$2"; }

jellyfin="$(env_value JELLYFIN_VERSION internal/provider/supported_jellyfin_version.env)"

plugins=""
for f in internal/provider/supported_*_plugin_version.env; do
  name="$(env_value PLUGIN_NAME "$f")"
  build="$(grep -oP '^[A-Z_]+_VERSION=\K\S+' "$f")"
  if [ -z "$name" ] || [ -z "$build" ]; then
    echo "$f needs PLUGIN_NAME and a *_VERSION key" >&2
    exit 1
  fi
  plugins+="- ${name} ${build}"$'\n'
done

changes=""
if [ -n "$version" ]; then
  changes="$(awk -v ver="$version" '
    index($0, "## [" ver "]") == 1 { on = 1; next }
    on && /^## \[/ { exit }
    on { print }
  ' CHANGELOG.md | sed -e '/./,$!d')"
fi

delimiter="EOF_$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')"
echo "JELLYFIN_VERSION=${jellyfin}"
echo "RELEASE_NOTES<<${delimiter}"
echo "Supports **Jellyfin ${jellyfin}** only. Use the provider release that matches your Jellyfin version."
echo
if [ -n "$plugins" ]; then
  echo "Supported plugins:"
  echo
  printf '%s' "$plugins"
  echo
fi
if [ -n "$changes" ]; then
  echo "$changes"
fi
echo "${delimiter}"
