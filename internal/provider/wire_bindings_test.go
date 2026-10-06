// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

// typedClientResources talk to Jellyfin through internal/client's structs,
// whose json tags are their only spelling of each key.
var typedClientResources = map[string]string{
	"jellyfin_api_key":              "creates and lists keys through client.APIKey",
	"jellyfin_plugin":               "installs and removes plugins through client.Plugin and client.Package",
	"jellyfin_plugin_configuration": "stores the configuration as opaque JSON",
	"jellyfin_plugin_repository":    "writes client.PluginRepository entries",
	"jellyfin_restart":              "only posts a restart",
}

// TestUnitWireBindings binds every resource that reads and writes Jellyfin
// JSON, so a binding Bind rejects fails here rather than at plan time.
// TestAccWireKeysMatchTheServer checks the keys against a server.
func TestUnitWireBindings(t *testing.T) {
	ctx := t.Context()
	seen := map[string]bool{}
	for _, newResource := range New("test")().Resources(ctx) {
		r := newResource()
		name := resourceTypeName(ctx, r)
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
	}
	for name := range typedClientResources {
		if !seen[name] {
			t.Errorf("%s is listed but the provider has no such resource", name)
		}
	}
}

func mustWire(t *testing.T, bind func() (*wire.Binding, error)) *wire.Binding {
	t.Helper()
	b, err := bind()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func checkRoundTrip[T any](t *testing.T, b *wire.Binding, fixture string) {
	t.Helper()
	data := readWire[T](t, b, fixture)
	checkSameJSON(t, writeWire(t, b, &data), fixture)
}

func writeWire(t *testing.T, b *wire.Binding, model any) map[string]json.RawMessage {
	t.Helper()
	doc := map[string]json.RawMessage{}
	if d := b.OverlayModel(t.Context(), doc, model); d.HasError() {
		t.Fatalf("write: %v", d)
	}
	return doc
}

// readWire reads raw through b into a new model, from a prior whose every
// attribute is null, as the read that follows an import does.
func readWire[T any](t *testing.T, b *wire.Binding, raw string) T {
	t.Helper()
	return readWireIn[T](t.Context(), t, b, raw)
}

func readWireIn[T any](ctx context.Context, t *testing.T, b *wire.Binding, raw string) T {
	t.Helper()
	obj := flattenFromNull(ctx, t, b, raw)
	var m T
	if d := obj.As(ctx, &m, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatalf("read into %T: %v", m, d)
	}
	return m
}

func flattenFromNull(ctx context.Context, t *testing.T, b *wire.Binding, raw string) types.Object {
	t.Helper()
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("parsing %s: %v", raw, err)
	}
	obj, d := b.Flatten(ctx, doc, types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatalf("read: %v", d)
	}
	return obj
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
