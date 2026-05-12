terraform {
  required_providers {
    jellyfin = {
      source = "ThePhaseless/jellyfin"
    }
  }
}

# Configure the provider using environment variables:
# JELLYFIN_ENDPOINT - The Jellyfin server URL
# JELLYFIN_API_KEY  - The API key for authentication
# JELLYFIN_USERNAME - Username for authentication/bootstrap
# JELLYFIN_PASSWORD - Password for authentication/bootstrap
provider "jellyfin" {
  # The block can stay empty when auth is provided via JELLYFIN_* env vars.
}
