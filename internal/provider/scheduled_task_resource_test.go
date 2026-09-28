// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
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
			{
				Config: `
resource "jellyfin_scheduled_task" "test" {
  task_id = "7738148ffcd07979c7ceb148e06b3aed"

  triggers = [
    {
      type              = "DailyTrigger"
      time_of_day_ticks = 864000000000
    }
  ]
}
`,
				ExpectError: regexp.MustCompile(`must be between 0 and\s+863999999999`),
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

func TestAccScheduledTaskResourceUppercaseTaskID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_scheduled_task" "test" {
  task_id = "7738148FFCD07979C7CEB148E06B3AED"

  triggers = [
    {
      type              = "DailyTrigger"
      time_of_day_ticks = 72000000000
    }
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.test", "id", "7738148FFCD07979C7CEB148E06B3AED"),
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.test", "task_id", "7738148FFCD07979C7CEB148E06B3AED"),
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

func testAccScheduledTaskConfig(selector string) string {
	return testAccScheduledTaskIntervalConfig(selector, 432000000000)
}

func testAccScheduledTaskIntervalConfig(selector string, intervalTicks int64) string {
	return fmt.Sprintf(`
resource "jellyfin_scheduled_task" "test" {
  %s

  triggers = [
    {
      type           = "IntervalTrigger"
      interval_ticks = %d
    }
  ]
}
`, selector, intervalTicks)
}

func TestAccScheduledTaskResourceKey(t *testing.T) {
	const name = "jellyfin_scheduled_task.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccScheduledTaskConfig(`key = "RefreshLibrary"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectKnownValue(name, tfjsonpath.New("task_id"), knownvalue.StringExact(scanMediaLibraryID)),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", scanMediaLibraryID),
					resource.TestCheckResourceAttr(name, "task_id", scanMediaLibraryID),
					resource.TestCheckResourceAttr(name, "key", "RefreshLibrary"),
					resource.TestCheckResourceAttr(name, "triggers.0.interval_ticks", "432000000000"),
				),
			},
			{
				ResourceName:      name,
				ImportState:       true,
				ImportStateId:     "RefreshLibrary",
				ImportStateVerify: true,
			},
			{
				ResourceName:      name,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:    name,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				ImportStateId:   "RefreshLibrary",
				GenerateConfig:  true,
			},
			{
				ResourceName:    name,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				ImportStateId:   scanMediaLibraryID,
				GenerateConfig:  true,
			},
			{
				Config: testAccScheduledTaskIntervalConfig(`key = "RefreshLibrary"`, 864000000000),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue(name, tfjsonpath.New("task_id"), knownvalue.StringExact(scanMediaLibraryID)),
					},
				},
				Check: resource.TestCheckResourceAttr(name, "triggers.0.interval_ticks", "864000000000"),
			},
			{
				Config: testAccScheduledTaskIntervalConfig(`task_id = "`+scanMediaLibraryID+`"`, 864000000000),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: testAccScheduledTaskConfig(`task_id = "` + scanMediaLibraryID + `"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue(name, tfjsonpath.New("key"), knownvalue.StringExact("RefreshLibrary")),
					},
				},
			},
			{
				Config: testAccScheduledTaskConfig(`key = "CleanLogFiles"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(name, plancheck.ResourceActionReplace),
						plancheck.ExpectKnownValue(name, tfjsonpath.New("task_id"), knownvalue.StringExact(cleanLogFilesID)),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", cleanLogFilesID),
					resource.TestCheckResourceAttr(name, "key", "CleanLogFiles"),
				),
			},
		},
	})
}

func testAccScheduledTaskFromDataConfig(input, selector string) string {
	return fmt.Sprintf(`
resource "terraform_data" "selector" {
  input = %q
}
`, input) + testAccScheduledTaskConfig(selector)
}

func TestAccScheduledTaskResourceKeyUnknownAtPlan(t *testing.T) {
	const name = "jellyfin_scheduled_task.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccScheduledTaskFromDataConfig("RefreshLibrary", "key = terraform_data.selector.output"),
				Check:  resource.TestCheckResourceAttr(name, "task_id", scanMediaLibraryID),
			},
			{
				Config: testAccScheduledTaskFromDataConfig("CleanLogFiles", "key = terraform_data.selector.output"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionReplace)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", cleanLogFilesID),
					resource.TestCheckResourceAttr(name, "task_id", cleanLogFilesID),
					resource.TestCheckResourceAttr(name, "key", "CleanLogFiles"),
				),
			},
		},
	})
}

func TestAccScheduledTaskResourceKeyWithTaskIDUnknownAtPlan(t *testing.T) {
	const name = "jellyfin_scheduled_task.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccScheduledTaskFromDataConfig(cleanLogFilesID, `key = "RefreshLibrary"`+"\n  task_id = terraform_data.selector.output"),
				ExpectError: regexp.MustCompile(`task_id\s+"` + cleanLogFilesID + `"\s+names\s+the\s+task\s+with\s+the\s+key\s+"CleanLogFiles"`),
			},
			{
				Config: testAccScheduledTaskFromDataConfig(scanMediaLibraryID, `key = "RefreshLibrary"`+"\n  task_id = terraform_data.selector.output"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", scanMediaLibraryID),
					resource.TestCheckResourceAttr(name, "key", "RefreshLibrary"),
				),
			},
		},
	})
}

func TestAccScheduledTaskResourceRejectsSelectorsAtPlan(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccScheduledTaskConfig(`key = "RefreshLibrary"` + "\n  task_id = \"" + cleanLogFilesID + `"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`task_id\s+"` + cleanLogFilesID + `"\s+names\s+the\s+task\s+with\s+the\s+key\s+"CleanLogFiles"`),
			},
			{
				Config:      testAccScheduledTaskConfig(""),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Set key, such as "RefreshLibrary", or task_id`),
			},
			{
				Config:      testAccScheduledTaskConfig(`key = "refreshlibrary"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`No scheduled task has the key "refreshlibrary"(.|\n)*RefreshLibrary`),
			},
		},
	})
}

// A state saved before the key attribute existed holds no key until the
// refresh reads one.
func TestAccScheduledTaskResourceKeyForTaskIDFromV038(t *testing.T) {
	testAccPreCheck(t)
	testAccPutBackServerConfiguration(t, scanMediaLibraryID)
	namespace, _, _ := strings.Cut(providerSource, "/")
	t.Setenv(resource.EnvTfAccProviderNamespace, namespace)
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ExternalProviders: v038,
				Config:            testAccScheduledTaskConfig(`task_id = "` + scanMediaLibraryID + `"`),
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccScheduledTaskConfig(`key = "RefreshLibrary"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
