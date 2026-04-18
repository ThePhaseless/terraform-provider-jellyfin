// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccInitialSetupResource exercises the initial setup resource. It is skipped unless the
// JELLYFIN_INITIAL_SETUP_ENDPOINT environment variable points at a server whose startup wizard
// has not been completed yet, so it will not fail against a normal acceptance-test server.
func TestAccInitialSetupResource(t *testing.T) {
	endpoint := os.Getenv("JELLYFIN_INITIAL_SETUP_ENDPOINT")
	if endpoint == "" {
		t.Skip("JELLYFIN_INITIAL_SETUP_ENDPOINT must be set to run the initial setup test")
	}

	c := client.NewClient(endpoint, "")
	info, err := c.GetPublicSystemInfo()
	if err != nil {
		t.Fatalf("failed to query public system info: %v", err)
	}
	if info.StartupWizardCompleted {
		t.Skip("startup wizard already completed on the target server")
	}

	t.Setenv("JELLYFIN_ENDPOINT", endpoint)
	t.Setenv("JELLYFIN_API_KEY", "")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_initial_setup" "test" {
  username = "admin"
  password = "ChangeMe!1"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_initial_setup.test", "username", "admin"),
					resource.TestCheckResourceAttr("jellyfin_initial_setup.test", "ui_culture", "en-US"),
				),
			},
		},
	})
}
