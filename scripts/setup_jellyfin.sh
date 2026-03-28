#!/usr/bin/env bash
# setup_jellyfin.sh - Sets up a fresh Jellyfin instance for testing.
# This script completes the startup wizard and creates an API key.
set -euo pipefail

JELLYFIN_URL="${JELLYFIN_ENDPOINT:-http://localhost:8096}"
MAX_WAIT=60

echo "Waiting for Jellyfin to start at ${JELLYFIN_URL}..."
for i in $(seq 1 "$MAX_WAIT"); do
    if curl -sf "${JELLYFIN_URL}/System/Info/Public" > /dev/null 2>&1; then
        echo "Jellyfin is up!"
        break
    fi
    if [ "$i" -eq "$MAX_WAIT" ]; then
        echo "ERROR: Jellyfin did not start within ${MAX_WAIT}s"
        exit 1
    fi
    sleep 1
done

# Check if startup wizard is already completed.
WIZARD_COMPLETED=$(curl -sf "${JELLYFIN_URL}/System/Info/Public" | python3 -c "import sys,json; print(json.load(sys.stdin).get('StartupWizardCompleted', False))" 2>/dev/null || echo "False")

if [ "$WIZARD_COMPLETED" = "True" ]; then
    echo "Startup wizard already completed."
else
    echo "Completing startup wizard..."

    # Set startup configuration.
    curl -sf -X POST "${JELLYFIN_URL}/Startup/Configuration" \
        -H "Content-Type: application/json" \
        -d '{"UICulture":"en-US","MetadataCountryCode":"US","PreferredMetadataLanguage":"en"}'

    # Set startup user.
    curl -sf -X POST "${JELLYFIN_URL}/Startup/User" \
        -H "Content-Type: application/json" \
        -d '{"Name":"admin","Password":"admin123"}'

    # Complete startup.
    curl -sf -X POST "${JELLYFIN_URL}/Startup/Complete"

    echo "Startup wizard completed."
fi

# Authenticate to get a token.
echo "Authenticating..."
AUTH_RESPONSE=$(curl -sf -X POST "${JELLYFIN_URL}/Users/AuthenticateByName" \
    -H "Content-Type: application/json" \
    -H 'Authorization: MediaBrowser Client="Terraform", Device="Setup", DeviceId="setup-script", Version="1.0.0"' \
    -d '{"Username":"admin","Pw":"admin123"}')

ACCESS_TOKEN=$(echo "$AUTH_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['AccessToken'])")
echo "Got access token."

# Create an API key.
echo "Creating API key..."
curl -sf -X POST "${JELLYFIN_URL}/Auth/Keys" \
    -H "Authorization: MediaBrowser Token=\"${ACCESS_TOKEN}\"" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "app=terraform-provider-test"

# Fetch the API key.
API_KEYS_RESPONSE=$(curl -sf "${JELLYFIN_URL}/Auth/Keys" \
    -H "Authorization: MediaBrowser Token=\"${ACCESS_TOKEN}\"")

API_KEY=$(echo "$API_KEYS_RESPONSE" | python3 -c "import sys,json; items=json.load(sys.stdin)['Items']; print(items[-1]['AccessToken'])")

echo ""
echo "=== Test Environment Ready ==="
echo "JELLYFIN_ENDPOINT=${JELLYFIN_URL}"
echo "JELLYFIN_API_KEY=${API_KEY}"
echo ""
echo "export JELLYFIN_ENDPOINT=${JELLYFIN_URL}"
echo "export JELLYFIN_API_KEY=${API_KEY}"
