// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"
)

func TestUnitNetworkingConfigurationOverlay(t *testing.T) {
	ctx := context.Background()
	fixture := `{"BaseURL":"BaseURL","EnableHTTPS":true,"RequireHTTPS":true,"CertificatePath":"CertificatePath","CertificatePassword":"CertificatePassword","InternalHTTPPort":8096,"InternalHTTPSPort":8920,"PublicHTTPPort":8096,"PublicHTTPSPort":8920,"AutoDiscovery":true,"EnableUPnP":false,"EnableIPv4":true,"EnableIPv6":false,"EnableRemoteAccess":true,"LocalNetworkSubnets":["10.0.0.0/8"],"LocalNetworkAddresses":["localhost"],"KnownProxies":["10.244.0.0/16"],"IgnoreVirtualInterfaces":true,"VirtualInterfaceNames":["veth"],"EnablePublishedServerURIByRequest":true,"PublishedServerURIBySubnet":["all=https://example.com"],"RemoteIPFilter":[],"IsRemoteIPFilterBlacklist":false}`

	var data NetworkingConfigurationResourceModel
	flattenNetworkingConfiguration(ctx, fixture, &data, nil)

	base := map[string]json.RawMessage{}
	overlayNetworkingConfiguration(ctx, base, &data)

	result, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	var want map[string]interface{}
	if err := json.Unmarshal([]byte(fixture), &want); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("round-trip mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}
