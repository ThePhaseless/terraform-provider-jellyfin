# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- `jellyfin_plugin`: wait for the requested *version* to appear, not just the plugin name. Jellyfin's install is asynchronous and returns well before the download lands, so a name-only match let create return while the previous version was still the only one on disk — and a `jellyfin_restart` sequenced after it would reload that older assembly. The same match now also decides whether an install is needed at all, so changing the pinned version no longer sees the old version and skips the install. Three- and four-segment versions compare equal, so `2.5.22` and `2.5.22.0` both work.

## [0.3.4] - 2026-08-25

### Changed

- Declared support for Jellyfin 10.11.11 and JellyfinSecurity plugin 2.5.22.0 (versions tested in CI).

## [0.3.3] - 2026-08-25

### Fixed

- `jellyfin_plugin`: the tracked JellyfinSecurity version is again the four-segment assembly version (`2.5.22.0`). Jellyfin resolves an install against `manifest.json`, which carries assembly versions, so the three-segment git tag introduced in 0.3.2 matched nothing and made an install a silent no-op — no error, no log line, no plugin.
- `jellyfin_restart`: wait for the server to answer three times running after a settle delay, rather than requiring it to stop answering. Jellyfin restarts in-process rather than exiting, so it may never go away; and `HasPendingRestart` cannot gate the wait either, because background plugin auto-updates raise it again within seconds of a restart clearing it.

## [0.3.2] - 2026-08-25

### Fixed

- `jellyfin_restart`: wait for the server to stop answering before waiting for it to come back. Jellyfin keeps serving for a moment after accepting `POST /System/Restart`, so the previous wait polled the outgoing process, saw `HasPendingRestart=false`, and reported the restart complete within seconds. A following resource then wrote configuration that the restart discarded, or failed outright because the plugin had not loaded yet.

## [0.3.1] - 2026-08-25

### Changed

- Declared support for Jellyfin 10.11.11 and JellyfinSecurity plugin 2.5.22 (versions tested in CI).

### Added

- `jellyfin_security_plugin_configuration`: `ntfy_token`, `ntfy_username` and `ntfy_password` for authenticated ntfy topics, and `webhook_headers` for webhook receivers that authenticate by header (JellyfinSecurity 2.5.21).
- `jellyfin_security_plugin_configuration`: `rp_initiated_logout_enabled` and `rp_initiated_logout_redirect_uri` on `oidc_providers`, ending the IdP session on Jellyfin sign-out.

### Fixed

- Renovate wrote the JellyfinSecurity version with its `v` tag prefix, which left `compareDottedVersions` unable to parse it and broke the release workflow's version extraction.
- The plugin schema-change check fetched a non-existent `v{major}.{minor}.{patch}.0` tag and, because `curl` was not run with `--fail`, silently compared two empty property lists and passed every bump.

## [0.2.4] - 2026-07-27

### Changed

- Declared support for Jellyfin 10.11.11 and JellyfinSecurity plugin 2.5.20.0 (versions tested in CI).

## [0.2.3] - 2026-07-22

### Fixed

- `jellyfin_sso_plugin_configuration`: write resource state after Create/Update/Read. The `apply` and `read` methods mutated the model but never called `state.Set`, causing Terraform to report "Missing Resource State After Create".

## [0.2.2] - 2026-07-22

### Fixed

- `jellyfin_sso_plugin_configuration`: compare the installed SSO-Auth plugin ID with the provider’s hardcoded GUID in a case- and dash-insensitive way. Jellyfin returns the installed plugin ID without dashes, which caused the preinstall check to incorrectly reject servers that already had the plugin.

## [0.2.1] - 2026-07-22

### Fixed

- Republished `v0.2.0` as `v0.2.1` because the Terraform Registry cached stale checksums after the initial release artifacts were replaced.

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

[Unreleased]: https://github.com/ThePhaseless/terraform-provider-jellyfin/compare/v0.2.3...HEAD
[0.2.3]: https://github.com/ThePhaseless/terraform-provider-jellyfin/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/ThePhaseless/terraform-provider-jellyfin/compare/v0.2.1...v0.2.2
[0.2.0]: https://github.com/ThePhaseless/terraform-provider-jellyfin/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/ThePhaseless/terraform-provider-jellyfin/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/ThePhaseless/terraform-provider-jellyfin/releases/tag/v0.1.0
