// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

// TestAccWireKeysMatchTheServer checks every binding of a Jellyfin resource
// against the OpenAPI document of the server the acceptance tests run, the one
// Jellyfin release the provider supports: each key a binding writes must be a
// property of the object the document describes there, of the JSON kind its
// codec writes, and a rebuilt object must leave out no property but those it
// declares Unmanaged. It fails on a misspelt key, such as one with an acronym
// KeyOf does not capitalize, which would otherwise read as null forever.
func TestAccWireKeysMatchTheServer(t *testing.T) {
	testAccPreCheck(t)
	ctx := t.Context()

	spec, err := testAccClient(t).GetOpenAPISpec(ctx)
	if err != nil {
		t.Fatalf("reading the OpenAPI document: %v", err)
	}
	api, err := parseOpenAPISchemas(spec)
	if err != nil {
		t.Fatal(err)
	}

	for _, newResource := range New("test")().Resources(ctx) {
		r := newResource()
		name := resourceTypeName(ctx, r)
		bound, ok := r.(wireBound)
		if !ok || name == "jellyfin_security_plugin_configuration" {
			continue
		}
		b, err := bound.Wire()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for _, o := range b.Objects() {
			served, err := api.objectAt(b.Object, o.KeyPath)
			if err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
			for _, problem := range compareWireObject(o, served) {
				t.Errorf("%s: %s in %s", name, problem, wireObjectName(b.Object, o.KeyPath))
			}
		}
	}
}

// TestAccSecurityPluginWireKeysMatchThePlugin checks the JellyfinSecurity
// binding against the configuration the supported plugin build serves, as
// TestAccWireKeysMatchTheServer does for Jellyfin's own objects, and checks
// that a write keeps every key the plugin serves, as the JSON type it serves.
func TestAccSecurityPluginWireKeysMatchThePlugin(t *testing.T) {
	testAccSecurityPluginPreCheck(t)
	c := testAccInstallSecurityPlugin(t)

	shape := testAccSecurityPluginPayloadShape(t, c)
	served, err := payloadShapeObjects(shape)
	if err != nil {
		t.Fatal(err)
	}
	b := mustWire(t, securityPluginWire)
	for _, o := range b.Objects() {
		keys, ok := served[o.KeyPath]
		if !ok {
			t.Errorf("the plugin serves no object at %s", wireObjectName(b.Object, o.KeyPath))
			continue
		}
		for _, problem := range compareWireObject(o, keys) {
			t.Errorf("%s in %s", problem, wireObjectName(b.Object, o.KeyPath))
		}
	}

	checkSecurityPluginWriteKeepsShape(t, b, shape)
}

// checkSecurityPluginWriteKeepsShape reads a payload holding a value for every
// line of shape and writes it back over itself: each rebuilt OIDC provider,
// role mapping and user email must keep every served key, and every value must
// go out as the JSON type the plugin serves.
func checkSecurityPluginWriteKeepsShape(t *testing.T, b *wire.Binding, shape []string) {
	t.Helper()
	payload, err := securityPluginPayloadFromShape(shape)
	if err != nil {
		t.Fatalf("building a payload from the served shape: %v", err)
	}
	data := readWire[JellyfinSecurityPluginConfigurationResourceModel](t, b, payload)

	written, err := parseJSONObject(payload)
	if err != nil {
		t.Fatalf("parsing payload: %v", err)
	}
	if d := b.OverlayModel(t.Context(), written, &data); d.HasError() {
		t.Fatalf("overlay: %v", d.Errors())
	}
	raw, err := json.Marshal(written)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reduceSecurityPluginPayload(string(raw))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	for _, line := range linesNotIn(shape, got) {
		t.Errorf("served %q is not written back by the resource", line)
	}
	for _, line := range linesNotIn(got, shape) {
		t.Errorf("resource writes %q, which the plugin does not serve", line)
	}
}

func wireObjectName(root, keyPath string) string {
	if keyPath == "" {
		return root
	}
	return root + "." + keyPath
}

// compareWireObject lists what o gets wrong about served, the object's keys
// with their JSON kinds; a kind of "" matches any.
func compareWireObject(o wire.Object, served map[string]wire.Kind) []string {
	var problems []string
	for _, key := range slices.Sorted(maps.Keys(o.Keys)) {
		want := o.Keys[key]
		got, ok := served[key]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("key %s is not a property the server has (it has %s)", key, strings.Join(slices.Sorted(maps.Keys(served)), ", ")))
		case !kindsMatch(want, got):
			problems = append(problems, fmt.Sprintf("key %s is written as %s, but the server has %s", key, want, got))
		}
	}
	if o.Rebuilt {
		for _, key := range slices.Sorted(maps.Keys(served)) {
			if _, claimed := o.Keys[key]; !claimed && !slices.Contains(o.Unmanaged, key) {
				problems = append(problems, fmt.Sprintf("rebuilding the object drops %s, which no attribute claims; bind it or declare it Unmanaged", key))
			}
		}
	}
	for _, key := range o.Unmanaged {
		if _, ok := served[key]; !ok {
			problems = append(problems, fmt.Sprintf("Unmanaged names %s, which the server does not have", key))
		}
	}
	return problems
}

