// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccScheduledTaskResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create: configure scan library task to run every 12 hours.
			{
				Config: `
resource "jellyfin_scheduled_task" "test" {
  id = "7738148ffcd07979c7ceb148e06b3aed"

  triggers_json = jsonencode([
    {
      Type          = "IntervalTrigger"
      IntervalTicks = 432000000000
    }
  ])
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.test", "id", "7738148ffcd07979c7ceb148e06b3aed"),
					resource.TestCheckResourceAttrSet("jellyfin_scheduled_task.test", "triggers_json"),
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_scheduled_task.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update: add a daily trigger.
			{
				Config: `
resource "jellyfin_scheduled_task" "test" {
  id = "7738148ffcd07979c7ceb148e06b3aed"

  triggers_json = jsonencode([
    {
      Type          = "IntervalTrigger"
      IntervalTicks = 864000000000
    },
    {
      Type           = "DailyTrigger"
      TimeOfDayTicks = 36000000000
    }
  ])
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_scheduled_task.test", "triggers_json"),
				),
			},
		},
	})
}
