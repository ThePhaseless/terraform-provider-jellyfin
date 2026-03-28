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
provider "jellyfin" {
  endpoint = "http://localhost:8096"
  api_key  = var.jellyfin_api_key
}

variable "jellyfin_api_key" {
  type      = string
  sensitive = true
}
