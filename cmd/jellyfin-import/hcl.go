// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// hclField maps a Jellyfin JSON property to a Terraform attribute. nested
// describes the attributes of each element when the property is a list of
// objects.
type hclField struct {
	json   string
	attr   string
	nested []hclField
}

// hclAttributes renders the fields present in a JSON object as HCL attribute
// values, skipping nulls. depth is the block nesting level the attributes are
// written at, used to indent multi-line values.
func hclAttributes(raw string, fields []hclField, depth int) (map[string]string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, fmt.Errorf("parsing JSON object: %w", err)
	}

	attrs := make(map[string]string, len(fields))
	for _, f := range fields {
		v, ok := obj[f.json]
		if !ok || string(v) == "null" {
			continue
		}
		rendered, err := hclValue(v, f.nested, depth)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.json, err)
		}
		attrs[f.attr] = rendered
	}
	return attrs, nil
}

func hclValue(raw json.RawMessage, nested []hclField, depth int) (string, error) {
	switch {
	case nested != nil:
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return "", err
		}
		if len(elements) == 0 {
			return "[]", nil
		}
		indent := strings.Repeat("  ", depth)
		var b strings.Builder
		b.WriteString("[\n")
		for _, e := range elements {
			attrs, err := hclAttributes(string(e), nested, depth+2)
			if err != nil {
				return "", err
			}
			b.WriteString(indent + "  {\n")
			keys := make([]string, 0, len(attrs))
			for k := range attrs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "%s    %s = %s\n", indent, k, attrs[k])
			}
			b.WriteString(indent + "  },\n")
		}
		b.WriteString(indent + "]")
		return b.String(), nil
	case bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")):
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return "", err
		}
		rendered := make([]string, len(elements))
		for i, e := range elements {
			s, err := hclValue(e, nil, depth)
			if err != nil {
				return "", err
			}
			rendered[i] = s
		}
		return "[" + strings.Join(rendered, ", ") + "]", nil
	case bytes.HasPrefix(bytes.TrimSpace(raw), []byte(`"`)):
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return hclString(s), nil
	default:
		// Numbers and booleans read the same in HCL as in JSON.
		return string(bytes.TrimSpace(raw)), nil
	}
}

// hclString quotes s as an HCL string literal, escaping control characters
// and the ${ and %{ template introducers, which HCL would otherwise
// interpolate.
func hclString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		case (r == '$' || r == '%') && strings.HasPrefix(s[i+1:], "{"):
			b.WriteRune(r)
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// importBlock generates a Terraform import block.
func importBlock(resourceType, name, id string) string {
	return fmt.Sprintf(`import {
  to = %s.%s
  id = %s
}
`, resourceType, name, hclString(id))
}

// resourceBlock generates a Terraform resource block from a map of attributes.
func resourceBlock(resourceType, name string, attrs map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "resource %s %s {\n", hclString(resourceType), hclString(name))

	keys := sortedKeys(attrs)
	for _, k := range keys {
		fmt.Fprintf(&b, "  %s = %s\n", k, attrs[k])
	}

	b.WriteString("}\n")
	return b.String()
}

// sortedKeys returns map keys in sorted order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
