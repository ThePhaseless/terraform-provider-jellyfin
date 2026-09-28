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
			testAccAttrsStep("jellyfin_networking_configuration", testAccNetworkingAttrs(false)),
			{
				ResourceName:      "jellyfin_networking_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "networking",
			},
			testAccAttrsStep("jellyfin_networking_configuration", testAccNetworkingAttrs(true)),
		},
	})
}

func testAccNetworkingAttrs(enableIPv6 bool) []testAccAttr {
	return []testAccAttr{
		{"base_url", ""},
		{"enable_https", false},
		{"require_https", false},
		{"internal_http_port", 8096},
		{"internal_https_port", 8920},
		{"public_http_port", 8096},
		{"public_https_port", 8920},
		{"auto_discovery", true},
		{"enable_ipv4", true},
		{"enable_ipv6", enableIPv6},
		{"enable_remote_access", true},
		{"known_proxies", []string{}},
		{"local_network_subnets", []string{}},
		{"local_network_addresses", []string{}},
		{"remote_ip_filter", []string{}},
		{"is_remote_ip_filter_blacklist", false},
		{"certificate_path", ""},
		{"certificate_password", ""},
		{"enable_upnp", false},
		{"ignore_virtual_interfaces", true},
		{"virtual_interface_names", []string{"veth"}},
		{"enable_published_server_uri_by_request", false},
		{"published_server_uri_by_subnet", []string{}},
	}
}
