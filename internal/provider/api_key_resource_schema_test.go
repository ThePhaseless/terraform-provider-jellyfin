// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func TestAPIKeyResourceIDIsSensitive(t *testing.T) {
	t.Parallel()

	var resp resource.SchemaResponse
	(&APIKeyResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attr, ok := resp.Schema.Attributes["id"].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("id attribute type = %T, want schema.StringAttribute", resp.Schema.Attributes["id"])
	}
	if !attr.Sensitive {
		t.Fatal("id attribute must be sensitive because it mirrors access_token")
	}
}

// Another Create running at the same time adds a key of its own between the
// two listings; each resource must take the key it asked for.
func TestUnitCreatedAPIKeyTakesTheKeyOfItsApp(t *testing.T) {
	before := []client.APIKey{{AccessToken: "old", AppName: "grafana"}}
	after := []client.APIKey{
		{AccessToken: "old", AppName: "grafana"},
		{AccessToken: "tok-sonarr", AppName: "sonarr"},
		{AccessToken: "tok-radarr", AppName: "radarr"},
	}
	for _, test := range []struct {
		app, want string
		after     []client.APIKey
	}{
		{"radarr", "tok-radarr", after},
		{"sonarr", "tok-sonarr", after},
		{"renamed-by-the-server", "tok-sonarr", after[:2]},
		{"renamed-by-the-server", "", after},
		{"radarr", "", before},
	} {
		got := createdAPIKey(before, test.after, test.app)
		switch {
		case got == nil && test.want != "":
			t.Errorf("%s: found no key, want %s", test.app, test.want)
		case got != nil && got.AccessToken != test.want:
			t.Errorf("%s: took %s, want %q", test.app, got.AccessToken, test.want)
		}
	}
}
