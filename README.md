# Terraform Provider for Jellyfin

A [Terraform](https://www.terraform.io) provider for managing [Jellyfin](https://jellyfin.org/) media server instances.

## Features

- **User Management** — Create, update, and delete users with policy control
- **Library Management** — Configure media libraries with custom paths and options
- **Plugin Repositories** — Manage plugin repository sources
- **Plugin Installation** — Install and uninstall plugins from repositories
- **Plugin Configuration** — Universal plugin settings via JSON (supports SSO-Auth, and any other plugin)
- **System Configuration** — Full server configuration management
- **Encoding Configuration** — Transcoding and hardware acceleration settings
- **Initial Setup** — Configure a fresh Jellyfin instance after installation
- **Data Sources** — Read server information and status

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) at or above the `go` line in `go.mod` to build the provider, and in `tools/go.mod` to run `make generate`
- A running Jellyfin server instance

## Quick Start

```hcl
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

# Read server info
data "jellyfin_system_info" "server" {}

# Create a user
resource "jellyfin_user" "viewer" {
  name             = "viewer"
  password         = "secret123"
  is_administrator = false
}

# Add a movie library
resource "jellyfin_library" "movies" {
  name            = "Movies"
  collection_type = "movies"
  paths           = ["/media/movies"]
}

# Add a plugin repository
resource "jellyfin_plugin_repository" "stable" {
  name    = "Jellyfin Stable"
  url     = "https://repo.jellyfin.org/files/plugin/manifest.json"
  enabled = true
}

# Configure the server
resource "jellyfin_system_configuration" "main" {
  server_name = "My Jellyfin"
}
```

## Authentication

The provider supports two authentication methods:

### API Key (Recommended)

```hcl
provider "jellyfin" {
  endpoint = "http://localhost:8096"
  api_key  = "your-api-key"
}
```

### Username/Password and Bootstrap

```hcl
provider "jellyfin" {
  endpoint = "http://localhost:8096"
  username = "admin"
  password = "password"
}
```

If the Jellyfin startup wizard has not been completed yet, configure the
provider with `username` and `password`. The provider uses them to create the
initial admin user, completes the wizard, and then authenticates with that user.
After bootstrap, either API key or username/password authentication can be used.

### Environment Variables

All provider attributes can be set via environment variables:

| Variable | Description |
|----------|-------------|
| `JELLYFIN_ENDPOINT` | Jellyfin server URL |
| `JELLYFIN_API_KEY` | API key for authentication |
| `JELLYFIN_USERNAME` | Username for authentication |
| `JELLYFIN_PASSWORD` | Password for authentication |

You can keep the provider block empty and rely entirely on `JELLYFIN_*`
environment variables for authentication, including bootstrap:

```hcl
provider "jellyfin" {}
```

## Universal Plugin Configuration

Any plugin can be configured using the `jellyfin_plugin_configuration` resource with JSON:

```hcl
# SSO-Auth Plugin Configuration
resource "jellyfin_plugin_configuration" "sso" {
  plugin_id = jellyfin_plugin.sso_auth.id

  # SSO-Auth keys its providers by name.
  configuration_json = jsonencode({
    SamlConfigs = {}
    OidConfigs = {
      authelia = {
        OidClientId = "jellyfin"
        OidSecret   = var.oidc_secret
        OidEndpoint = "https://auth.example.com"
        Enabled     = true
      }
    }
  })
}
```

## Development

### Building

```shell
go install
```

This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

### Testing

Start a local Jellyfin instance:

```shell
docker compose --env-file internal/provider/supported_jellyfin_version.env up -d
eval "$(./scripts/setup_jellyfin.sh | grep '^export ')"
```

Run acceptance tests:

```shell
TF_ACC=1 go test -v ./internal/provider/
```

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

### Linting

```shell
golangci-lint run
```

### Documentation

To generate or update documentation, run `make generate`.
