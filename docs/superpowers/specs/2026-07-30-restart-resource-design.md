# Design: `jellyfin_restart` resource + `pending_restart` on `jellyfin_system_info`

Date: 2026-07-30
Status: Approved (approach A) — pending spec review

## Goal

Let users (1) **observe** when Jellyfin has a pending restart, and (2) **trigger** an
explicit, ordered, blocking restart from Terraform — primarily so a
`jellyfin_plugin` → `jellyfin_plugin_configuration` / `jellyfin_security_plugin_configuration`
workflow succeeds in a single `terraform apply`.

## Background / problem (evidence)

- `jellyfin_plugin` Create installs the plugin, then `waitForPlugin` polls `/Plugins`
  until it appears in the **installed list** — which Jellyfin populates immediately,
  *before the plugin is loaded* (`internal/provider/plugin_resource.go:168-174`, `:242-258`).
  State goes green, but the plugin is not active.
- Plugin configuration resources write `/Plugins/{id}/Configuration`
  (`internal/client/plugins.go:117-123`), an endpoint served by the **loaded** plugin.
  - `jellyfin_plugin_configuration` (generic) has **no guard** — it POSTs straight to
    that endpoint (`plugin_configuration_resource.go:109`).
  - `jellyfin_security_plugin_configuration` guards with `requireJellyfinSecurityPluginInstalled`,
    which only checks `GetInstalledPlugins` (the installed list, populated pre-restart) —
    so it **passes even when the plugin is not loaded**, then `apply` calls
    `GetPluginConfiguration` and fails (`jellyfin_security_plugin_configuration_resource.go:506-518`, `:550`).
- Jellyfin only loads plugins on startup/restart. `[INFERENCE — Jellyfin runtime behavior.
  The repo proves the provider writes that endpoint and only guards on the installed list.
  Exact pre-restart failure mode (404 vs. default-config drift) to be confirmed by a smoke
  test against the bundled docker-compose during implementation.]`
- No restart endpoint is called anywhere in the provider (grep for `/System/Restart`,
  `RestartServer`, `Restart(`, `Shutdown` → no matches). `HasPendingRestart` is parsed
  in `SystemInfo` (`internal/client/system.go:19`) but never surfaced — the
  `jellyfin_system_info` data source model/schema/Read omit it
  (`system_info_data_source.go:30-36`, `:47-72`, `:100-106`).

De-facto workflow today: **apply (fails at the config step) → manual restart → apply again.**

## Non-goals

- Auto-restarting on arbitrary config changes.
- Surfacing `pending_restart` on `jellyfin_plugin` itself (YAGNI; the data source is the
  natural observation point).
- Changing the behavior of existing resources (plugin install, plugin configuration).
- Making the restart implicit or automatic — it stays strictly opt-in.

## Design

### Component 1 — `pending_restart` on the `jellyfin_system_info` data source

A pure, additive change that revives the currently-dead `HasPendingRestart` field.

- Add computed `BoolAttribute` `pending_restart` to the `jellyfin_system_info` schema.
- Add `PendingRestart types.Bool` to `SystemInfoDataSourceModel`.
- In `Read`, set it from `info.HasPendingRestart`.
- No behavior change to anything else; the field is already returned by `GetSystemInfo`.

This gives automation a way to **observe** pending-restart state independently of the
restart resource, and stops the provider from silently swallowing the signal.

### Component 2 — `jellyfin_restart` resource (approach A)

A **trigger-based action resource** modeled on `null_resource` / `terraform_data` — the
community-standard pattern for bolting a point-in-time action onto Terraform.

#### Schema

| Attribute    | Type                | Required | Computed | Notes |
|--------------|---------------------|----------|----------|-------|
| `id`         | String              |          | yes      | Generated ID (UUID) for state identity. |
| `triggers`   | Map(String→String)  | optional |          | When any value changes between state and plan, the resource is **replaced** → the restart re-fires. Unchanged → no-op on apply. |
| `timeout`    | Int64 (seconds)     | optional |          | Max time to wait for the server to come back up. Default **120**. |
| `completed_at` | String (RFC3339)  |          | yes      | Timestamp the restart wait finished. |

- A change to `triggers` forces replacement (Delete no-op + Create). Implemented via a
  framework plan modifier that sets `RequiresReplace` on `triggers` diff. Exact API to be
  confirmed against the `terraform-plugin-framework` version in `go.mod` during planning.
- `timeout` default 120s via the framework's default-value mechanism, matching whichever
  convention existing resources use (to be confirmed during planning).

#### CRUD behavior

- **Create**: `POST /System/Restart`, then **block** until the server is back up (see
  "Blocking wait" below). On success, set `id` (UUID), echo `triggers`, set `completed_at`.
- **Read**: keep state as-is (the action is point-in-time; no server query needed).
- **Update**: not supported — `triggers` changes force replacement, so Update is never
  reached. Add a guard diagnostic mirroring `plugin_resource.go:222-225`.
