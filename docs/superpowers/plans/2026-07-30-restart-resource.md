# jellyfin_restart Resource Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `jellyfin_restart` action resource (trigger-based, blocking) and a `pending_restart` computed attribute on the `jellyfin_system_info` data source, so a `jellyfin_plugin` → `jellyfin_restart` → `jellyfin_plugin_configuration` workflow succeeds in a single apply.

**Architecture:** A new resource modeled on `null_resource`/`terraform_data`: a `triggers` map forces replacement (so a restart re-fires only when a trigger value changes); `Create` POSTs `/System/Restart` then blocks polling `/System/Info` until `HasPendingRestart == false`. `Read`/`Delete` are no-ops; `Update` is unsupported (replace-only). The data-source change revives the already-parsed `HasPendingRestart` field that is currently parsed but never surfaced.

**Tech Stack:** Go 1.25.8, terraform-plugin-framework v1.19.0, terraform-plugin-framework-validators v0.19.0, terraform-plugin-testing v1.16.0. Docs via `tfplugindocs` (`make generate`). Verified against Jellyfin 10.11.11 — see `docs/superpowers/specs/2026-07-30-restart-resource-design.md`.

## Global Constraints

- Every new `.go` file starts with the license header (matches existing files):
  ```go
  // Copyright IBM Corp. 2021, 2025
  // SPDX-License-Identifier: MPL-2.0
  ```
- Run `make fmt` (`gofmt -s -w -e .`) before every commit.
- Unit tests run with `make test` (no live server needed for the tests in this plan). Acceptance tests run with `TF_ACC=1` and a live Jellyfin.
- Resource type name: `jellyfin_restart`. Data-source attribute: `pending_restart`.
- The restart is strictly opt-in and always fires on create/replace (the `null_resource` contract). No auto-restart, no import.

---

## File Structure

- **Create** `internal/provider/restart_resource.go` — the `jellyfin_restart` resource: model, schema, CRUD, and two helpers (`waitForServerReady`, `randomID`).
- **Create** `internal/provider/restart_resource_unit_test.go` — unit tests for the helpers (httptest-based, no live server).
- **Create** `internal/provider/restart_resource_test.go` — gated acceptance test for the full workflow.
- **Create** `examples/resources/jellyfin_restart/resource.tf` — example for tfplugindocs.
- **Modify** `internal/client/system.go` — add `RestartServer`.
- **Modify** `internal/client/client_test.go` — add a unit test for `RestartServer`.
- **Modify** `internal/provider/system_info_data_source.go` — add `pending_restart` (model, schema, Read).
- **Modify** `internal/provider/system_info_data_source_test.go` — assert `pending_restart`.
- **Modify** `internal/provider/provider.go` — register `NewRestartResource`.
- **Generated** `docs/resources/restart.md` and updated `docs/data-sources/system_info.md` via `make generate`.

---

### Task 1: Client `RestartServer` method

**Files:**
- Modify: `internal/client/system.go` (add method after `UpdateSystemConfiguration`, ~line 98)
- Test: `internal/client/client_test.go` (append a test)

**Interfaces:**
- Consumes: existing `(*Client).post` helper (`internal/client/client.go:116`).
- Produces: `func (c *Client) RestartServer(ctx context.Context) error` — used by Task 3.

- [ ] **Step 1: Write the failing test**

Append to `internal/client/client_test.go`:

```go
func TestRestartServerPostsSystemRestart(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := NewClient(server.URL, "test-key").RestartServer(context.Background()); err != nil {
		t.Fatalf("RestartServer() error = %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/System/Restart" {
		t.Fatalf("expected POST /System/Restart, got %s %s", gotMethod, gotPath)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/client/ -run TestRestartServerPostsSystemRestart -v`
Expected: compile error — `c.RestartServer undefined (type *Client has no field or method RestartServer)`.

- [ ] **Step 3: Implement `RestartServer`**

Add to `internal/client/system.go`, immediately after the `UpdateSystemConfiguration` function (after line 98):

