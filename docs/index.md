---
page_title: "Jellyfin Provider"
subcategory: ""
description: |-
  The Jellyfin provider manages a Jellyfin media server instance.
---

# Jellyfin Provider

The Jellyfin provider allows you to manage a [Jellyfin](https://jellyfin.org/) media server instance.
It supports managing users, libraries, plugins, system configuration, and initial setup.

## Authentication

The provider supports API key and username/password authentication. All settings can also be configured via environment variables.

| Variable | Description |
|----------|-------------|
| `JELLYFIN_ENDPOINT` | Jellyfin server URL |
| `JELLYFIN_API_KEY` | API key for authentication |
| `JELLYFIN_USERNAME` | Username for authentication |
| `JELLYFIN_PASSWORD` | Password for authentication |

## Example Usage

```terraform
terraform {
  required_providers {
    jellyfin = {
      source = "ThePhaseless/jellyfin"
    }
  }
}

provider "jellyfin" {
  endpoint = "http://localhost:8096"
  api_key  = var.jellyfin_api_key
}
```

## Schema

### Optional

- `endpoint` (String, Optional) The URL of the Jellyfin server. Can also be set via `JELLYFIN_ENDPOINT`.
- `api_key` (String, Optional, Sensitive) The API key for authentication. Can also be set via `JELLYFIN_API_KEY`.
- `username` (String, Optional) Username for authentication. Can also be set via `JELLYFIN_USERNAME`.
- `password` (String, Optional, Sensitive) Password for authentication. Can also be set via `JELLYFIN_PASSWORD`.
