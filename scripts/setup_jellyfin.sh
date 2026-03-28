#!/usr/bin/env bash
# setup_jellyfin.sh - Sets up a fresh Jellyfin instance for testing.
# This script completes the startup wizard and creates an API key.
set -uo pipefail

JELLYFIN_URL="${JELLYFIN_ENDPOINT:-http://localhost:8096}"
MAX_WAIT=120

echo "Waiting for Jellyfin to become ready at ${JELLYFIN_URL}..."
for i in $(seq 1 "$MAX_WAIT"); do
    # Wait until the server is fully initialized.
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

# Check if startup wizard is already completed.
WIZARD_COMPLETED=$(curl -s "${JELLYFIN_URL}/System/Info/Public" | python3 -c "import sys,json; print(json.load(sys.stdin).get('StartupWizardCompleted', False))" 2>/dev/null || echo "False")

if [ "$WIZARD_COMPLETED" = "True" ]; then
    echo "Startup wizard already completed."
else
    echo "Completing startup wizard..."

    curl -s -o /dev/null -X POST "${JELLYFIN_URL}/Startup/Configuration" \
        -H "Content-Type: application/json" \
        -d '{"UICulture":"en-US","MetadataCountryCode":"US","PreferredMetadataLanguage":"en"}'
    echo "  - Configuration set"

    curl -s -o /dev/null -X POST "${JELLYFIN_URL}/Startup/User" \
        -H "Content-Type: application/json" \
        -d '{"Name":"admin","Password":"Admin123!"}'
    echo "  - User set"

    curl -s -o /dev/null -X POST "${JELLYFIN_URL}/Startup/Complete"
    echo "  - Wizard completed"

    # Wait for wizard completion to propagate.
    sleep 2
fi

# Authenticate to get a token.
echo "Authenticating..."
AUTH_RESPONSE=$(curl -s -X POST "${JELLYFIN_URL}/Users/AuthenticateByName" \
    -H "Content-Type: application/json" \
    -H 'Authorization: MediaBrowser Client="Terraform", Device="Setup", DeviceId="setup-script", Version="1.0.0"' \
    -d '{"Username":"admin","Pw":"Admin123!"}')

ACCESS_TOKEN=$(echo "$AUTH_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['AccessToken'])")
if [ -z "$ACCESS_TOKEN" ]; then
    echo "ERROR: Failed to authenticate. Response: $AUTH_RESPONSE"
    exit 1
fi
echo "Got access token."

# Create an API key.
echo "Creating API key..."
curl -s -o /dev/null -X POST "${JELLYFIN_URL}/Auth/Keys?app=terraform-provider-test" \
    -H "Authorization: MediaBrowser Token=\"${ACCESS_TOKEN}\""

# Fetch the API key.
API_KEYS_RESPONSE=$(curl -s "${JELLYFIN_URL}/Auth/Keys" \
    -H "Authorization: MediaBrowser Token=\"${ACCESS_TOKEN}\"")

API_KEY=$(echo "$API_KEYS_RESPONSE" | python3 -c "import sys,json; items=json.load(sys.stdin)['Items']; print(items[-1]['AccessToken'])")

echo ""
echo "=== Test Environment Ready ==="
echo "JELLYFIN_ENDPOINT=${JELLYFIN_URL}"
echo "JELLYFIN_API_KEY=${API_KEY}"
echo ""
echo "export JELLYFIN_ENDPOINT=${JELLYFIN_URL}"
echo "export JELLYFIN_API_KEY=${API_KEY}"
