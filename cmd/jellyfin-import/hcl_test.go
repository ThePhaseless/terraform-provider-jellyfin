// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"testing"
)

func TestHCLString(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
		want  string
	}{
		"plain":                 {"hello", `"hello"`},
		"empty":                 {"", `""`},
		"quotes":                {`say "hi"`, `"say \"hi\""`},
		"backslash":             {`back\slash`, `"back\\slash"`},
		"template introducers":  {"${a} %{b}", `"$${a} %%{b}"`},
		"lone markers":          {"$c %d $ % {e}", `"$c %d $ % {e}"`},
		"escaped introducers":   {"$${a} %%{b}", `"$$${a} %%%{b}"`},
		"unterminated":          {"cost ${", `"cost $${"`},
		"control characters":    {"line\nnext\r\t\x01\x1f", `"line\nnext\r\t\u0001\u001f"`},
		"non-ASCII is kept":     {"café ☕", `"café ☕"`},
		"introducer after rune": {"é${x}", `"é$${x}"`},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := hclString(tc.input); got != tc.want {
				t.Errorf("hclString(%q) = %s, want %s", tc.input, got, tc.want)
			}
		})
	}
}

func TestHCLAttributesRendersObjectFields(t *testing.T) {
	fields := []hclField{
		{json: "Name", attr: "name"},
		{json: "Options", attr: "options", object: true, nested: []hclField{
			{json: "Size", attr: "size"},
			{json: "Tags", attr: "tags"},
			{json: "Missing", attr: "missing"},
			{json: "Entries", attr: "entries", nested: []hclField{
				{json: "Key", attr: "key"},
			}},
		}},
	}
	raw := `{"Name": "n", "Options": {"Size": 1.5, "Tags": ["a"], "Missing": null, "Entries": [{"Key": "k"}]}}`

	attrs, err := hclAttributes(raw, fields, 1)
	if err != nil {
		t.Fatalf("hclAttributes() error: %v", err)
	}

	want := `{
    entries = [
      {
        key = "k"
      },
    ]
    size = 1.5
    tags = ["a"]
  }`
	if attrs["options"] != want {
		t.Errorf("options =\n%s\nwant\n%s", attrs["options"], want)
	}
	if attrs["name"] != `"n"` {
		t.Errorf("name = %s, want \"n\"", attrs["name"])
	}
}