```go
// RestartServer restarts the Jellyfin server. The HTTP API becomes unavailable
// while the server restarts; callers should poll GetSystemInfo until
// HasPendingRestart is false before issuing further requests.
func (c *Client) RestartServer(ctx context.Context) error {
	if err := c.post(ctx, "/System/Restart", nil); err != nil {
		return fmt.Errorf("restarting server: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/client/ -run TestRestartServerPostsSystemRestart -v`
Expected: PASS.

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add internal/client/system.go internal/client/client_test.go
git commit -m "feat(client): add RestartServer for POST /System/Restart"
```

---

### Task 2: `pending_restart` on the `jellyfin_system_info` data source

**Files:**
- Modify: `internal/provider/system_info_data_source.go` (model ~line 30-36, schema ~line 47-72, Read ~line 100-106)
- Test: `internal/provider/system_info_data_source_test.go` (extend the existing `Check`)

**Interfaces:**
- Consumes: existing `(*client.Client).GetSystemInfo` returning `SystemInfo` (which already parses `HasPendingRestart`, `internal/client/system.go:19`).
- Produces: a computed `pending_restart` (Bool) attribute on the `jellyfin_system_info` data source.

- [ ] **Step 1: Write the failing test**

In `internal/provider/system_info_data_source_test.go`, add a `pending_restart` assertion to the existing `Check` (after the `server_name` line):

```go
				resource.TestCheckResourceAttrSet("data.jellyfin_system_info.test", "pending_restart"),
```

The full `Check` becomes:

```go
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("data.jellyfin_system_info.test", "id"),
				resource.TestCheckResourceAttrSet("data.jellyfin_system_info.test", "version"),
				resource.TestCheckResourceAttrSet("data.jellyfin_system_info.test", "server_name"),
				resource.TestCheckResourceAttrSet("data.jellyfin_system_info.test", "pending_restart"),
			),
```

- [ ] **Step 2: Run the test to verify it fails**

Start a Jellyfin for acceptance tests and export credentials (replace endpoint/port with your instance):
```bash
JELLYFIN_VERSION=$(grep JELLYFIN_VERSION internal/provider/supported_jellyfin_version.env | cut -d= -f2) \
  docker compose up -d
bash scripts/setup_jellyfin.sh   # prints the exports to set
export JELLYFIN_ENDPOINT=http://localhost:8096 JELLYFIN_USERNAME=admin JELLYFIN_PASSWORD='Admin123!'
TF_ACC=1 go test ./internal/provider/ -run TestAccSystemInfoDataSource -v
```
Expected: FAIL — `pending_restart` is not a supported attribute (`Attribute "pending_restart" not found` / plan error), because it is not yet in the schema.

- [ ] **Step 3: Add the attribute to the model**

In `internal/provider/system_info_data_source.go`, add a field to `SystemInfoDataSourceModel` after `LocalAddress`:

```go
type SystemInfoDataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	ServerName      types.String `tfsdk:"server_name"`
	Version         types.String `tfsdk:"version"`
	OperatingSystem types.String `tfsdk:"operating_system"`
	LocalAddress    types.String `tfsdk:"local_address"`
	PendingRestart  types.Bool   `tfsdk:"pending_restart"`
}
```

- [ ] **Step 4: Add the attribute to the schema**

In the `Schema` method, add a `pending_restart` attribute after the `local_address` block (inside `Attributes: map[string]schema.Attribute{ ... }`):

```go
			"pending_restart": schema.BoolAttribute{
				Description:         "Whether the Jellyfin server has a pending restart (e.g. after a plugin install). True until the server is restarted.",
				MarkdownDescription: "Whether the Jellyfin server has a pending restart (e.g. after a plugin install). True until the server is restarted.",
				Computed:            true,
			},
```

- [ ] **Step 5: Populate it in `Read`**

In the `Read` method, add `PendingRestart` to the struct literal:

```go
	data := SystemInfoDataSourceModel{
		ID:              types.StringValue(info.ID),
		ServerName:      types.StringValue(info.ServerName),
		Version:         types.StringValue(info.Version),
		OperatingSystem: types.StringValue(info.OperatingSystem),
		LocalAddress:    types.StringValue(info.LocalAddress),
		PendingRestart:  types.BoolValue(info.HasPendingRestart),
	}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `TF_ACC=1 go test ./internal/provider/ -run TestAccSystemInfoDataSource -v`
