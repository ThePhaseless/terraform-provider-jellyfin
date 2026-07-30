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
