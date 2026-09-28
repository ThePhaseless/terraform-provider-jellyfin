// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

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

func testAccRestartPreCheck(t *testing.T, what string) {
	t.Helper()

	if os.Getenv("JELLYFIN_RESTART_ACC") == "" {
		t.Skipf("set JELLYFIN_RESTART_ACC=1 to run %s; run against a disposable Jellyfin (e.g. the bundled docker-compose) in isolation, not a shared instance", what)
	}
	testAccPreCheck(t)
}

func testAccClient(t *testing.T) *client.Client {
	t.Helper()

	c, _, err := configureClient(
		context.WithoutCancel(t.Context()),
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

// testAccRegisterRepository registers the plugin repository for the rest of the
// test unless it already is, and puts the previous list back afterwards.
func testAccRegisterRepository(t *testing.T, name, repoURL string) {
	t.Helper()

	c := testAccClient(t)
	repos, err := c.GetPluginRepositories(t.Context())
	if err != nil {
		t.Fatalf("failed to get plugin repositories: %v", err)
	}
	for _, r := range repos {
		if r.URL == repoURL {
			return
		}
	}

	if err := c.SetPluginRepositories(t.Context(), append(repos, client.PluginRepository{Name: name, URL: repoURL, Enabled: true})); err != nil {
		t.Fatalf("failed to register repository %s: %v", repoURL, err)
	}
	t.Cleanup(func() {
		// t.Context() is done by now, and the provider's sign-in has signed c
		// out, since both share a device ID.
		if err := testAccClient(t).SetPluginRepositories(context.WithoutCancel(t.Context()), repos); err != nil {
			t.Errorf("failed to restore plugin repositories: %v", err)
		}
	})
}

func testAccJellyfin12OrLater(t *testing.T) bool {
	t.Helper()

	info, err := client.NewClient(os.Getenv("JELLYFIN_ENDPOINT"), "").GetPublicSystemInfo(t.Context())
	if err != nil {
		t.Fatalf("reading the Jellyfin version: %v", err)
	}
	return release.Compare(info.Version, "12") >= 0
}

// testAccAttr is a top-level attribute of a configuration resource: a string,
// bool, number or list of strings.
type testAccAttr struct {
	name  string
	value any
}

// testAccAttrsStep applies resourceType.test with attrs and checks that state
// holds each of them, a list by its length.
func testAccAttrsStep(resourceType string, attrs []testAccAttr) resource.TestStep {
	address := resourceType + ".test"
	var config strings.Builder
	checks := make([]resource.TestCheckFunc, 0, len(attrs))

	fmt.Fprintf(&config, "resource %q \"test\" {\n", resourceType)
	for _, a := range attrs {
		switch v := a.value.(type) {
		case string:
			fmt.Fprintf(&config, "  %s = %q\n", a.name, v)
			checks = append(checks, resource.TestCheckResourceAttr(address, a.name, v))
		case []string:
			quoted := make([]string, len(v))
			for i, s := range v {
				quoted[i] = strconv.Quote(s)
			}
			fmt.Fprintf(&config, "  %s = [%s]\n", a.name, strings.Join(quoted, ", "))
			checks = append(checks, resource.TestCheckResourceAttr(address, a.name+".#", strconv.Itoa(len(v))))
		default:
			fmt.Fprintf(&config, "  %s = %v\n", a.name, v)
			checks = append(checks, resource.TestCheckResourceAttr(address, a.name, fmt.Sprint(v)))
		}
	}
	config.WriteString("}\n")

	return resource.TestStep{Config: config.String(), Check: resource.ComposeAggregateTestCheckFunc(checks...)}
}