Expected: PASS.

- [ ] **Step 7: Format and commit**

```bash
make fmt
git add internal/provider/system_info_data_source.go internal/provider/system_info_data_source_test.go
git commit -m "feat(system_info): surface pending_restart (HasPendingRestart)"
```

---

### Task 3: `jellyfin_restart` resource + helper unit tests

**Files:**
- Create: `internal/provider/restart_resource.go`
- Create: `internal/provider/restart_resource_unit_test.go`

**Interfaces:**
- Consumes: `(*client.Client).RestartServer` (Task 1), `(*client.Client).GetSystemInfo`, and the package-level `startupStatusDelay` const (`internal/provider/provider.go:31`).
- Produces: type `RestartResource` with `NewRestartResource() resource.Resource`; helpers `func waitForServerReady(ctx context.Context, c *client.Client, timeout time.Duration) error` and `func randomID() (string, error)`. Registered in Task 4.

- [ ] **Step 1: Write the failing unit tests**

Create `internal/provider/restart_resource_unit_test.go`:

```go
// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func TestRandomIDIsUniqueAndHex(t *testing.T) {
	t.Parallel()

	a, err := randomID()
	if err != nil {
		t.Fatalf("randomID() error = %v", err)
	}
	b, err := randomID()
	if err != nil {
		t.Fatalf("randomID() error = %v", err)
	}
	if a == b {
		t.Fatalf("expected distinct ids, got %q twice", a)
	}
	if len(a) != 32 {
		t.Fatalf("expected 32-char hex id, got %q (len %d)", a, len(a))
	}
}

func TestWaitForServerReadyReturnsImmediatelyWhenNotPending(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": "s", "HasPendingRestart": false})
	}))
	defer server.Close()

	c := client.NewClient(server.URL, "k")
	if err := waitForServerReady(context.Background(), c, 10*time.Second); err != nil {
		t.Fatalf("waitForServerReady() error = %v", err)
	}
}

func TestWaitForServerReadyPollsUntilNotPending(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pending := calls.Add(1) < 2 // pending on the first call, clear on the second
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": "s", "HasPendingRestart": pending})
	}))
	defer server.Close()

	c := client.NewClient(server.URL, "k")
	if err := waitForServerReady(context.Background(), c, 10*time.Second); err != nil {
		t.Fatalf("waitForServerReady() error = %v", err)
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("expected at least 2 polls, got %d", got)
	}
}

func TestWaitForServerReadyTimesOut(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": "s", "HasPendingRestart": true})
	}))
	defer server.Close()

	c := client.NewClient(server.URL, "k")
	err := waitForServerReady(context.Background(), c, 500*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/provider/ -run 'TestRandomID|TestWaitForServerReady' -v`
Expected: compile error — `randomID undefined` and `waitForServerReady undefined`.

- [ ] **Step 3: Implement the resource**

Create `internal/provider/restart_resource.go`:

