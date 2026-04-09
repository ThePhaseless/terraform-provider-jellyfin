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
- [Go](https://golang.org/doc/install) >= 1.24
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

### Username/Password

```hcl
provider "jellyfin" {
  endpoint = "http://localhost:8096"
  username = "admin"
  password = "password"
}
```

### Environment Variables

All provider attributes can be set via environment variables:

| Variable | Description |
|----------|-------------|
| `JELLYFIN_ENDPOINT` | Jellyfin server URL |
| `JELLYFIN_API_KEY` | API key for authentication |
| `JELLYFIN_USERNAME` | Username for authentication |
| `JELLYFIN_PASSWORD` | Password for authentication |

## Universal Plugin Configuration

Any plugin can be configured using the `jellyfin_plugin_configuration` resource with JSON:

```hcl
# SSO-Auth Plugin Configuration
resource "jellyfin_plugin_configuration" "sso" {
  plugin_id = jellyfin_plugin.sso_auth.id

  configuration_json = jsonencode({
    SamlConfigs = []
    OidConfigs = [{
      OidClientId = "jellyfin"
      OidSecret   = var.oidc_secret
      OidEndpoint = "https://auth.example.com"
      Enabled     = true
    }]
  })
}
```

## Development

### Building

```shell
go install
```

### Testing

Start a local Jellyfin instance:

```shell
docker compose up -d
./scripts/setup_jellyfin.sh
```

Run acceptance tests:

```shell
export JELLYFIN_ENDPOINT=http://localhost:8096
export JELLYFIN_API_KEY=<from setup script>
TF_ACC=1 go test -v ./internal/provider/
```

### Linting

```shell
golangci-lint run
```

## Using the provider

Fill this in for each provider

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `make generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```