- **Delete**: no-op (we do not "un-restart" or revert). Just remove from state.
- **Import**: not supported (an action resource has nothing meaningful to import).

#### First-apply semantics

Creating the resource **always restarts**, even on first apply. This is the honest
`null_resource` contract: adding the resource fires the action. Users who do not want a
restart do not add the resource. Documented explicitly in the resource description.

#### Blocking wait (required, not fire-and-forget)

Blocking is what makes the `plugin → restart → plugin_configuration` workflow work in one
apply — a fire-and-forget restart would race the configuration resource that follows it.

After `POST /System/Restart` the HTTP API becomes unavailable (connection refused / 503),
then returns. Poll using the **existing** retry pattern from `getPublicSystemInfo`
(`internal/provider/provider.go:209-231`, `startupStatusDelay = 1s`):

1. Poll `GET /System/Info/Public` (no-auth, available earliest), retrying on connection
   errors and 503, until it responds 200 — server is back up.
2. Then `GET /System/Info` and confirm `HasPendingRestart == false` — restart fully
   completed and plugins loaded. `[INFERENCE: HasPendingRestart clears after restart
   completes; confirm in smoke test.]` If still pending once the API is up, keep polling
   within the remaining `timeout` budget.

Bounded by `timeout` (default 120s). On timeout: return a diagnostic error and do **not**
set state, so the next apply re-fires Create. Acknowledged trade-off: a re-fire after a
timed-out-but-still-running restart may issue a second `POST /System/Restart`. Mitigated by
a generous default timeout and fast polling; documented as a known edge case.

#### Reusable pieces

- `internal/client/system.go`: add `RestartServer(ctx) error` → `c.post(ctx, "/System/Restart", nil)`.
- Provider helper `waitForServerReady(ctx, timeout) error` (new, in the provider package),
  reusing `startupStatusDelay` and mirroring the `getPublicSystemInfo` loop, calling
  `client.GetPublicSystemInfo` then `client.GetSystemInfo` for the `HasPendingRestart` check.

#### Registration & docs

- Register `NewRestartResource()` in `Resources()` (`internal/provider/provider.go`).
- Generate docs via `tfplugindocs` (`docs/resources/restart.md`) and add an example under
  `examples/resources/restart/`.
- Update `docs/data-sources/system_info.md` (generated) to reflect `pending_restart`.

### Data flow (the motivating workflow)

```hcl
resource "jellyfin_plugin" "sso_auth" { name = "JellyfinSecurity"; /* ... */ }

resource "jellyfin_restart" "load_plugins" {
  triggers = { plugin_version = jellyfin_plugin.sso_auth.version }
}

resource "jellyfin_security_plugin_configuration" "sso" {
  plugin_id  = jellyfin_plugin.sso_auth.id
  depends_on = [jellyfin_restart.load_plugins]
  /* ... */
}
```

User changes `jellyfin_plugin.sso_auth.version` → `jellyfin_restart.triggers.plugin_version`
changes → restart resource planned for replace → apply: Delete(no-op) + Create(POST
`/System/Restart`, wait ready) → `jellyfin_security_plugin_configuration` (depends_on the
restart) runs against a **loaded** plugin. One apply, end to end.

## Files

- New: `internal/provider/restart_resource.go`
- New: `internal/provider/restart_resource_unit_test.go`, `internal/provider/restart_resource_test.go`
- Edit: `internal/client/system.go` — add `RestartServer`
- Edit: `internal/provider/system_info_data_source.go` — add `pending_restart`
- Edit: `internal/provider/system_info_data_source_test.go` (+ unit test) — assert `pending_restart`
- Edit: `internal/provider/provider.go` — register `NewRestartResource`
- New/generated: `docs/resources/restart.md`, `examples/resources/restart/...`
- Generated: `docs/data-sources/system_info.md`

## Testing

- **Unit tests** for the restart resource schema/plan modifiers, following existing
  `*_unit_test.go` patterns (e.g. `plugin_repository_resource_unit_test.go`).
- **Acceptance test** (`restart_resource_test.go`) exercising the full workflow against the
  bundled docker-compose Jellyfin: install plugin → restart → configure, following
  `plugin_resource_test.go` / `plugin_configuration_resource_test.go` patterns.
- **Smoke test** (folded into the acc test or run manually): confirm the pre-restart
  behavior of `/Plugins/{id}/Configuration` (the `[INFERENCE]` above) — i.e. that the
  config resource genuinely needs the restart.

## Open items for the implementation plan

- Confirm the exact pre-restart failure mode of `/Plugins/{id}/Configuration` (smoke test).
- Confirm the framework plan-modifier API for `RequiresReplace` on a map attribute and the
  default-value convention used by existing resources, against the `go.mod` framework version.
- Confirm `HasPendingRestart` clears promptly after restart (smoke test).
