// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"sort"
	"strings"
)

// hclObject renders attrs as an HCL object whose closing brace is indented
// to depth.
func hclObject(attrs map[string]string, depth int) string {
	indent := strings.Repeat("  ", depth)
	var b strings.Builder
	b.WriteString("{\n")
	writeAttributes(&b, attrs, indent+"  ")
	b.WriteString(indent + "}")
	return b.String()
}

// writeAttributes writes attrs sorted by name, aligning the equals signs the
// way terraform fmt does: across each run of consecutive single-line values,
// which a multi-line value ends without being aligned itself.
func writeAttributes(b *strings.Builder, attrs map[string]string, indent string) {
	keys := sortedKeys(attrs)
	for start := 0; start < len(keys); {
		end, width := start, 0
		for ; end < len(keys) && !strings.Contains(attrs[keys[end]], "\n"); end++ {
			width = max(width, len(keys[end]))
		}
		for _, k := range keys[start:end] {
			fmt.Fprintf(b, "%s%-*s = %s\n", indent, width, k, attrs[k])
		}
		if end < len(keys) {
			fmt.Fprintf(b, "%s%s = %s\n", indent, keys[end], attrs[keys[end]])
			end++
		}
		start = end
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

func importBlock(resourceType, name, id string) string {
	return fmt.Sprintf(`import {
  to = %s.%s
  id = %s
}
`, resourceType, name, hclString(id))
}

func resourceBlock(resourceType, name string, attrs map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "resource %s %s {\n", hclString(resourceType), hclString(name))
	writeAttributes(&b, attrs, "  ")
	b.WriteString("}\n")
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
