// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccPluginConfigurationResource(t *testing.T) {
	const (
		name = "jellyfin_plugin_configuration.test"
		// MusicBrainz is built in, so always available.
		pluginID = "8c95c4d2e50c4fb0a4f36c06ff0f9a1a"
		dashedID = "8c95c4d2-e50c-4fb0-a4f3-6c06ff0f9a1a"
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMusicBrainzConfigurationConfig(pluginID, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "plugin_id", pluginID),
					resource.TestCheckResourceAttrSet(name, "configuration_json"),
				),
			},
			{
				ResourceName:                         name,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "plugin_id",
				ImportStateId:                        pluginID,
				ImportStateVerifyIgnore:              []string{"configuration_json"},
			},
			// Importing by the dashed spelling of the GUID the configuration
			// holds dash-free must not plan a replacement.
			{
				ResourceName:    name,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				ImportStateId:   dashedID,
			},
			// Nor does switching the configuration to that spelling.
			{
				Config: testAccMusicBrainzConfigurationConfig(dashedID, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: resource.TestCheckResourceAttr(name, "plugin_id", pluginID),
			},
			{
				Config: testAccMusicBrainzConfigurationConfig(pluginID, 2),
				Check:  resource.TestCheckResourceAttrSet(name, "configuration_json"),
			},
		},
	})
}

func testAccMusicBrainzConfigurationConfig(pluginID string, rateLimit int) string {
	return fmt.Sprintf(`
resource "jellyfin_plugin_configuration" "test" {
  plugin_id          = %q
  configuration_json = jsonencode({
    Server            = "https://musicbrainz.org"
    RateLimit         = %d
    ReplaceArtistName = false
  })
}
`, pluginID, rateLimit)
}

func TestAccPluginConfigurationPartialOwnership(t *testing.T) {
	testAccPreCheck(t)
	const id = "8c95c4d2e50c4fb0a4f36c06ff0f9a1a"
	original, err := testAccClient(t).GetPluginConfiguration(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testAccClient(t).UpdatePluginConfiguration(context.WithoutCancel(t.Context()), id, original); err != nil {
			t.Error(err)
		}
	})
	setRemote := func(raw string) {
		t.Helper()
		if err := testAccClient(t).UpdatePluginConfiguration(t.Context(), id, raw); err != nil {
			t.Fatal(err)
		}
	}
	check := func(rate float64, server string) resource.TestCheckFunc {
		return func(_ *terraform.State) error {
			raw, err := testAccClient(t).GetPluginConfiguration(t.Context(), id)
			if err != nil {
				return err
			}
			var values map[string]any
			if err := json.Unmarshal([]byte(raw), &values); err != nil {
				return err
			}
			if values["RateLimit"] != rate || values["Server"] != server {
				return fmt.Errorf("unexpected configuration: %s", raw)
			}
			return nil
		}
	}
	config := func(raw string) string {
		return fmt.Sprintf(`resource "jellyfin_plugin_configuration" "test" {
 plugin_id = %q
 configuration_json = %q
}`, id, raw)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{PreConfig: func() { setRemote(`{"Server":"https://outside.example","RateLimit":7,"ReplaceArtistName":false}`) }, Config: config(`{"RateLimit":2}`), Check: check(2, "https://outside.example")},
			{PreConfig: func() { setRemote(`{"Server":"https://fresh.example","RateLimit":2,"ReplaceArtistName":false}`) }, Config: config(`{"RateLimit":3}`), Check: check(3, "https://fresh.example")},
			{Config: config(`{}`), Check: check(3, "https://fresh.example")},
			{Config: config(`{}`), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
		},
	})
}
