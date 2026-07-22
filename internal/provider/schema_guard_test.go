// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	jellyfinAPISchemaGolden     = "testdata/jellyfin_api_schema.golden"
	ssoPluginConfigSchemaGolden = "testdata/sso_plugin_config_schema.golden"
)

func TestAccJellyfinAPISchemaGuard(t *testing.T) {
	testAccPreCheck(t)
	c := testAccClient(t)

	spec, err := c.GetOpenAPISpec(context.Background())
	if err != nil {
		t.Fatalf("getting OpenAPI spec: %v", err)
	}

	lines, err := reduceOpenAPISpec(spec)
	if err != nil {
		t.Fatalf("reducing OpenAPI spec: %v", err)
	}

	checkSchemaGolden(t, jellyfinAPISchemaGolden, lines)
}

func reduceOpenAPISpec(spec string) ([]string, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(spec), &doc); err != nil {
		return nil, fmt.Errorf("parsing OpenAPI spec: %w", err)
	}

	var out []string

	// Paths: op <METHOD> <path> | params=...
	if rawPaths, ok := doc["paths"]; ok {
		var paths map[string]json.RawMessage
		if err := json.Unmarshal(rawPaths, &paths); err != nil {
			return nil, fmt.Errorf("parsing paths: %w", err)
		}

		var pathKeys []string
		for p := range paths {
			pathKeys = append(pathKeys, p)
		}
		sort.Strings(pathKeys)

		for _, p := range pathKeys {
			var ops map[string]json.RawMessage
			if err := json.Unmarshal(paths[p], &ops); err != nil {
				return nil, fmt.Errorf("parsing path %s: %w", p, err)
			}

			var opKeys []string
			for op := range ops {
				opKeys = append(opKeys, op)
			}
			sort.Strings(opKeys)

			for _, op := range opKeys {
				var opDoc map[string]json.RawMessage
				if err := json.Unmarshal(ops[op], &opDoc); err != nil {
					return nil, fmt.Errorf("parsing operation %s %s: %w", op, p, err)
				}

				var paramParts []string
				if rawParams, ok := opDoc["parameters"]; ok {
					var params []map[string]json.RawMessage
					if err := json.Unmarshal(rawParams, &params); err == nil {
						for _, param := range params {
							name := jsonString(param, "name")
							in := jsonString(param, "in")
							required := jsonBool(param, "required")
							paramParts = append(paramParts, fmt.Sprintf("%s:%s:required=%t", name, in, required))
						}
					}
				}
				sort.Strings(paramParts)

				line := fmt.Sprintf("op %s %s", strings.ToUpper(op), p)
				if len(paramParts) > 0 {
					line += " | params=" + strings.Join(paramParts, ",")
				}
				out = append(out, line)
			}
		}
	}

	// Components schemas: schema <name>: <signature>
	if rawComponents, ok := doc["components"]; ok {
		var components map[string]json.RawMessage
		if err := json.Unmarshal(rawComponents, &components); err != nil {
			return nil, fmt.Errorf("parsing components: %w", err)
		}

		if rawSchemas, ok := components["schemas"]; ok {
			var schemas map[string]json.RawMessage
			if err := json.Unmarshal(rawSchemas, &schemas); err != nil {
				return nil, fmt.Errorf("parsing schemas: %w", err)
			}

			var schemaKeys []string
			for name := range schemas {
				schemaKeys = append(schemaKeys, name)
			}
			sort.Strings(schemaKeys)

			for _, name := range schemaKeys {
				sig, err := schemaSignature(schemas, name, map[string]bool{})
				if err != nil {
					return nil, fmt.Errorf("signature for %s: %w", name, err)
				}
				out = append(out, fmt.Sprintf("schema %s: %s", name, sig))
			}
		}
	}

	sort.Strings(out)
	return dedupStrings(out), nil
}

func schemaSignature(schemas map[string]json.RawMessage, name string, visiting map[string]bool) (string, error) {
	if visiting[name] {
		return "*" + name, nil // cycle terminal
	}
	visiting[name] = true

	raw, ok := schemas[name]
	if !ok {
		return "", fmt.Errorf("schema %s not found", name)
	}

	var s map[string]json.RawMessage
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("parsing schema %s: %w", name, err)
	}

	return typeSignature(schemas, s, visiting)
}

