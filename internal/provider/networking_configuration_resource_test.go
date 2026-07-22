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
				Config: `
resource "jellyfin_networking_configuration" "test" {
  base_url = ""
  enable_https = false
  require_https = false
  internal_http_port = 8096
  internal_https_port = 8920
  public_http_port = 8096
  public_https_port = 8920
  auto_discovery = true
  enable_ipv4 = true
  enable_ipv6 = false
  enable_remote_access = true
  known_proxies = []
  local_network_subnets = []
  local_network_addresses = []
  remote_ip_filter = []
  is_remote_ip_filter_blacklist = false
  certificate_path = ""
  certificate_password = ""
  enable_upnp = false
  ignore_virtual_interfaces = true
  virtual_interface_names = ["veth"]
  enable_published_server_uri_by_request = false
  published_server_uri_by_subnet = []
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "base_url", ""),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "enable_https", "false"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "require_https", "false"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "internal_http_port", "8096"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "internal_https_port", "8920"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "public_http_port", "8096"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "public_https_port", "8920"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "auto_discovery", "true"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "enable_ipv4", "true"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "enable_ipv6", "false"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "enable_remote_access", "true"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "known_proxies.#", "0"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "local_network_subnets.#", "0"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "local_network_addresses.#", "0"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "remote_ip_filter.#", "0"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "is_remote_ip_filter_blacklist", "false"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "certificate_path", ""),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "certificate_password", ""),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "enable_upnp", "false"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "ignore_virtual_interfaces", "true"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "virtual_interface_names.#", "1"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "enable_published_server_uri_by_request", "false"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "published_server_uri_by_subnet.#", "0"),
				),
			},
			// ImportState.
			{
				ResourceName:      "jellyfin_networking_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "networking",
			},
			// Update.
			{
				Config: `
resource "jellyfin_networking_configuration" "test" {
  base_url = ""
  enable_https = false
  require_https = false
  internal_http_port = 8096
  internal_https_port = 8920
  public_http_port = 8096
  public_https_port = 8920
  auto_discovery = true
  enable_ipv4 = true
  enable_ipv6 = true
  enable_remote_access = true
  known_proxies = []
  local_network_subnets = []
  local_network_addresses = []
  remote_ip_filter = []
  is_remote_ip_filter_blacklist = false
  certificate_path = ""
  certificate_password = ""
  enable_upnp = false
  ignore_virtual_interfaces = true
  virtual_interface_names = ["veth"]
  enable_published_server_uri_by_request = false
  published_server_uri_by_subnet = []
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "base_url", ""),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "enable_https", "false"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "require_https", "false"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "internal_http_port", "8096"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "internal_https_port", "8920"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "public_http_port", "8096"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "public_https_port", "8920"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "auto_discovery", "true"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "enable_ipv4", "true"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "enable_ipv6", "true"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "enable_remote_access", "true"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "known_proxies.#", "0"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "local_network_subnets.#", "0"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "local_network_addresses.#", "0"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "remote_ip_filter.#", "0"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "is_remote_ip_filter_blacklist", "false"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "certificate_path", ""),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "certificate_password", ""),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "enable_upnp", "false"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "ignore_virtual_interfaces", "true"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "virtual_interface_names.#", "1"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "enable_published_server_uri_by_request", "false"),
				resource.TestCheckResourceAttr("jellyfin_networking_configuration.test", "published_server_uri_by_subnet.#", "0"),
				),
			},
		},
	})
}
