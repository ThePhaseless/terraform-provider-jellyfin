// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

const wireBindingsGolden = "testdata/wire_bindings.golden"

// typedClientResources talk to Jellyfin through internal/client's structs,
// whose json tags are their only spelling of each key.
var typedClientResources = map[string]string{
	"jellyfin_api_key":              "creates and lists keys through client.APIKey",
	"jellyfin_plugin":               "installs and removes plugins through client.Plugin and client.Package",
	"jellyfin_plugin_configuration": "stores the configuration as opaque JSON",
	"jellyfin_plugin_repository":    "writes client.PluginRepository entries",
	"jellyfin_restart":              "only posts a restart",
}

func TestUnitWireBindings(t *testing.T) {
	ctx := context.Background()
	seen := map[string]bool{}
	var lines []string
	for _, newResource := range New("test")().Resources(ctx) {
		r := newResource()
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "jellyfin"}, &meta)
		name := meta.TypeName
		seen[name] = true

		bound, isBound := r.(wireBound)
		pending, isPending := pendingWireMigration[name]
		_, isTyped := typedClientResources[name]
		var wireFn func() (*wire.Binding, error)
		switch {
		case isBound && (isPending || isTyped):
			t.Errorf("%s implements wireBound, so drop it from pendingWireMigration and typedClientResources", name)
			continue
		case isBound:
			wireFn = bound.Wire
		case isPending && isTyped:
			t.Errorf("%s is both pending migration and a typed-client resource", name)
			continue
		case isPending:
			wireFn = pending
		case isTyped:
			continue
		default:
			t.Errorf("%s neither implements wireBound nor is listed in typedClientResources", name)
			continue
		}

		b, err := wireFn()
		if err != nil {
			t.Errorf("%s:\n%v", name, err)
			continue
		}
		if !(types.ObjectType{AttrTypes: b.AttrTypes}).Equal(schemaOf(r).Type()) {
			t.Errorf("%s: the binding's attribute types differ from the resource schema", name)
		}
		for _, line := range b.Describe() {
			lines = append(lines, name+" "+line)
		}
	}
	for _, list := range []map[string]string{typedClientResources, pendingNames()} {
		for name := range list {
			if !seen[name] {
				t.Errorf("%s is listed but the provider has no such resource", name)
			}
		}
	}
	sort.Strings(lines)
	checkWireBindingsGolden(t, lines)
}

func pendingNames() map[string]string {
	out := map[string]string{}
	for name := range pendingWireMigration {
		out[name] = "pending migration"
	}
	return out
}

func checkWireBindingsGolden(t *testing.T, lines []string) {
	t.Helper()
	got := strings.Join(lines, "\n") + "\n"
	if os.Getenv("SCHEMA_GUARD_UPDATE") == "1" {
		if err := os.WriteFile(wireBindingsGolden, []byte(got), 0o600); err != nil {
			t.Fatalf("writing %s: %v", wireBindingsGolden, err)
		}
		return
	}
	want, err := os.ReadFile(wireBindingsGolden)
	if err != nil {
		t.Fatalf("reading %s: %v; run the test with SCHEMA_GUARD_UPDATE=1 to create it", wireBindingsGolden, err)
	}
	if string(want) == got {
		return
	}
	wantSet := map[string]bool{}
	for _, l := range strings.Split(strings.TrimSpace(string(want)), "\n") {
		wantSet[l] = true
	}
	gotSet := map[string]bool{}
	var msg strings.Builder
	for _, l := range lines {
		gotSet[l] = true
		if !wantSet[l] {
			msg.WriteString("  + " + l + "\n")
		}
	}
	for _, l := range strings.Split(strings.TrimSpace(string(want)), "\n") {
		if !gotSet[l] {
			msg.WriteString("  - " + l + "\n")
		}
	}
	t.Fatalf(`the bindings differ from %s:

%s
Each line is an attribute and the Jellyfin key it reads and writes, or an
object's keys that no attribute claims. A change here changes what the
provider sends: review it, then run the test with SCHEMA_GUARD_UPDATE=1 to
record it.`, wireBindingsGolden, msg.String())
}
