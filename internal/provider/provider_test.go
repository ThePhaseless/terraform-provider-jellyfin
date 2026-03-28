// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"jellyfin": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()

	if os.Getenv("JELLYFIN_ENDPOINT") == "" {
		t.Skip("JELLYFIN_ENDPOINT must be set for acceptance tests")
	}
	if os.Getenv("JELLYFIN_API_KEY") == "" {
		t.Skip("JELLYFIN_API_KEY must be set for acceptance tests")
	}
}
