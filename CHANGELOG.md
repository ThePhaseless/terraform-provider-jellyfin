# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-07-22

### Breaking Changes

- Removed raw JSON attributes from all typed configuration resources. `jellyfin_branding_configuration`, `jellyfin_metadata_configuration`, `jellyfin_networking_configuration`, `jellyfin_encoding_configuration`, `jellyfin_livetv_configuration`, `jellyfin_system_configuration`, `jellyfin_user`, and `jellyfin_scheduled_task` now expose fully typed Terraform attributes.
- `jellyfin_user` no longer accepts `policy_json`; use the typed `policy` block.
- `jellyfin_library` no longer accepts `library_options_json`; use the typed `library_options` block.

### Added

- New `jellyfin_sso_plugin_configuration` resource with typed `oid_configs` and `saml_configs` maps. Server-managed fields such as `CanonicalLinks` are omitted from the configuration and no longer drift.
- Runtime version warnings when the Jellyfin server or installed SSO plugin is newer than the tested versions.
- Single-source version files (`internal/provider/supported_jellyfin_version.env`, `internal/provider/supported_sso_plugin_version.env`) managed by Renovate and interpolated into CI, Docker Compose, and the provider binary.
- Schema guards for the Jellyfin OpenAPI surface and the SSO plugin payload.
- Auto patch release workflow when a version file changes on `main`.

### Changed

- Docker Compose image tag is now sourced from `supported_jellyfin_version.env`.

## [0.1.1] - 2026-07-16

### Fixed

- `jellyfin_plugin` resource: import now works by plugin name (e.g. `terraform import jellyfin_plugin.x "SSO-Auth"`). Previously, `ImportState` passed the import ID through as the resource `id`, causing `Read` to fail matching against installed plugins and silently removing the resource from state (#84).
- `jellyfin_plugin` resource: `Create` now detects when a plugin is already installed and treats it as idempotent instead of erroring with a 404 from Jellyfin (#84).

### Changed

- Updated various dependencies (Go modules, GitHub Actions, Docker images, devcontainer features).

## [0.1.0] - 2026-05-13

### Added

- Initial Terraform provider implementation for managing Jellyfin users, libraries, plugins, API keys, scheduled tasks, and server configuration.

[Unreleased]: https://github.com/ThePhaseless/terraform-provider-jellyfin/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/ThePhaseless/terraform-provider-jellyfin/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/ThePhaseless/terraform-provider-jellyfin/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/ThePhaseless/terraform-provider-jellyfin/releases/tag/v0.1.0
