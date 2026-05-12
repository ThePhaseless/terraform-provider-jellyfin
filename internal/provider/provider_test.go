// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"jellyfin": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()

	if os.Getenv("JELLYFIN_ENDPOINT") == "" {
		t.Skip("JELLYFIN_ENDPOINT must be set for acceptance tests")
	}
	if os.Getenv("JELLYFIN_API_KEY") == "" && (os.Getenv("JELLYFIN_USERNAME") == "" || os.Getenv("JELLYFIN_PASSWORD") == "") {
		t.Skip("JELLYFIN_API_KEY or JELLYFIN_USERNAME/JELLYFIN_PASSWORD must be set for acceptance tests")
	}
}

func testAccClient(t *testing.T) *client.Client {
	t.Helper()

	c, err := configureClient(
		context.Background(),
		os.Getenv("JELLYFIN_ENDPOINT"),
		os.Getenv("JELLYFIN_API_KEY"),
		os.Getenv("JELLYFIN_USERNAME"),
		os.Getenv("JELLYFIN_PASSWORD"),
	)
	if err != nil {
		t.Fatalf("failed to configure Jellyfin acceptance test client: %v", err)
	}
	return c
}