// kindsMatch lets the plugin's untyped "number" stand for an integer, and an
// empty list it serves for a list of anything.
func kindsMatch(want, got wire.Kind) bool {
	switch {
	case want == "" || got == "" || want == got:
		return true
	case got == "number":
		return want == "integer"
	case got == "[]number":
		return want == "[]integer"
	case got == "[]":
		return strings.HasPrefix(string(want), "[]")
	}
	return false
}

// openAPISchemas holds the components.schemas of an OpenAPI document.
type openAPISchemas map[string]map[string]json.RawMessage

func parseOpenAPISchemas(spec string) (openAPISchemas, error) {
	var doc struct {
		Components struct {
			Schemas openAPISchemas `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(spec), &doc); err != nil {
		return nil, fmt.Errorf("parsing the OpenAPI document: %w", err)
	}
	return doc.Components.Schemas, nil
}

// objectAt returns the properties, with their kinds, of the object at keyPath
// from the schema root.
func (s openAPISchemas) objectAt(root, keyPath string) (map[string]wire.Kind, error) {
	schema, ok := s[root]
	if !ok {
		return nil, fmt.Errorf("the OpenAPI document has no schema %s", root)
	}
	at := root
	if keyPath != "" {
		for segment := range strings.SplitSeq(keyPath, ".") {
			key, isList := strings.CutSuffix(segment, "[]")
			props := s.properties(schema)
			raw, ok := props[key]
			if !ok {
				return nil, fmt.Errorf("%s has no property %s", at, key)
			}
			prop := s.resolve(raw)
			if isList {
				if jsonField[string](prop, "type") != "array" {
					return nil, fmt.Errorf("%s.%s is not a list", at, key)
				}
				prop = s.resolve(prop["items"])
			}
			if s.properties(prop) == nil {
				return nil, fmt.Errorf("%s.%s is not an object", at, segment)
			}
			schema, at = prop, at+"."+segment
		}
	}
	out := map[string]wire.Kind{}
	for key, raw := range s.properties(schema) {
		out[key] = s.kind(raw)
	}
	return out, nil
}

func (s openAPISchemas) properties(schema map[string]json.RawMessage) map[string]json.RawMessage {
	return jsonField[map[string]json.RawMessage](schema, "properties")
}

// resolve follows a $ref, directly or as the one schema of an allOf, oneOf or
// anyOf, which is how the document marks a nullable reference.
func (s openAPISchemas) resolve(raw json.RawMessage) map[string]json.RawMessage {
	var schema map[string]json.RawMessage
	if json.Unmarshal(raw, &schema) != nil {
		return nil
	}
	for _, combinator := range []string{"allOf", "oneOf", "anyOf"} {
		if parts := jsonField[[]json.RawMessage](schema, combinator); len(parts) == 1 {
			return s.resolve(parts[0])
		}
	}
	if ref := jsonField[string](schema, "$ref"); ref != "" {
		return s[strings.TrimPrefix(ref, "#/components/schemas/")]
	}
	return schema
}

func (s openAPISchemas) kind(raw json.RawMessage) wire.Kind {
	schema := s.resolve(raw)
	switch typ := jsonField[string](schema, "type"); {
	case typ == "array":
		return "[]" + s.kind(schema["items"])
	case typ == "object" || s.properties(schema) != nil:
		return "object"
	case typ == "integer" || typ == "number" || typ == "boolean" || typ == "string":
		return wire.Kind(typ)
	}
	return ""
}

func jsonField[T any](m map[string]json.RawMessage, key string) T {
	var v T
	if raw, ok := m[key]; ok {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

// payloadShapeObjects groups the lines of a reduced plugin payload into its
// objects, keyed by key path as wire.Object spells it.
func payloadShapeObjects(lines []string) (map[string]map[string]wire.Kind, error) {
	out := map[string]map[string]wire.Kind{"": {}}
	for _, line := range lines {
		keyPath, typ, ok := strings.Cut(line, ": ")
		if !ok {
			return nil, fmt.Errorf("malformed shape line %q", line)
		}
		parent, key := "", keyPath
		if i := strings.LastIndex(keyPath, "."); i >= 0 {
			parent, key = keyPath[:i], keyPath[i+1:]
		}
		if out[parent] == nil {
			out[parent] = map[string]wire.Kind{}
		}
		kind := wire.Kind(typ)
		switch typ {
		case "array":
			kind = "[]"
		case "null":
			kind = ""
		}
		out[parent][key] = kind
		switch typ {
		case "object":
			if out[keyPath] == nil {
				out[keyPath] = map[string]wire.Kind{}
			}
		case "[]object":
			if out[keyPath+"[]"] == nil {
				out[keyPath+"[]"] = map[string]wire.Kind{}
			}
		}
	}
	return out, nil
}

func linesNotIn(lines, other []string) []string {
	have := map[string]bool{}
	for _, line := range other {
		have[line] = true
	}
	var out []string
	for _, line := range lines {
		if !have[line] {
			out = append(out, line)
		}
	}
	return out
}