func typeSignature(schemas map[string]json.RawMessage, s map[string]json.RawMessage, visiting map[string]bool) (string, error) {
	if ref := jsonString(s, "$ref"); ref != "" {
		refName := ref[strings.LastIndex(ref, "/")+1:]
		return schemaSignature(schemas, refName, visiting)
	}

	if rawAllOf, ok := s["allOf"]; ok {
		var allOf []map[string]json.RawMessage
		if err := json.Unmarshal(rawAllOf, &allOf); err != nil {
			return "", err
		}
		parts := []string{"allOf"}
		for _, item := range allOf {
			sig, err := typeSignature(schemas, item, visiting)
			if err != nil {
				return "", err
			}
			parts = append(parts, sig)
		}
		return strings.Join(parts, ","), nil
	}

	if rawAnyOf, ok := s["anyOf"]; ok {
		var anyOf []map[string]json.RawMessage
		if err := json.Unmarshal(rawAnyOf, &anyOf); err != nil {
			return "", err
		}
		parts := []string{"anyOf"}
		for _, item := range anyOf {
			sig, err := typeSignature(schemas, item, visiting)
			if err != nil {
				return "", err
			}
			parts = append(parts, sig)
		}
		return strings.Join(parts, ","), nil
	}

	if rawOneOf, ok := s["oneOf"]; ok {
		var oneOf []map[string]json.RawMessage
		if err := json.Unmarshal(rawOneOf, &oneOf); err != nil {
			return "", err
		}
		parts := []string{"oneOf"}
		for _, item := range oneOf {
			sig, err := typeSignature(schemas, item, visiting)
			if err != nil {
				return "", err
			}
			parts = append(parts, sig)
		}
		return strings.Join(parts, ","), nil
	}

	typ := jsonString(s, "type")
	format := jsonString(s, "format")

	switch typ {
	case "object":
		parts := []string{"object"}
		if rawProps, ok := s["properties"]; ok {
			var props map[string]json.RawMessage
			if err := json.Unmarshal(rawProps, &props); err != nil {
				return "", err
			}
			var keys []string
			for k := range props {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				var prop map[string]json.RawMessage
				if err := json.Unmarshal(props[k], &prop); err != nil {
					return "", err
				}
				sig, err := typeSignature(schemas, prop, visiting)
				if err != nil {
					return "", err
				}
				parts = append(parts, fmt.Sprintf("%s=%s", k, sig))
			}
		}
		if rawAdd, ok := s["additionalProperties"]; ok {
			var add map[string]json.RawMessage
			if err := json.Unmarshal(rawAdd, &add); err == nil {
				sig, err := typeSignature(schemas, add, visiting)
				if err != nil {
					return "", err
				}
				parts = append(parts, "map="+sig)
			}
		}
		return "{" + strings.Join(parts, ",") + "}", nil
	case "array":
		if rawItems, ok := s["items"]; ok {
			var items map[string]json.RawMessage
			if err := json.Unmarshal(rawItems, &items); err != nil {
				return "", err
			}
			sig, err := typeSignature(schemas, items, visiting)
			if err != nil {
				return "", err
			}
			return "[]" + sig, nil
		}
		return "[]any", nil
	case "string":
		if format != "" {
			return "string:" + format, nil
		}
		return "string", nil
	case "integer":
		if format != "" {
			return "integer:" + format, nil
		}
		return "integer", nil
	case "number":
		if format != "" {
			return "number:" + format, nil
		}
		return "number", nil
	case "boolean":
		return "boolean", nil
	case "":
		if jsonBool(s, "nullable") {
			return "null", nil
		}
		return "any", nil
	default:
		return typ, nil
	}
}

func jsonString(m map[string]json.RawMessage, key string) string {
	raw, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func jsonBool(m map[string]json.RawMessage, key string) bool {
	raw, ok := m[key]
	if !ok {
		return false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false
	}
	return b
}

func checkSchemaGolden(t *testing.T, goldenPath string, actual []string) {
	t.Helper()

	sort.Strings(actual)
	actual = dedupStrings(actual)

	if os.Getenv("SCHEMA_GUARD_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("creating golden directory: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(strings.Join(actual, "\n")+"\n"), 0600); err != nil {
			t.Fatalf("writing golden file: %v", err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden file %s: %v (set SCHEMA_GUARD_UPDATE=1 to create it)", goldenPath, err)
	}

	want := strings.Split(strings.TrimSpace(string(wantBytes)), "\n")
	wantSet := map[string]bool{}
	for _, line := range want {
		wantSet[line] = true
	}
	actualSet := map[string]bool{}
	for _, line := range actual {
		actualSet[line] = true
	}

	var missing []string
	for _, line := range want {
		if !actualSet[line] {
			missing = append(missing, line)
		}
	}
	var unexpected []string
	for _, line := range actual {
		if !wantSet[line] {
			unexpected = append(unexpected, line)
		}
	}

	if len(missing) > 0 || len(unexpected) > 0 {
		var msg strings.Builder
		msg.WriteString("schema guard mismatch:\n")
		if len(missing) > 0 {
			msg.WriteString("\nremoved or renamed (in golden, not served):\n")
			for _, line := range missing {
				msg.WriteString("  - ")
				msg.WriteString(line)
				msg.WriteString("\n")
			}
		}
		if len(unexpected) > 0 {
			msg.WriteString("\nadded (served, not in golden):\n")
			for _, line := range unexpected {
				msg.WriteString("  + ")
				msg.WriteString(line)
				msg.WriteString("\n")
			}
		}
		msg.WriteString("\nfull actual list:\n")
		for _, line := range actual {
			msg.WriteString(line)
			msg.WriteString("\n")
		}
		t.Fatal(msg.String())
	}
}

func dedupStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func TestAccSSOPluginConfigSchemaGuard(t *testing.T) {
	t.Skip("SSO plugin payload schema guard requires plugin installation; run manually with SCHEMA_GUARD_UPDATE=1 after installing the SSO-Auth plugin")
}

func TestUnitReduceOpenAPISpec(t *testing.T) {
	spec := "{\"paths\":{\"/System/Info\":{\"get\":{\"parameters\":[{\"name\":\"foo\",\"in\":\"query\",\"required\":true}]}}},\"components\":{\"schemas\":{\"SystemInfo.Version\":{\"type\":\"string\"},\"Parent\":{\"type\":\"object\",\"properties\":{\"child\":{\"$ref\":\"#/components/schemas/Child\"}}},\"Child\":{\"type\":\"object\",\"properties\":{\"name\":{\"type\":\"string\"},\"self\":{\"$ref\":\"#/components/schemas/Child\"}}}}}}"

	lines, err := reduceOpenAPISpec(spec)
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}

	want := []string{
		"op GET /System/Info | params=foo:query:required=true",
		"schema Child: {object,name=string,self=*Child}",
		"schema Parent: {object,child={object,name=string,self=*Child}}",
		"schema SystemInfo.Version: string",
	}

	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected lines:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}
