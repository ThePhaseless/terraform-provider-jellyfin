// Copyright Sofie Finnes 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"testing"
)

func TestUnitPluginConfigurationMerge(t *testing.T) {
	cases := []struct{ name, current, desired, want string }{
		{"runtime mappings", `{"LdapUsers":[{"id":"123"}],"Host":"old"}`, `{"Host":"new"}`, `{"Host":"new","LdapUsers":[{"id":"123"}]}`},
		{"nested providers", `{"OIDC":{"a":{"secret":"keep","enabled":false},"b":{"enabled":true}}}`, `{"OIDC":{"a":{"enabled":true}}}`, `{"OIDC":{"a":{"enabled":true,"secret":"keep"},"b":{"enabled":true}}}`},
		{"atomic array", `{"users":[{"id":1,"extra":true},{"id":2}]}`, `{"users":[{"id":3}]}`, `{"users":[{"id":3}]}`},
		{"explicit null", `{"value":{"other":1},"keep":2}`, `{"value":null}`, `{"keep":2,"value":null}`},
		{"empty root", `{"keep":9007199254740993}`, `{}`, `{"keep":9007199254740993}`},
		{"empty nested", `{"value":{"keep":1}}`, `{"value":{}}`, `{"value":{"keep":1}}`},
		{"replace scalar with object", `{"value":false}`, `{"value":{}}`, `{"value":{}}`},
		{"replace object with scalar", `{"value":{"keep":1}}`, `{"value":true}`, `{"value":true}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := mergeConfiguration(c.current, c.desired)
			if err != nil {
				t.Fatal(err)
			}
			want, err := normalizeJSON(c.want)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("got %s; want %s", got, want)
			}
		})
	}
	for _, invalid := range []string{`null`, `[]`, `1`, `true`, `"secret"`, `{} {}`} {
		if _, err := mergeConfiguration(`{}`, invalid); err == nil {
			t.Errorf("accepted %s", invalid)
		}
		if _, err := mergeConfiguration(invalid, `{}`); err == nil {
			t.Errorf("accepted server %s", invalid)
		}
	}
}

func TestUnitPluginConfigurationOwnershipSurvivesDrift(t *testing.T) {
	mask, err := configurationOwnership(`{"section":{"secret":"never store this","array":[{"value":1}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	if string(mask) != `{"section":{"array":null,"secret":null}}` {
		t.Fatalf("unexpected mask %s", mask)
	}
	for _, remote := range []string{`{}`, `{"section":null}`, `{"section":false}`, `{"section":{}}`, `{"section":{"secret":"changed","array":[{"value":1,"extra":true}],"unowned":true}}`} {
		for range 2 {
			got, err := servedConfiguration(remote, string(mask))
			if err != nil {
				t.Fatal(err)
			}
			var observed map[string]any
			if err := json.Unmarshal([]byte(got), &observed); err != nil {
				t.Fatal(err)
			}
			if section, ok := observed["section"].(map[string]any); ok {
				if _, exists := section["unowned"]; exists {
					t.Fatal("unowned field leaked")
				}
				if list, ok := section["array"].([]any); ok {
					element, ok := list[0].(map[string]any)
					if !ok || element["extra"] != true {
						t.Fatal("array was projected by index")
					}
				}
			}
		}
	}
}
