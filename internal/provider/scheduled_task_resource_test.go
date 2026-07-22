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
  task_id = "7738148ffcd07979c7ceb148e06b3aed"

  triggers = [
    {
      type           = "IntervalTrigger"
      interval_ticks = 432000000000
    }
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.test", "id", "7738148ffcd07979c7ceb148e06b3aed"),
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.test", "triggers.#", "1"),
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
  task_id = "7738148ffcd07979c7ceb148e06b3aed"

  triggers = [
    {
      type           = "IntervalTrigger"
      interval_ticks = 864000000000
    },
    {
      type             = "DailyTrigger"
      time_of_day_ticks = 36000000000
    }
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.test", "triggers.#", "2"),
				),
			},
		},
	})
}
