// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

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
		_, isTyped := typedClientResources[name]
		switch {
		case isBound && isTyped:
			t.Errorf("%s implements wireBound, so drop it from typedClientResources", name)
			continue
		case isTyped:
			continue
		case !isBound:
			t.Errorf("%s neither implements wireBound nor is listed in typedClientResources", name)
			continue
		}

		b, err := bound.Wire()
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
	for name := range typedClientResources {
		if !seen[name] {
			t.Errorf("%s is listed but the provider has no such resource", name)
		}
	}
	sort.Strings(lines)
	checkWireBindingsGolden(t, lines)
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

// readWire reads raw through b into a new model, from a prior whose every
// attribute is null, as the read that follows an import does.
func readWire[T any](t *testing.T, b *wire.Binding, raw string) T {
	t.Helper()
	return readWireIn[T](context.Background(), t, b, raw)
}

func readWireIn[T any](ctx context.Context, t *testing.T, b *wire.Binding, raw string) T {
	t.Helper()
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("parsing %s: %v", raw, err)
	}
	obj, d := b.Flatten(ctx, doc, types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatalf("read: %v", d)
	}
	var m T
	if d := obj.As(ctx, &m, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatalf("read into %T: %v", m, d)
	}
	return m
}

// checkSameJSON fails t unless got marshals to the JSON want holds, in any
// key order.
func checkSameJSON(t *testing.T, got any, want string) {
	t.Helper()
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var g, w any
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("parsing %s: %v", want, err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("JSON mismatch\n got: %s\nwant: %s", raw, want)
	}
}
