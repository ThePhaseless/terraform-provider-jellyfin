#!/usr/bin/env bash
# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# setup_jellyfin.sh - Prepares a fresh Jellyfin instance for testing.
# The provider completes the startup wizard during acceptance tests.
set -euo pipefail

JELLYFIN_URL="${JELLYFIN_ENDPOINT:-http://localhost:8096}"
JELLYFIN_USERNAME="${JELLYFIN_USERNAME:-admin}"
JELLYFIN_PASSWORD="${JELLYFIN_PASSWORD:-Admin123!}"
MAX_WAIT=120

echo "Waiting for Jellyfin to become ready at ${JELLYFIN_URL}..."
for i in $(seq 1 "$MAX_WAIT"); do
    # /Startup/User returns JSON when not configured, or 401 when already configured.
    HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${JELLYFIN_URL}/Startup/User" 2>/dev/null || echo "000")
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "401" ]; then
        echo "Jellyfin is ready! (waited ${i}s)"
        break
    fi
    if [ "$i" -eq "$MAX_WAIT" ]; then
        echo "ERROR: Jellyfin did not become ready within ${MAX_WAIT}s"
        exit 1
    fi
    sleep 1
done

# Create test media directories inside the container.
echo "Creating test media directories..."
docker compose exec -T jellyfin mkdir -p /media/movies /media/tvshows
echo "  - Media directories created"

echo ""
echo "=== Test Environment Ready ==="
echo "JELLYFIN_ENDPOINT=${JELLYFIN_URL}"
echo "JELLYFIN_USERNAME=${JELLYFIN_USERNAME}"
echo "JELLYFIN_PASSWORD=${JELLYFIN_PASSWORD}"
echo ""
echo "export JELLYFIN_ENDPOINT=${JELLYFIN_URL}"
echo "export JELLYFIN_USERNAME=${JELLYFIN_USERNAME}"
echo "export JELLYFIN_PASSWORD=${JELLYFIN_PASSWORD}"