```go
// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

// NewRestartResource creates a new jellyfin_restart resource.
func NewRestartResource() resource.Resource {
	return &RestartResource{}
}

// RestartResource implements an action resource that restarts the Jellyfin
// server. It is modeled on null_resource/terraform_data: the restart fires on
// create and whenever a value in `triggers` changes (which forces replacement).
type RestartResource struct {
	client *client.Client
}

// RestartResourceModel describes the resource data model.
type RestartResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Triggers    types.Map    `tfsdk:"triggers"`
	Timeout     types.Int64  `tfsdk:"timeout"`
	CompletedAt types.String `tfsdk:"completed_at"`
}

func (r *RestartResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_restart"
}

func (r *RestartResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Restarts the Jellyfin server. The restart fires on create and whenever any value in `triggers` changes (which forces replacement). Use it to load newly installed plugins so a following jellyfin_plugin_configuration can run in the same apply.",
		MarkdownDescription: "Restarts the Jellyfin server. The restart fires on create and whenever any value in `triggers` changes (which forces replacement). Use it to load newly installed plugins so a following `jellyfin_plugin_configuration` can run in the same apply.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "Unique identifier for this restart action.",
				MarkdownDescription: "Unique identifier for this restart action.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"triggers": schema.MapAttribute{
				Description:         "Map of arbitrary string values that, when changed, force a new restart. Tie restarts to upstream changes, e.g. `triggers = { plugin_version = jellyfin_plugin.x.version }`.",
				MarkdownDescription: "Map of arbitrary string values that, when changed, force a new restart. Tie restarts to upstream changes, e.g. `triggers = { plugin_version = jellyfin_plugin.x.version }`.",
				ElementType:         types.StringType,
				Optional:            true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"timeout": schema.Int64Attribute{
				Description:         "Maximum number of seconds to wait for the server to come back up after restart. Defaults to 120.",
				MarkdownDescription: "Maximum number of seconds to wait for the server to come back up after restart. Defaults to 120.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(120),
			},
			"completed_at": schema.StringAttribute{
				Description:         "RFC3339 timestamp marking when the server was ready after the restart.",
				MarkdownDescription: "RFC3339 timestamp marking when the server was ready after the restart.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *RestartResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}

	r.client = c
}

func (r *RestartResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RestartResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RestartServer(ctx); err != nil {
		resp.Diagnostics.AddError("Failed to restart Jellyfin server", err.Error())
		return
	}

	timeout := 120 * time.Second
	if !data.Timeout.IsNull() && !data.Timeout.IsUnknown() {
		timeout = time.Duration(data.Timeout.ValueInt64()) * time.Second
	}
	if err := waitForServerReady(ctx, r.client, timeout); err != nil {
		resp.Diagnostics.AddError("Jellyfin server did not become ready after restart", err.Error())
		return
	}

	id, err := randomID()
	if err != nil {
		resp.Diagnostics.AddError("Failed to generate restart resource id", err.Error())
		return
	}
	data.ID = types.StringValue(id)
	data.CompletedAt = types.StringValue(time.Now().UTC().Format(time.RFC3339))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RestartResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// The restart is a point-in-time action; refresh preserves state as-is.
	var data RestartResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RestartResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// `triggers` forces replacement, so Update is never reached.
	resp.Diagnostics.AddError("Update not supported", "jellyfin_restart is replace-only: change a value in triggers to force a new restart.")
}

func (r *RestartResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Nothing to undo on the server; just drop from state.
}

// waitForServerReady polls /System/Info until the server responds and
// HasPendingRestart is false, indicating the restart completed and plugins are
// loaded. Any HTTP/network/decode error (the expected mid-restart states) is
// treated as "not ready" and retried. Reuses startupStatusDelay from provider.go.
func waitForServerReady(ctx context.Context, c *client.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("Jellyfin server did not report ready (HasPendingRestart=false) within %s", timeout)
		}
		if info, err := c.GetSystemInfo(ctx); err == nil && !info.HasPendingRestart {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(startupStatusDelay):
		}
	}
}

// randomID returns a 16-byte hex string for the resource id.
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

- [ ] **Step 4: Run the unit tests to verify they pass**

Run: `go test ./internal/provider/ -run 'TestRandomID|TestWaitForServerReady' -v`
Expected: PASS (3 tests). Note: `TestWaitForServerReadyPollsUntilNotPending` and `TestWaitForServerReadyTimesOut` each take ~1s due to `startupStatusDelay`.

- [ ] **Step 5: Build the whole module to confirm it compiles**

Run: `go build ./...`
Expected: no errors. (`RestartResource` is not yet registered, but unused exported types/methods compile fine in Go.)

- [ ] **Step 6: Format and commit**

```bash
make fmt
git add internal/provider/restart_resource.go internal/provider/restart_resource_unit_test.go
git commit -m "feat(restart): add jellyfin_restart resource and wait/id helpers"
```

---

### Task 4: Register the resource + acceptance test

**Files:**
- Modify: `internal/provider/provider.go` (the `Resources` slice, ~line 249-266)
- Create: `internal/provider/restart_resource_test.go`

**Interfaces:**
- Consumes: `NewRestartResource` (Task 3), the shared acc helpers `testAccPreCheck`, `testAccProtoV6ProviderFactories`, `testAccClient`, `testAccFindInstallablePlugin`, and the `stableRepoURL` const (`internal/provider/provider_test.go`, `plugin_resource_test.go`).
- Produces: `jellyfin_restart` registered with the provider; `TestAccRestartResource`.

- [ ] **Step 1: Write the failing acceptance test**

Create `internal/provider/restart_resource_test.go`:

```go
// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccRestartResource exercises the motivating workflow: install a plugin
// (which sets HasPendingRestart), then restart so the plugin loads. It is
// gated because restarting disrupts any other tests sharing the same Jellyfin
// instance — run it against a disposable Jellyfin in isolation, not the shared
// CI/server instance.
func TestAccRestartResource(t *testing.T) {
	if os.Getenv("JELLYFIN_RESTART_ACC") == "" {
		t.Skip("set JELLYFIN_RESTART_ACC=1 to run the disruptive restart acceptance test; run against a disposable Jellyfin (e.g. the bundled docker-compose) in isolation, not a shared instance")
	}
	testAccPreCheck(t)
	pluginName, pluginVersion := testAccFindInstallablePlugin(t, stableRepoURL)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "jellyfin_plugin" "test" {
  name           = %q
  version        = %q
  repository_url = %q
}

resource "jellyfin_restart" "after_plugin" {
  triggers = {
    plugin_version = jellyfin_plugin.test.version
  }
}
`, pluginName, pluginVersion, stableRepoURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_restart.after_plugin", "id"),
					resource.TestCheckResourceAttrSet("jellyfin_restart.after_plugin", "completed_at"),
					resource.TestCheckResourceAttr("jellyfin_restart.after_plugin", "triggers.plugin_version", pluginVersion),
					func(*terraform.State) error {
						c := testAccClient(t)
						info, err := c.GetSystemInfo(context.Background())
						if err != nil {
							return fmt.Errorf("reading system info after restart: %w", err)
						}
						if info.HasPendingRestart {
							return fmt.Errorf("expected HasPendingRestart=false after restart, got true")
						}
						return nil
					},
				),
			},
		},
	})
}
```

- [ ] **Step 2: Run the test to verify it fails (resource not registered)**

Run: `TF_ACC=1 JELLYFIN_RESTART_ACC=1 JELLYFIN_ENDPOINT=http://localhost:8096 JELLYFIN_USERNAME=admin JELLYFIN_PASSWORD='Admin123!' go test ./internal/provider/ -run TestAccRestartResource -v`
Expected: FAIL — `The resource type "jellyfin_restart" is not registered` (the provider does not yet expose it).

- [ ] **Step 3: Register the resource**

In `internal/provider/provider.go`, add `NewRestartResource,` to the slice returned by `Resources` (after `NewAPIKeyResource,`):

```go
func (p *JellyfinProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewUserResource,
		NewLibraryResource,
		NewPluginRepositoryResource,
		NewPluginResource,
		NewPluginConfigurationResource,
		NewJellyfinSecurityPluginConfigurationResource,
		NewSystemConfigurationResource,
		NewEncodingConfigurationResource,
		NewNetworkingConfigurationResource,
		NewBrandingConfigurationResource,
		NewScheduledTaskResource,
		NewLiveTVConfigurationResource,
		NewMetadataConfigurationResource,
		NewAPIKeyResource,
		NewRestartResource,
	}
}
```

- [ ] **Step 4: Run the acceptance test against a disposable, isolated Jellyfin**

Use a dedicated Jellyfin (not one shared with other tests), e.g. on an alternate host port to avoid clashes:
```bash
JELLYFIN_VERSION=$(grep JELLYFIN_VERSION internal/provider/supported_jellyfin_version.env | cut -d= -f2) docker compose up -d
bash scripts/setup_jellyfin.sh
export JELLYFIN_ENDPOINT=http://localhost:8096 JELLYFIN_USERNAME=admin JELLYFIN_PASSWORD='Admin123!'
TF_ACC=1 JELLYFIN_RESTART_ACC=1 go test ./internal/provider/ -run TestAccRestartResource -v
```
Expected: PASS — the plugin installs, the restart fires, the server comes back with `HasPendingRestart=false`, and `id`/`completed_at` are set. (If `8096` is already in use on your host, point `docker compose` at a different host port and adjust `JELLYFIN_ENDPOINT`.)

- [ ] **Step 5: Run the full unit suite to confirm no regressions**

Run: `make test`
Expected: PASS (unit tests only; acc tests are skipped without `TF_ACC`).

- [ ] **Step 6: Format and commit**

```bash
make fmt
git add internal/provider/provider.go internal/provider/restart_resource_test.go
git commit -m "feat(restart): register jellyfin_restart and add acceptance test"
```

---

### Task 5: Docs (example + generate)

**Files:**
- Create: `examples/resources/jellyfin_restart/resource.tf`
- Generated: `docs/resources/restart.md`, `docs/data-sources/system_info.md`

**Interfaces:**
- Consumes: the registered `jellyfin_restart` resource (Task 4) and the `pending_restart` attribute (Task 2).
- Produces: user-facing docs.

- [ ] **Step 1: Create the example**

Create `examples/resources/jellyfin_restart/resource.tf`:

```hcl
resource "jellyfin_plugin" "example" {
  name           = "Bookshelf"
  repository_url = "https://repo.jellyfin.org/files/plugin/manifest.json"
}

# Restart Jellyfin so the newly installed plugin loads. Any resource that
# configures the plugin should depend on this restart so it runs against the
# loaded plugin in the same apply:
#
#   resource "jellyfin_plugin_configuration" "example" {
#     plugin_id          = jellyfin_plugin.example.id
#     configuration_json = jsonencode({ /* plugin-specific */ })
#     depends_on         = [jellyfin_restart.example]
#   }
resource "jellyfin_restart" "example" {
  triggers = {
    plugin_version = jellyfin_plugin.example.version
  }
}
```

(No `import.sh` — `jellyfin_restart` does not support import.)

- [ ] **Step 2: Generate the docs**

Run: `make generate`
Expected: `tfplugindocs` writes `docs/resources/restart.md` and updates `docs/data-sources/system_info.md` to include `pending_restart`. (`make generate` also runs `copywrite` headers and `terraform fmt` on `examples/`.)

- [ ] **Step 3: Verify the generated docs**

Run: `cat docs/resources/restart.md`
Expected: a `# jellyfin_restart (Resource)` page with the example, a schema table listing `id`, `triggers`, `timeout`, `completed_at`, and no `## Import` section.

Run: `grep pending_restart docs/data-sources/system_info.md`
Expected: a match (the new computed attribute documented).

- [ ] **Step 4: Format and commit**

```bash
make fmt
git add examples/resources/jellyfin_restart/ docs/resources/restart.md docs/data-sources/system_info.md
git commit -m "docs: add jellyfin_restart resource docs and pending_restart"
```

---

## Self-Review

**Spec coverage:**
- `pending_restart` on `jellyfin_system_info` → Task 2.
- `RestartServer` client method → Task 1.
- `jellyfin_restart` resource (trigger-based, blocking, create-restarts, Read/Update/Delete semantics) → Task 3.
- `waitForServerReady` using `/System/Info` + `HasPendingRestart==false`, tolerating mid-restart errors, reusing `startupStatusDelay`, bounded by `timeout` (default 120) → Task 3.
- `triggers` forces replace (`mapplanmodifier.RequiresReplace()`) → Task 3.
- `timeout` default 120 (`int64default.StaticInt64(120)`) → Task 3.
- Registration → Task 4.
- Acceptance test exercising install → restart → server-ready, gated for isolation → Task 4.
- Unit tests for the restart/wait logic → Tasks 1, 3.
- Docs → Task 5.
- Spec open item (framework plan-modifier / default-value APIs): resolved — `mapplanmodifier.RequiresReplace()` and `int64default.StaticInt64(120)` match the repo's existing `*planmodifier`/`*default` usage.

**Placeholder scan:** none — every code step contains the full code; every command has expected output.

**Type consistency:** `RestartResourceModel` fields (`ID`, `Triggers`, `Timeout`, `CompletedAt`) match the schema keys (`id`, `triggers`, `timeout`, `completed_at`) and are used consistently in `Create`/`Read`. `waitForServerReady(ctx, *client.Client, time.Duration) error` and `randomID() (string, error)` match their call sites in `Create` and their unit tests. `RestartServer(ctx) error` matches its call site and unit test.
