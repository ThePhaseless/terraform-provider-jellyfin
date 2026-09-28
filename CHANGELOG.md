# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `jellyfin_scheduled_task`: optional `key`, the readable task key Jellyfin lists (for example `PluginUpdates` or `RefreshLibrary`), instead of the hashed `task_id`, which is now optional and computed. `key` and `task_id` may both be set if they name the same task, so configuration written by `terraform plan -generate-config-out` plans cleanly. A key no task has, or one several tasks share, fails at plan time. Import accepts a key or an ID, and `jellyfin-import` writes `key`.
- `jellyfin_library`: `library_options.subtitle_fetchers`, the enabled subtitle fetchers in priority order. Every other fetcher the server offers for the library's content type is disabled, and one installed later shows up as a change.
- `jellyfin_system_configuration`: `metadata_options[].metadata_fetchers` and `image_fetchers`, the enabled fetchers for the entry's item type in priority order, with every other offered fetcher disabled. They read as null for item types Jellyfin offers no fetchers for, such as Person.

### Changed

- Every JSON-backed resource now maps its attributes to Jellyfin's keys from the committed Jellyfin OpenAPI and JellyfinSecurity schema goldens, instead of about 340 key names typed by hand in the provider and again in `jellyfin-import`. A CI test proves each attribute resolves to exactly one real key, so a misspelt key (the kind that left `enable_case_sensitive_item_ids` always null) now fails CI instead of shipping. Schema, state and the payloads sent to Jellyfin are unchanged; an acceptance test applies each resource with v0.3.8 and plans it with this version to prove it.
- `jellyfin_encoding_configuration`: setting `subtitle_extraction_timeout_minutes` or `hls_audio_seek_strategy` against a server older than Jellyfin 12.0 now fails at plan instead of apply, with the same message. The check uses the server version, so it also catches a value the server would have omitted. While such a value stays in the configuration, a destroy fails the same way.
- When Jellyfin does not keep a value the plan set, apply now reports "Value not kept by Jellyfin" at that attribute instead of Terraform's generic "inconsistent result after apply". On `jellyfin_library` this replaces the combined "Similar item settings not supported" error.
- `jellyfin-import` renders every configuration and scheduled task from the provider's own read, so its output cannot drift from what the provider reads.
- `jellyfin_library`: `type_options[].metadata_fetchers`, `image_fetchers` and `similar_item_providers` are now priority-ordered. Changing one also writes Jellyfin's matching order key, as the web UI does: the listed names first, then the rest of the server's order. A list left unchanged keeps its order, so existing configurations plan no change.
- `jellyfin_library` and `jellyfin_system_configuration` reads also call `GET /Libraries/AvailableOptions` to learn which providers the server offers. If that call fails, the read keeps the combined lists' prior values and warns instead of failing, and a combined list the apply did not write keeps its planned value, so a provider that a plugin installed in the same apply adds shows up at the next refresh instead of failing the apply.
- `jellyfin_networking_configuration`: `base_url` must be empty, or start with `/` and not end with one, as Jellyfin stores it; any other value read back changed and failed the apply.
- `jellyfin_security_plugin_configuration`: `webhook_url` and `webhook_headers` are sensitive. The lists the plugin stores as one joined string (`scopes`, `acr_values`, `allowed_groups`, `admin_groups`, `additional_allowed_cidrs`, `library_ids`) reject empty values and values holding their separator, which read back split.
- `jellyfin_user`: `policy.enabled_folders`, `blocked_media_folders`, `enabled_channels` and `blocked_channels` take IDs only as Jellyfin lists them, 32 lowercase hex digits; other spellings read back changed and failed the apply.
- The provider refuses an `endpoint`, `api_key`, `username` or `password` unknown at plan time, or defers when Terraform allows it, instead of falling back to the `JELLYFIN_*` environment variables, which could name another server.
- `jellyfin-import` imports each plugin once, as its Read reads it, skips plugin repositories that share a name, warns that `imports.tf` holds API key tokens, and no longer prints `JELLYFIN_API_KEY` or `JELLYFIN_PASSWORD` in its usage text.

### Deprecated

- `jellyfin_library`: `type_options[].metadata_fetcher_order`, `image_fetcher_order` and `similar_item_provider_order` (use the list attribute; while set, they still override the order), and `library_options.disabled_subtitle_fetchers` and `subtitle_fetcher_order` (use `subtitle_fetchers`).
- `jellyfin_system_configuration`: `metadata_options[].disabled_metadata_fetchers`, `metadata_fetcher_order`, `disabled_image_fetchers` and `image_fetcher_order` (use `metadata_fetchers` and `image_fetchers`).
- The deprecated attributes still work on their own. Setting a disabled or order list together with the combined attribute that replaces it is a plan-time error.

