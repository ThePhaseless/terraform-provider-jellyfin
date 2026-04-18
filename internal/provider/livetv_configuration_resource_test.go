// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccLiveTVConfigurationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read.
			{
				Config: `
resource "jellyfin_livetv_configuration" "test" {
  configuration_json = jsonencode({
    EnableRecordingSubfolders                = false
    EnableOriginalAudioWithEncodedRecordings = false
    PrePaddingSeconds                        = 0
    PostPaddingSeconds                       = 0
    SaveRecordingNFO                         = true
    SaveRecordingImages                      = true
    RecordingPostProcessorArguments          = "\"{path}\""
    TunerHosts                               = []
    ListingProviders                         = []
    MediaLocationsCreated                    = []
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_livetv_configuration.test", "configuration_json"),
				),
			},
			// ImportState.
			{
				ResourceName:            "jellyfin_livetv_configuration.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateId:           "livetv",
				ImportStateVerifyIgnore: []string{"configuration_json"},
			},
			// Update.
			{
				Config: `
resource "jellyfin_livetv_configuration" "test" {
  configuration_json = jsonencode({
    EnableRecordingSubfolders                = true
    EnableOriginalAudioWithEncodedRecordings = false
    PrePaddingSeconds                        = 120
    PostPaddingSeconds                       = 120
    SaveRecordingNFO                         = true
    SaveRecordingImages                      = true
    RecordingPostProcessorArguments          = "\"{path}\""
    TunerHosts                               = []
    ListingProviders                         = []
    MediaLocationsCreated                    = []
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_livetv_configuration.test", "configuration_json"),
				),
			},
		},
	})
}
