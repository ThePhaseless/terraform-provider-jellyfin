#!/usr/bin/env bash
# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# setup_jellyfin.sh - Prepares a fresh Jellyfin instance for testing.
# The provider completes the startup wizard during acceptance tests.
set -euo pipefail

JELLYFIN_ENDPOINT="${JELLYFIN_ENDPOINT:-http://localhost:8096}"
JELLYFIN_USERNAME="${JELLYFIN_USERNAME:-admin}"
JELLYFIN_PASSWORD="${JELLYFIN_PASSWORD:-Admin123!}"
MAX_WAIT=120
ENV_FILE="${JELLYFIN_ENV_FILE:-$(dirname "$0")/../internal/provider/supported_jellyfin_version.env}"

echo "Waiting for Jellyfin to become ready at ${JELLYFIN_ENDPOINT}..." >&2
for i in $(seq 1 "$MAX_WAIT"); do
    # /Startup/User returns JSON when not configured, or 401 when already configured.
    HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${JELLYFIN_ENDPOINT}/Startup/User" 2>/dev/null || echo "000")
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "401" ]; then
        echo "Jellyfin is ready! (waited ${i}s)" >&2
        break
    fi
    if [ "$i" -eq "$MAX_WAIT" ]; then
        echo "ERROR: Jellyfin did not become ready within ${MAX_WAIT}s" >&2
        exit 1
    fi
    sleep 1
done

echo "Creating test media directories..." >&2
docker compose --env-file "${ENV_FILE}" exec -T jellyfin mkdir -p /media/movies /media/tvshows >&2
echo "  - Media directories created" >&2

echo "" >&2
echo "=== Test Environment Ready ===" >&2
echo "JELLYFIN_ENDPOINT=${JELLYFIN_ENDPOINT}" >&2
echo "JELLYFIN_USERNAME=${JELLYFIN_USERNAME}" >&2
echo "" >&2
printf 'export JELLYFIN_ENDPOINT=%q\n' "${JELLYFIN_ENDPOINT}"
printf 'export JELLYFIN_USERNAME=%q\n' "${JELLYFIN_USERNAME}"
printf 'export JELLYFIN_PASSWORD=%q\n' "${JELLYFIN_PASSWORD}"
