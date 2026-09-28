// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestUnitNetworkingConfigurationRoundTrip(t *testing.T) {
	fixture := `{"BaseUrl":"BaseURL","EnableHttps":true,"RequireHttps":true,"CertificatePath":"CertificatePath","CertificatePassword":"CertificatePassword","InternalHttpPort":8096,"InternalHttpsPort":8920,"PublicHttpPort":8096,"PublicHttpsPort":8920,"AutoDiscovery":true,"EnableUPnP":false,"EnableIPv4":true,"EnableIPv6":false,"EnableRemoteAccess":true,"LocalNetworkSubnets":["10.0.0.0/8"],"LocalNetworkAddresses":["localhost"],"KnownProxies":["10.244.0.0/16"],"IgnoreVirtualInterfaces":true,"VirtualInterfaceNames":["veth"],"EnablePublishedServerUriByRequest":true,"PublishedServerUriBySubnet":["all=https://example.com"],"RemoteIPFilter":[],"IsRemoteIPFilterBlacklist":false}`
	checkRoundTrip[NetworkingConfigurationResourceModel](t, mustWire(t, networkingWire), fixture)
}

// Jellyfin stores a base URL with a leading / and without a trailing one.
func TestUnitNetworkingBaseURLTakesOnlyWhatJellyfinStores(t *testing.T) {
	a, ok := schemaOf(&NetworkingConfigurationResource{}).Attributes["base_url"].(schema.StringAttribute)
	if !ok {
		t.Fatal("base_url is not a string attribute")
	}
	testUnitAssertStringValidation(t, a, map[string]bool{
		"":            false,
		"/jellyfin":   false,
		"/a/b":        false,
		"jellyfin":    true,
		"/jellyfin/":  true,
		"/":           true,
		" ":           true,
		"/jelly fin ": false,
	})
}
