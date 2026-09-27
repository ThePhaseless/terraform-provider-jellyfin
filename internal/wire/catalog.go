// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

// Package wire maps Terraform attributes to the JSON keys Jellyfin reads and
// writes. The keys come from the schema goldens the acceptance tests keep in
// schema/, so a key is spelled in one place: the server's own API document.
package wire

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"sync"
)

//go:embed schema/jellyfin_api_schema.golden
var pinnedAPIGolden string

//go:embed schema/jellyfin_api_schema_floor.golden
var floorAPIGolden string

//go:embed schema/security_plugin_config_schema.golden
var securityPluginGolden string

//go:embed schema/floor.env
var floorEnv string

// SecurityPluginRoot names the object that security_plugin_config_schema.golden
// describes.
const SecurityPluginRoot = "JellyfinSecurity"

type Prop struct {
	Key  string
	List bool
	// Ref names the object the property, or each of its elements, holds.
	Ref string
	// Scalar is boolean, integer, number, string or any; an enum is a string.
	Scalar string
	Format string
	// Sig is the property's type as the golden spells it.
	Sig string
}

// Index maps an object name to its properties by JSON key.
type Index map[string]map[string]Prop

func (ix Index) add(object string, p Prop) {
	if ix[object] == nil {
		ix[object] = map[string]Prop{}
	}
	ix[object][p.Key] = p
}

type catalog struct {
	pinned Index
	floor  Index
	// unversioned objects have no floor to derive Since from.
	unversioned map[string]bool
	floorVer    string
	sinceVer    string
}

var embedded = sync.OnceValue(func() *catalog {
	pinned := parseAPIGolden(pinnedAPIGolden)
	unversioned := map[string]bool{}
	for name, props := range parseSecurityGolden(SecurityPluginRoot, securityPluginGolden) {
		pinned[name] = props
		unversioned[name] = true
	}
	return &catalog{
		pinned:      pinned,
		floor:       parseAPIGolden(floorAPIGolden),
		unversioned: unversioned,
		floorVer:    envValue(floorEnv, "JELLYFIN_VERSION"),
		sinceVer:    envValue(floorEnv, "NEXT_JELLYFIN_VERSION"),
	}
})

// Pinned returns the objects of the supported Jellyfin and security plugin
// versions. Callers must not modify it.
func Pinned() Index { return embedded().pinned }

// FloorVersion is the Jellyfin release jellyfin_api_schema_floor.golden
// records.
func FloorVersion() string { return embedded().floorVer }

func envValue(content, key string) string {
	for _, line := range strings.Split(content, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), key+"="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// parseAPIGolden reads the "schema Object.Key: signature" lines of
// jellyfin_api_schema.golden. A line without a key describes a schema that is
// not an object, such as an enum, which properties referring to it take as
// their scalar type.
func parseAPIGolden(text string) Index {
	ix := Index{}
	aliases := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		rest, ok := strings.CutPrefix(line, "schema ")
		if !ok {
			continue
		}
		name, sig, ok := strings.Cut(rest, ": ")
		if !ok {
			continue
		}
		object, key, isProp := strings.Cut(name, ".")
		if !isProp {
			if strings.HasPrefix(sig, "{object") {
				if ix[object] == nil {
					ix[object] = map[string]Prop{}
				}
				continue
			}
			aliases[object] = sig
			continue
		}
		ix.add(object, parseSignature(key, sig))
	}
	for _, props := range ix {
		for key, p := range props {
			if alias, ok := aliases[p.Ref]; ok {
				p.Ref = ""
				p.Scalar, p.Format = scalarOf(alias)
				props[key] = p
			}
		}
	}
	return ix
}

func parseSignature(key, sig string) Prop {
	p := Prop{Key: key, Sig: sig}
	s := sig
	if elem, ok := strings.CutPrefix(s, "[]"); ok {
		p.List, s = true, elem
	}
	if ref, ok := strings.CutPrefix(s, "#"); ok {
		p.Ref = ref
		return p
	}
	p.Scalar, p.Format = scalarOf(s)
	return p
}

func scalarOf(sig string) (scalar, format string) {
	base, _, _ := strings.Cut(sig, " ")
	if strings.HasPrefix(base, "{") {
		return "object", ""
	}
	scalar, format, _ = strings.Cut(base, ":")
	return scalar, format
}

// parseSecurityGolden reads the "Path: type" lines of
// security_plugin_config_schema.golden into objects named after their path
// below root, such as "JellyfinSecurity.OidcProviders[]".
func parseSecurityGolden(root, text string) Index {
	ix := Index{root: {}}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		keyPath, typ, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		parent, key := root, keyPath
		if i := strings.LastIndex(keyPath, "."); i >= 0 {
			parent, key = root+"."+keyPath[:i], keyPath[i+1:]
		}
		p := Prop{Key: key, Sig: typ}
		t := typ
		if elem, ok := strings.CutPrefix(t, "[]"); ok {
			p.List, t = true, elem
		}
		switch t {
		case "object":
			p.Ref = parent + "." + key
			if p.List {
				p.Ref += "[]"
			}
		case "array":
			p.List, p.Scalar = true, "any"
		default:
			p.Scalar = t
		}
		ix.add(parent, p)
	}
	return ix
}

func norm(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "")) }

// Resolve returns the one property of object whose key equals name when case
// and underscores are ignored, which is how every attribute name of the
// provider relates to its Jellyfin key.
func (ix Index) Resolve(object, name string) (Prop, error) {
	props, ok := ix[object]
	if !ok {
		return Prop{}, fmt.Errorf("the goldens have no schema %q", object)
	}
	var hits []Prop
	for _, p := range props {
		if norm(p.Key) == norm(name) {
			hits = append(hits, p)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return Prop{}, fmt.Errorf("no property of %s matches %q (nearest %s)", object, name, ix.nearest(object, name))
	default:
		keys := make([]string, len(hits))
		for i, h := range hits {
			keys[i] = h.Key
		}
		sort.Strings(keys)
		return Prop{}, fmt.Errorf("%s has several properties matching %q: %s", object, name, strings.Join(keys, ", "))
	}
}

func (ix Index) nearest(object, name string) string {
	best, bestDist := "", -1
	keys := make([]string, 0, len(ix[object]))
	for k := range ix[object] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if d := levenshtein(norm(k), norm(name)); bestDist < 0 || d < bestDist {
			best, bestDist = k, d
		}
	}
	if best == "" {
		return "none"
	}
	return best
}

func levenshtein(a, b string) int {
	row := make([]int, len(b)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(a); i++ {
		diag := row[0]
		row[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			next := min(row[j]+1, row[j-1]+1, diag+cost)
			diag, row[j] = row[j], next
		}
	}
	return row[len(b)]
}

// lookup follows a key path from object through the objects its properties
// refer to, and returns the last property and the object that holds it.
func (ix Index) lookup(object string, keyPath []string) (Prop, string, error) {
	for i, key := range keyPath {
		p, ok := ix[object][key]
		if !ok {
			return Prop{}, "", fmt.Errorf("%s has no property %q", object, key)
		}
		if i == len(keyPath)-1 {
			return p, object, nil
		}
		if p.Ref == "" || p.List {
			return Prop{}, "", fmt.Errorf("%s.%s is not an object, so the key path %s cannot continue through it", object, key, strings.Join(keyPath, "."))
		}
		object = p.Ref
	}
	return Prop{}, "", fmt.Errorf("empty key path")
}
