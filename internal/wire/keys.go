// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

// Package wire maps Terraform attributes to the JSON keys Jellyfin reads and
// writes. A key is the attribute's name in PascalCase unless an option names
// another, so a key is spelled in one place; Objects lists what a binding
// writes, for a test to check against a live server.
package wire

import (
	"maps"
	"slices"
	"strings"
)

// KeyOf returns the key an attribute name maps to by default: each
// underscore-separated word capitalized, as Jellyfin spells most keys.
// Acronyms Jellyfin capitalizes whole, such as EnableIPv6, take a Key option.
func KeyOf(name string) string {
	var b strings.Builder
	for word := range strings.SplitSeq(name, "_") {
		if word == "" {
			continue
		}
		b.WriteString(strings.ToUpper(word[:1]))
		b.WriteString(word[1:])
	}
	return b.String()
}

func norm(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "")) }

// Kind is the JSON shape a key holds, as its codec writes it: boolean,
// integer, number or string, prefixed with [] for a list, or object. A codec
// the binding cannot describe has the kind "".
type Kind string

// Object is one JSON object a binding writes: the document, a nested object
// or each element of a nested list.
type Object struct {
	// KeyPath leads to it from the document, with [] after a list's key; the
	// document's is "".
	KeyPath string
	// Rebuilt objects are written from the attributes alone, so a key the
	// server has that Keys lacks is dropped unless Unmanaged lists it.
	// Other objects are written over the served object and keep such keys.
	Rebuilt bool
	Keys    map[string]Kind
	// Attrs names, for each key an attribute writes, that attribute and the
	// key path it maps to, as a Key option would spell it.
	Attrs     map[string]KeyOwner
	Unmanaged []string
}

// KeyOwner is the attribute that writes a key, and its key path.
type KeyOwner struct {
	Attr    string
	KeyPath string
}

// Objects lists every object b writes, in the order Bind reached them.
func (b *Binding) Objects() []Object {
	kinds := map[string]map[string]Kind{}
	owners := map[string]map[string]KeyOwner{}
	var walk func(x *Binding, inst string)
	walk = func(x *Binding, inst string) {
		for _, f := range x.Fields {
			if !f.Mode.hasKey() {
				continue
			}
			container := joinKeyPath(inst, strings.Join(f.KeyPath[:len(f.KeyPath)-1], "."))
			if kinds[container] == nil {
				kinds[container] = map[string]Kind{}
			}
			kinds[container][f.key()] = f.kind()
			if owners[container] == nil {
				owners[container] = map[string]KeyOwner{}
			}
			owners[container][f.key()] = KeyOwner{Attr: f.Path, KeyPath: strings.Join(f.KeyPath, ".")}
			switch {
			case f.Elem != nil && f.isList():
				walk(f.Elem, joinKeyPath(inst, strings.Join(f.KeyPath, "."))+"[]")
			case f.Elem != nil:
				walk(f.Elem, joinKeyPath(inst, strings.Join(f.KeyPath, ".")))
			}
		}
	}
	walk(b, "")

	out := make([]Object, 0, len(b.instances))
	for _, inst := range b.instances {
		keys := kinds[inst.keyPath]
		if keys == nil {
			keys = map[string]Kind{}
		}
		for key := range inst.claimed {
			if _, ok := keys[key]; !ok {
				// Carried or shared keys have no attribute of their own.
				keys[key] = ""
			}
		}
		attrs := owners[inst.keyPath]
		if attrs == nil {
			attrs = map[string]KeyOwner{}
		}
		out = append(out, Object{
			KeyPath:   inst.keyPath,
			Rebuilt:   inst.rebuilt,
			Keys:      keys,
			Attrs:     attrs,
			Unmanaged: slices.Sorted(maps.Keys(inst.unmanaged)),
		})
	}
	return out
}

func (f *Field) kind() Kind {
	if f.Elem != nil {
		if f.isList() {
			return "[]object"
		}
		return "object"
	}
	switch f.Codec.(type) {
	case stringCodec, delimitedCodec:
		return "string"
	case boolCodec:
		return "boolean"
	case int64Codec:
		return "integer"
	case float64Codec:
		return "number"
	case stringListCodec:
		return "[]string"
	case int64ListCodec:
		return "[]integer"
	}
	return ""
}
