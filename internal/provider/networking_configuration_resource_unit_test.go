// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"
)

func TestUnitNetworkingConfigurationRoundTrip(t *testing.T) {
	ctx := context.Background()
	b, err := networkingWire()
	if err != nil {
		t.Fatal(err)
	}
	fixture := `{"BaseUrl":"BaseURL","EnableHttps":true,"RequireHttps":true,"CertificatePath":"CertificatePath","CertificatePassword":"CertificatePassword","InternalHttpPort":8096,"InternalHttpsPort":8920,"PublicHttpPort":8096,"PublicHttpsPort":8920,"AutoDiscovery":true,"EnableUPnP":false,"EnableIPv4":true,"EnableIPv6":false,"EnableRemoteAccess":true,"LocalNetworkSubnets":["10.0.0.0/8"],"LocalNetworkAddresses":["localhost"],"KnownProxies":["10.244.0.0/16"],"IgnoreVirtualInterfaces":true,"VirtualInterfaceNames":["veth"],"EnablePublishedServerUriByRequest":true,"PublishedServerUriBySubnet":["all=https://example.com"],"RemoteIPFilter":[],"IsRemoteIPFilterBlacklist":false}`

	data := readWire[NetworkingConfigurationResourceModel](t, b, fixture)
	base := map[string]json.RawMessage{}
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("write: %v", d)
	}
	checkSameJSON(t, base, fixture)
}
