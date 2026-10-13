// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

var v050 = map[string]resource.ExternalProvider{
	"jellyfin": {Source: providerSource, VersionConstraint: "0.5.0"},
}

// testAccPluginUpdatesDefaultTriggers are the triggers Jellyfin gives its
// PluginUpdates task, as testAccRestorePluginUpdateTriggers sets them.
const testAccPluginUpdatesDefaultTriggers = `[
    { type = "StartupTrigger" },
    { type = "IntervalTrigger", interval_ticks = 864000000000 },
  ]`

// testAccPluginUpdatesConfig manages the PluginUpdates task with triggers
// and pins the named plugin at version. taskExtra and pluginExtra are added
// to the body of each resource, such as a depends_on.
func testAccPluginUpdatesConfig(triggers, taskExtra, name, version, pluginExtra string) string {
	return fmt.Sprintf(`
resource "jellyfin_scheduled_task" "plugin_updates" {
  key      = "PluginUpdates"
  triggers = %s
  %s
}

resource "jellyfin_plugin" "test" {
  name           = %q
  version        = %q
  repository_url = %q
  %s
}
`, triggers, taskExtra, name, version, stableRepoURL, pluginExtra)
}

const (
	testAccTaskDependsOnPlugin = "depends_on = [jellyfin_plugin.test]"
	testAccPluginDependsOnTask = "depends_on = [jellyfin_scheduled_task.plugin_updates]"
)

// TestAccPluginResourceTurningUpdatesOffNeedsDependsOn turns automatic plugin
// updates off while the pinned version is older than the one the task
// installed, which changes the plugin's plan from keeping it to reinstalling
// the pinned version. A task that depends on the plugin is planned after it,
// so the plan fails; with depends_on the other way round it plans the
// reinstall, and one apply removes the triggers and reinstalls the plugin.
func TestAccPluginResourceTurningUpdatesOffNeedsDependsOn(t *testing.T) {
	pkg := testAccFindUninstalledPackage(t, stableRepoURL, 2)
	older, newer := pkg.Versions[1].Version, pkg.Versions[0].Version
	testAccRestorePluginUpdateTriggers(t)
	on := testAccPluginUpdatesConfig(testAccPluginUpdatesDefaultTriggers, testAccTaskDependsOnPlugin, pkg.Name, older, "")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPluginNotListed(t, pkg.Name),
		Steps: []resource.TestStep{
			{
				Config: on,
				Check:  resource.TestCheckResourceAttr("jellyfin_plugin.test", "installed_version", older),
			},
			{
				PreConfig: func() { testAccUpdatePluginAsTheTaskWould(t, pkg.Name, older, newer) },
				Config:    on,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr("jellyfin_plugin.test", "installed_version", newer),
			},
			{
				Config:      testAccPluginUpdatesConfig("[]", testAccTaskDependsOnPlugin, pkg.Name, older, ""),
				ExpectError: regexp.MustCompile(`Update Plugins schedule planned after the plugins it affects`),
			},
			{
				Config: testAccPluginUpdatesConfig("[]", "", pkg.Name, older, testAccPluginDependsOnTask),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("jellyfin_scheduled_task.plugin_updates", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("jellyfin_plugin.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.plugin_updates", "triggers.#", "0"),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "version", older),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "installed_version", older),
					testAccCheckPluginListedOnlyAt(t, pkg.Name, older),
				),
			},
		},
	})
}

// TestAccPluginResourceUnknownUpdateScheduleCountsAsOn plans triggers unknown
// until apply for a plugin that depends on the task and that the task
// updated past its pinned version: an unknown schedule counts as on, so the
// plugin is kept, and once the apply sets the triggers on, the next plan is
// empty.
func TestAccPluginResourceUnknownUpdateScheduleCountsAsOn(t *testing.T) {
	pkg := testAccFindUninstalledPackage(t, stableRepoURL, 2)
	older, newer := pkg.Versions[1].Version, pkg.Versions[0].Version
	testAccRestorePluginUpdateTriggers(t)
	known := testAccPluginUpdatesConfig(testAccPluginUpdatesDefaultTriggers, "", pkg.Name, older, testAccPluginDependsOnTask)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPluginNotListed(t, pkg.Name),
		Steps: []resource.TestStep{
			{
				Config: known,
			},
			{
				PreConfig: func() { testAccUpdatePluginAsTheTaskWould(t, pkg.Name, older, newer) },
				Config:    known,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: `
resource "terraform_data" "triggers" {
  input = ` + testAccPluginUpdatesDefaultTriggers + `
}
` + testAccPluginUpdatesConfig("terraform_data.triggers.output", "", pkg.Name, older, testAccPluginDependsOnTask),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue("jellyfin_scheduled_task.plugin_updates", tfjsonpath.New("triggers")),
						plancheck.ExpectResourceAction("jellyfin_plugin.test", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_scheduled_task.plugin_updates", "triggers.#", "2"),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "version", older),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "installed_version", newer),
					testAccCheckPluginListedOnlyAt(t, pkg.Name, newer),
				),
			},
		},
	})
}

// TestAccPluginResourceUpgradeFromV050 applies a pinned plugin and the
// PluginUpdates task with 0.5.0, whose state holds no private state, and then
// lets the task update the plugin: this provider counts the version in state
// as pinned and keeps the update, so it plans no change.
func TestAccPluginResourceUpgradeFromV050(t *testing.T) {
	pkg := testAccFindUninstalledPackage(t, stableRepoURL, 2)
	older, newer := pkg.Versions[1].Version, pkg.Versions[0].Version
	testAccRestorePluginUpdateTriggers(t)
	config := testAccPluginUpdatesConfig(testAccPluginUpdatesDefaultTriggers, "", pkg.Name, older, testAccPluginDependsOnTask)
	namespace, _, _ := strings.Cut(providerSource, "/")
	t.Setenv(resource.EnvTfAccProviderNamespace, namespace)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		CheckDestroy: testAccCheckPluginNotListed(t, pkg.Name),
		Steps: []resource.TestStep{
			{
				ExternalProviders: v050,
				Config:            config,
				Check:             resource.TestCheckResourceAttr("jellyfin_plugin.test", "installed_version", older),
			},
			{
				PreConfig:                func() { testAccUpdatePluginAsTheTaskWould(t, pkg.Name, older, newer) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "version", older),
					resource.TestCheckResourceAttr("jellyfin_plugin.test", "installed_version", newer),
				),
			},
		},
	})
}
