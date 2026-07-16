# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.1] - 2026-07-16

### Fixed

- `jellyfin_plugin` resource: import now works by plugin name (e.g. `terraform import jellyfin_plugin.x "SSO-Auth"`). Previously, `ImportState` passed the import ID through as the resource `id`, causing `Read` to fail matching against installed plugins and silently removing the resource from state (#84).
- `jellyfin_plugin` resource: `Create` now detects when a plugin is already installed and treats it as idempotent instead of erroring with a 404 from Jellyfin (#84).

### Changed

- Updated various dependencies (Go modules, GitHub Actions, Docker images, devcontainer features).

## [0.1.0] - 2026-05-13

### Added

- Initial Terraform provider implementation for managing Jellyfin users, libraries, plugins, API keys, scheduled tasks, and server configuration.

[Unreleased]: https://github.com/ThePhaseless/terraform-provider-jellyfin/compare/v0.1.0...HEAD
[0.1.1]: https://github.com/ThePhaseless/terraform-provider-jellyfin/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/ThePhaseless/terraform-provider-jellyfin/releases/tag/v0.1.0
