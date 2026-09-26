// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
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
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.test", "triggers.0.interval_ticks", "432000000000"),
					resource.TestCheckNoResourceAttr("jellyfin_scheduled_task.test", "triggers.0.time_of_day_ticks"),
					resource.TestCheckNoResourceAttr("jellyfin_scheduled_task.test", "triggers.0.day_of_week"),
					resource.TestCheckNoResourceAttr("jellyfin_scheduled_task.test", "triggers.0.max_runtime_ticks"),
				),
			},
			{
				Config: `
resource "jellyfin_scheduled_task" "test" {
  task_id = "7738148ffcd07979c7ceb148e06b3aed"

  triggers = [
    {
      type              = "WeeklyTrigger"
      time_of_day_ticks = 36000000000
    }
  ]
}
`,
				ExpectError: regexp.MustCompile(`day_of_week is required when type is "WeeklyTrigger"`),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_scheduled_task.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update: replace the interval trigger at index 0 with a weekly one and add daily and interval triggers.
			{
				Config: `
resource "jellyfin_scheduled_task" "test" {
  task_id = "7738148ffcd07979c7ceb148e06b3aed"

  triggers = [
    {
      type              = "WeeklyTrigger"
      day_of_week       = "Tuesday"
      time_of_day_ticks = 36000000000
    },
    {
      type              = "DailyTrigger"
      time_of_day_ticks = 72000000000
      max_runtime_ticks = 144000000000
    },
    {
      type           = "IntervalTrigger"
      interval_ticks = 864000000000
    }
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.test", "triggers.#", "3"),
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.test", "triggers.0.type", "WeeklyTrigger"),
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.test", "triggers.0.day_of_week", "Tuesday"),
					resource.TestCheckNoResourceAttr("jellyfin_scheduled_task.test", "triggers.0.interval_ticks"),
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.test", "triggers.1.max_runtime_ticks", "144000000000"),
					resource.TestCheckNoResourceAttr("jellyfin_scheduled_task.test", "triggers.1.day_of_week"),
					resource.TestCheckNoResourceAttr("jellyfin_scheduled_task.test", "triggers.2.time_of_day_ticks"),
				),
			},
			{
				ResourceName:      "jellyfin_scheduled_task.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
