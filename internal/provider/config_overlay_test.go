// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"testing"
)

func TestConfigOverlayParseJSONObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    map[string]json.RawMessage
		wantErr bool
	}{
		{
			name: "empty string",
			raw:  "",
			want: map[string]json.RawMessage{},
		},
		{
			name: "empty object",
			raw:  "{}",
			want: map[string]json.RawMessage{},
		},
		{
			name: "simple object",
			raw:  `{"a": "b", "c": true}`,
			want: map[string]json.RawMessage{
				"a": json.RawMessage(`"b"`),
				"c": json.RawMessage(`true`),
			},
		},
		{
			name:    "invalid json",
			raw:     `{"a":`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseJSONObject(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseJSONObject() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseJSONObject() len = %d, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if string(got[k]) != string(v) {
					t.Errorf("parseJSONObject()[%q] = %s, want %s", k, got[k], v)
				}
			}
		})
	}
}