### Fixed

- `jellyfin_system_configuration`, `jellyfin_livetv_configuration`, `jellyfin_scheduled_task`: an attribute left unset inside a list element (for example a tuner host's `source` or a trigger's `day_of_week`) that Jellyfin serves back as an empty string or list now stays null instead of failing the apply.
- `jellyfin_system_configuration`: inserting or reordering `metadata_options` entries wrote another item type's fetcher, saver or reader lists into an entry that left them unset. Each entry now keeps its own values.
- `jellyfin_system_configuration`: a create that set some `trickplay_options` reset the others to Jellyfin's defaults; they now keep the server's values. Applying it while a `jellyfin_plugin_repository` changes no longer reverts the repository list.
- `jellyfin_security_plugin_configuration`: removing or reordering `oidc_providers` planned, and wrote, another provider's `client_secret` and settings into an entry, and appending one failed the apply; `user_emails` paired entries the same way. Each entry now keeps the values of the prior entry with its `id`, and each of its `role_library_mappings` those of the prior mapping with its `role`. `enrollment_deadline` can be cleared by setting it to `""`.
- `jellyfin_user`: `policy.access_schedules` paired schedules by index in the same way, and demoting and disabling an administrator in one apply always failed with "Administrators cannot be disabled".
- `jellyfin_livetv_configuration`: the first apply rebuilt an existing tuner or listing provider from the configured settings alone, dropping its type, name and the rest; an entry with the `id` of an existing one now keeps its settings.
- `jellyfin_library`: paths listed in another order than Jellyfin's sorted one failed the create and replaced the library on every plan; the configured order is kept, and a change of order alone updates in place.
- `jellyfin_plugin_configuration`: a configuration that named some of the plugin's keys planned a change after every refresh and failed every update. Only the keys it names are compared with the server's, so a key added outside Terraform, such as an SSO provider added on the plugin's page, plans no change and is removed by the next update; an import still reads every key.
- `jellyfin_plugin_repository`: repositories created, changed or destroyed in one apply dropped each other's changes, and renaming one failed with an inconsistent result.
- `jellyfin_api_key`: keys created in one apply could both take the same token.
- `jellyfin_plugin`: an install of a version no repository offers now fails at once and says so, instead of waiting two minutes for the plugin.
- `jellyfin_restart`: changing only `timeout` no longer fails with "Update not supported".

## [0.3.8] - 2026-09-27

### Changed

- Declared support for Jellyfin 12.1 and JellyfinSecurity plugin 2.6.3.1 (versions tested in CI).

### Added

- `jellyfin_encoding_configuration`: `hls_audio_seek_strategy` and `subtitle_extraction_timeout_minutes`, new in Jellyfin 12. Setting them against an older server fails with an error naming the Jellyfin 12 requirement.
- `jellyfin_library`: `type_options[].similar_item_providers` and `similar_item_provider_order` (Jellyfin 12), and `type_options[].image_options[].min_width`. A configured `type_options` entry is now overlaid onto the server's entry for that type, so values the provider does not manage survive an apply.
- `jellyfin_security_plugin_configuration`: `pair_device_on_second_screen_approval`, `public_base_url` and `oidc_providers[].link_existing_users_by_username`. The last existed in the plugin already, and every apply used to reset it to false.
- `jellyfin_plugin`: computed `installed_version`, the version Jellyfin lists for the plugin.

### Changed

