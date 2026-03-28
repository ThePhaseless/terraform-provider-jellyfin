// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccNetworkingConfigurationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read.
			{
				Config: testAccNetworkingConfigurationResourceConfig(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_networking_configuration.test", "configuration_json"),
				),
			},
			// Update.
			{
				Config: testAccNetworkingConfigurationResourceConfig(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_networking_configuration.test", "configuration_json"),
				),
			},
		},
	})
}

func testAccNetworkingConfigurationResourceConfig(enableIPv6 bool) string {
	return `
resource "jellyfin_networking_configuration" "test" {
  configuration_json = jsonencode({
    BaseUrl            = ""
    EnableHttps        = false
    RequireHttps       = false
    InternalHttpPort   = 8096
    InternalHttpsPort  = 8920
    PublicHttpPort     = 8096
    PublicHttpsPort    = 8920
    AutoDiscovery      = true
    EnableIPv4         = true
    EnableIPv6         = ` + boolToString(enableIPv6) + `
    EnableRemoteAccess = true
    KnownProxies       = []
    LocalNetworkSubnets    = []
    LocalNetworkAddresses  = []
    RemoteIPFilter         = []
    IsRemoteIPFilterBlacklist = false
    CertificatePath    = ""
    CertificatePassword = ""
    EnableUPnP         = false
    IgnoreVirtualInterfaces = true
    VirtualInterfaceNames   = ["veth"]
    EnablePublishedServerUriByRequest = false
    PublishedServerUriBySubnet = []
  })
}
`
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
