// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccServerRestartResource exercises the server restart resource. It is gated on
// JELLYFIN_RESTART_TEST=1 because it actually restarts the test Jellyfin server.
func TestAccServerRestartResource(t *testing.T) {
	if os.Getenv("JELLYFIN_RESTART_TEST") != "1" {
		t.Skip("JELLYFIN_RESTART_TEST must be set to 1 to run the server restart test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_server_restart" "test" {
  triggers = {
    reason = "test-restart"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_server_restart.test", "id"),
					resource.TestCheckResourceAttrSet("jellyfin_server_restart.test", "last_restarted"),
				),
			},
		},
	})
}