- JellyfinSecurity support moves to the 2.6.3.1 build, which targets the Jellyfin 12 ABI. `jellyfin_plugin` resolves `supported` to the build the server can load.
- `jellyfin_plugin`: a `latest` or `supported` keyword now stays in `version` and is resolved only when the plugin is installed; `installed_version` reports the result. Switching between a keyword and the version it names updates state in place instead of reinstalling. A keyword the repositories cannot resolve fails the plan.
- `jellyfin_plugin_configuration` and `jellyfin_security_plugin_configuration`: `plugin_id` accepts the GUID with or without dashes, so importing with the other spelling no longer plans a replacement.
- Enum attributes are validated at plan time against the values Jellyfin accepts: the encoding configuration's seven enums, library image option types, scheduled task trigger types and days, and the system configuration's enums. Jellyfin 12 answers an invalid or empty enum value with a 500, and older servers silently rewrite it.
- `jellyfin_user`: `policy.max_parental_rating` and `policy.max_parental_sub_rating` are no longer computed. Setting one to null removes the limit, and in a configured `policy` block leaving one out also removes a limit set outside Terraform. Without a `policy` block the policy is still read from the server.
- `jellyfin_user` updates users and passwords through the documented `POST /Users?userId=` and `POST /Users/Password?userId=` routes; Jellyfin 12 no longer documents the `/Users/{id}` forms.
- `jellyfin_scheduled_task`: trigger attributes are no longer computed, so an omitted value plans as null. Each trigger type's required attributes are checked at plan time, and `time_of_day_ticks` and `max_runtime_ticks` are range-checked.
- `jellyfin_library`: `disabled` now maps to Jellyfin's `Enabled`, and `extract_chapters_during_library_scan` to `ExtractChapterImagesDuringLibraryScan`, so both take effect. Creating a library whose name is already in use fails and suggests an import, instead of adopting the existing library while Jellyfin creates a copy named `<name>2`.
- `jellyfin-import` writes typed attributes for every configuration resource and for scheduled task triggers (it used to write `configuration_json` and `triggers_json`, which no longer exist), escapes `${` and `%{` in strings, declares `required_providers`, and writes files `terraform fmt` accepts. Libraries without a collection type are imported as `mixed`.

### Deprecated

- `jellyfin_library`: 15 `library_options` attributes and `path_infos[].username` and `password` write keys Jellyfin's library options do not have, so setting any of them is now an error. `path_infos[].network_path` is rejected on Jellyfin 10.10 and later, which removed network paths. They will be removed in a future release.
- `jellyfin_branding_configuration`: `splashscreen_location`, which Jellyfin ignores; setting it is now an error.

### Fixed

- Acceptance tests had silently skipped in CI: the Jellyfin setup script failed without the compose env file, and a missing server skipped instead of failing. They now run, and a missing server fails the run when `TF_ACC` is set.
- Importing the configuration singletons failed with a `Value Conversion Error` on the first list attribute ([#113](https://github.com/ThePhaseless/terraform-provider-jellyfin/issues/113)).
- `jellyfin_library`: updating `library_options` sent an empty item id and failed with "Guid can't be empty" ([#116](https://github.com/ThePhaseless/terraform-provider-jellyfin/issues/116)); creating a library without `library_options` failed with a `Value Conversion Error`; a create Jellyfin rejects now names the paths it cannot find inside the server's filesystem ([#126](https://github.com/ThePhaseless/terraform-provider-jellyfin/issues/126)).
- `jellyfin_user`: creating a user without a `policy` block failed with a `Value Conversion Error`, and policy attributes stayed unknown after apply, tainting the resource on every run ([#115](https://github.com/ThePhaseless/terraform-provider-jellyfin/issues/115)). Every policy update also wiped `MaxParentalSubRating`, and a rename reset the user's display preferences on Jellyfin 12.
- `jellyfin_encoding_configuration`: `encoder_preset = ""` returned a 500 on Jellyfin 12 and an inconsistent result on 10.11.
- `jellyfin_scheduled_task`: omitted trigger attributes stayed unknown after apply, and changing a trigger's type re-sent the previous type's values.
- `jellyfin_livetv_configuration`: adding, inserting or removing a tuner host or listing provider applied the neighbouring entry's settings, and pointing the recording path at an existing directory failed with an inconsistent result.
- `jellyfin_security_plugin_configuration`: `enrollment_deadline` always failed with an inconsistent result, because the plugin rewrites the timestamp's layout.
- `jellyfin_plugin`: `version = "latest"` or `"supported"` failed every apply with an inconsistent result. Destroying a plugin left behind a second version Jellyfin's update task had installed next to it; under `create_before_destroy` a version change removed the replacement too. Destroying a plugin Jellyfin bundles and does not let users uninstall (TMDb, OMDb, MusicBrainz and others) now succeeds with a warning instead of failing. Concurrent installs and uninstalls are serialised.
- `jellyfin_system_configuration`: `enable_normalized_item_by_name_ids`, `enable_case_sensitive_item_ids` and `trickplay_options.process_priority` read keys Jellyfin does not send, so they were always null.

## [0.3.7] - 2026-09-15

### Changed

- Declared support for Jellyfin 12.1 and JellyfinSecurity plugin 2.5.22.0 (versions tested in CI).

## [0.3.6] - 2026-09-08

### Changed

- Declared support for Jellyfin 12.0 and JellyfinSecurity plugin 2.5.22.0 (versions tested in CI).

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
