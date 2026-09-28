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
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/release"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"jellyfin": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()

	// Under TF_ACC a missing server is a broken environment, not an opt-out.
	skip := t.Skip
	if os.Getenv("TF_ACC") != "" {
		skip = t.Fatal
	}
	if os.Getenv("JELLYFIN_ENDPOINT") == "" {
		skip("JELLYFIN_ENDPOINT must be set for acceptance tests")
	}
	if os.Getenv("JELLYFIN_API_KEY") == "" && (os.Getenv("JELLYFIN_USERNAME") == "" || os.Getenv("JELLYFIN_PASSWORD") == "") {
		skip("JELLYFIN_API_KEY or JELLYFIN_USERNAME/JELLYFIN_PASSWORD must be set for acceptance tests")
	}
}

func testAccClient(t *testing.T) *client.Client {
	t.Helper()

	c, _, err := configureClient(
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

func testAccJellyfinVersionAtLeast(t *testing.T, minVersion string) bool {
	t.Helper()

	info, err := client.NewClient(os.Getenv("JELLYFIN_ENDPOINT"), "").GetPublicSystemInfo(context.Background())
	if err != nil {
		t.Fatalf("reading the Jellyfin version: %v", err)
	}
	return release.Compare(info.Version, minVersion) >= 0
}
