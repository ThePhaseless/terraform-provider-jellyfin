// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
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

func TestConfigOverlayPutGetPrimitives(t *testing.T) {
	t.Parallel()

	m := map[string]json.RawMessage{}
	putJSONString(m, "s", types.StringValue("hello"))
	putJSONBool(m, "b", types.BoolValue(true))
	putJSONInt64(m, "i", types.Int64Value(42))
	putJSONFloat64(m, "f", types.Float64Value(3.14))

	if got := getJSONString(m, "s"); got.ValueString() != "hello" {
		t.Errorf("string = %v, want hello", got)
	}
	if got := getJSONBool(m, "b"); got.ValueBool() != true {
		t.Errorf("bool = %v, want true", got)
	}
	if got := getJSONInt64(m, "i"); got.ValueInt64() != 42 {
		t.Errorf("int = %v, want 42", got)
	}
	if got := getJSONFloat64(m, "f"); got.ValueFloat64() != 3.14 {
		t.Errorf("float = %v, want 3.14", got)
	}
}

func TestConfigOverlayJSONNullPrimitives(t *testing.T) {
	t.Parallel()

	m := map[string]json.RawMessage{
		"string_null": json.RawMessage("null"),
		"bool_null":   json.RawMessage("null"),
		"int_null":    json.RawMessage("null"),
		"float_null":  json.RawMessage("null"),
	}

	if !getJSONString(m, "string_null").IsNull() {
		t.Errorf("expected string null to be typed null, got %v", getJSONString(m, "string_null"))
	}
	if !getJSONBool(m, "bool_null").IsNull() {
		t.Errorf("expected bool null to be typed null, got %v", getJSONBool(m, "bool_null"))
	}
	if !getJSONInt64(m, "int_null").IsNull() {
		t.Errorf("expected int null to be typed null, got %v", getJSONInt64(m, "int_null"))
	}
	if !getJSONFloat64(m, "float_null").IsNull() {
		t.Errorf("expected float null to be typed null, got %v", getJSONFloat64(m, "float_null"))
	}
}

func TestConfigOverlayStringList(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	m := map[string]json.RawMessage{}
	list, d := types.ListValueFrom(ctx, types.StringType, []string{"a", "b"})
	if d.HasError() {
		t.Fatalf("creating list: %v", d)
	}
	if d := putJSONStringList(ctx, m, "list", list); d.HasError() {
		t.Fatalf("putJSONStringList: %v", d)
	}

	got, d := getJSONStringList(ctx, m, "list")
	if d.HasError() {
		t.Fatalf("getJSONStringList: %v", d)
	}

	var values []string
	if d := got.ElementsAs(ctx, &values, false); d.HasError() {
		t.Fatalf("ElementsAs: %v", d)
	}
	if !reflect.DeepEqual(values, []string{"a", "b"}) {
		t.Errorf("list = %v, want [a b]", values)
	}
}

func TestConfigOverlayInt64List(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	m := map[string]json.RawMessage{}
	list, d := types.ListValueFrom(ctx, types.Int64Type, []int64{1, 2, 3})
	if d.HasError() {
		t.Fatalf("creating list: %v", d)
	}
	if d := putJSONInt64List(ctx, m, "list", list); d.HasError() {
		t.Fatalf("putJSONInt64List: %v", d)
	}

	got, d := getJSONInt64List(ctx, m, "list")
	if d.HasError() {
		t.Fatalf("getJSONInt64List: %v", d)
	}

	var values []int64
	if d := got.ElementsAs(ctx, &values, false); d.HasError() {
		t.Fatalf("ElementsAs: %v", d)
	}
	if !reflect.DeepEqual(values, []int64{1, 2, 3}) {
		t.Errorf("list = %v, want [1 2 3]", values)
	}
}

func TestConfigOverlayStringMap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	m := map[string]json.RawMessage{}
	v, d := types.MapValueFrom(ctx, types.StringType, map[string]string{"k1": "v1", "k2": "v2"})
	if d.HasError() {
		t.Fatalf("creating map: %v", d)
	}
	if d := putJSONStringMap(ctx, m, "map", v); d.HasError() {
		t.Fatalf("putJSONStringMap: %v", d)
	}

	got, d := getJSONStringMap(ctx, m, "map")
	if d.HasError() {
		t.Fatalf("getJSONStringMap: %v", d)
	}

	var values map[string]string
	if d := got.ElementsAs(ctx, &values, false); d.HasError() {
		t.Fatalf("ElementsAs: %v", d)
	}
	if !reflect.DeepEqual(values, map[string]string{"k1": "v1", "k2": "v2"}) {
		t.Errorf("map = %v", values)
	}
}

func TestConfigOverlayRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	base := `{"server_owned":"keep","string":"orig","number":1,"flag":false,"tags":["x"],"meta":{"k":"v"}}`
	baseMap, err := parseJSONObject(base)
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}

	putJSONString(baseMap, "string", types.StringValue("new"))
	putJSONInt64(baseMap, "number", types.Int64Value(2))
	putJSONBool(baseMap, "flag", types.BoolValue(true))
	list, _ := types.ListValueFrom(ctx, types.StringType, []string{"a", "b"})
	if d := putJSONStringList(ctx, baseMap, "tags", list); d.HasError() {
		t.Fatalf("put list: %v", d)
	}
	meta, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"k": "v2"})
	if d := putJSONStringMap(ctx, baseMap, "meta", meta); d.HasError() {
		t.Fatalf("put map: %v", d)
	}

	result, err := json.Marshal(baseMap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	resultMap, err := parseJSONObject(string(result))
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}

	if got := getJSONString(resultMap, "server_owned").ValueString(); got != "keep" {
		t.Errorf("server_owned = %q, want keep", got)
	}
	if got := getJSONString(resultMap, "string").ValueString(); got != "new" {
		t.Errorf("string = %q, want new", got)
	}
	if got := getJSONInt64(resultMap, "number").ValueInt64(); got != 2 {
		t.Errorf("number = %d, want 2", got)
	}
	if got := getJSONBool(resultMap, "flag").ValueBool(); got != true {
		t.Errorf("flag = %v, want true", got)
	}
}

func TestConfigOverlayUnknownKeysPreserved(t *testing.T) {
	t.Parallel()

	base := `{"unknown_key":"value"}`
	m, err := parseJSONObject(base)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	putJSONString(m, "known", types.StringValue("x"))

	result, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["unknown_key"] != "value" {
		t.Errorf("unknown_key not preserved: %v", parsed)
	}
}

func TestConfigOverlayGetIgnoresUnknownKeys(t *testing.T) {
	t.Parallel()

	m := map[string]json.RawMessage{"key": json.RawMessage(`"value"`)}
	if !getJSONString(m, "missing").IsNull() {
		t.Errorf("expected missing key to be null")
	}
}
