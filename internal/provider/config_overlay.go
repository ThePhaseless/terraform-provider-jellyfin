// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// parseJSONObject parses a raw JSON string into a map of raw JSON messages.
// An empty string or "{}" returns an empty map.
func parseJSONObject(raw string) (map[string]json.RawMessage, error) {
	if raw == "" || raw == "{}" {
		return map[string]json.RawMessage{}, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, fmt.Errorf("failed to parse JSON object: %w", err)
	}

	return obj, nil
}

// putJSONString writes a string value into the JSON object map unless it is null or unknown.
func putJSONString(m map[string]json.RawMessage, key string, v types.String) {
	if v.IsNull() || v.IsUnknown() {
		return
	}

	b, _ := json.Marshal(v.ValueString())
	m[key] = b
}

// putJSONBool writes a bool value into the JSON object map unless it is null or unknown.
func putJSONBool(m map[string]json.RawMessage, key string, v types.Bool) {
	if v.IsNull() || v.IsUnknown() {
		return
	}

	b, _ := json.Marshal(v.ValueBool())
	m[key] = b
}

// putJSONInt64 writes an int64 value into the JSON object map unless it is null or unknown.
func putJSONInt64(m map[string]json.RawMessage, key string, v types.Int64) {
	if v.IsNull() || v.IsUnknown() {
		return
	}

	b, _ := json.Marshal(v.ValueInt64())
	m[key] = b
}

// putJSONFloat64 writes a float64 value into the JSON object map unless it is null or unknown.
func putJSONFloat64(m map[string]json.RawMessage, key string, v types.Float64) {
	if v.IsNull() || v.IsUnknown() {
		return
	}

	b, _ := json.Marshal(v.ValueFloat64())
	m[key] = b
}

// putJSONStringList writes a list of strings into the JSON object map unless it is null or unknown.
func putJSONStringList(ctx context.Context, m map[string]json.RawMessage, key string, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics

	if v.IsNull() || v.IsUnknown() {
		return diags
	}

	var elements []types.String
	if diagsExtend(&diags, v.ElementsAs(ctx, &elements, false)); diags.HasError() {
		return diags
	}

	values := make([]string, len(elements))
	for i, elem := range elements {
		values[i] = elem.ValueString()
	}

	b, _ := json.Marshal(values)
	m[key] = b

	return diags
}

// putJSONInt64List writes a list of int64 into the JSON object map unless it is null or unknown.
func putJSONInt64List(ctx context.Context, m map[string]json.RawMessage, key string, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics

	if v.IsNull() || v.IsUnknown() {
		return diags
	}

	var elements []types.Int64
	if diagsExtend(&diags, v.ElementsAs(ctx, &elements, false)); diagsHasError(diags) {
		return diags
	}

	values := make([]int64, len(elements))
	for i, elem := range elements {
		values[i] = elem.ValueInt64()
	}

	b, _ := json.Marshal(values)
	m[key] = b

	return diags
}

// putJSONStringMap writes a map of strings into the JSON object map unless it is null or unknown.
func putJSONStringMap(ctx context.Context, m map[string]json.RawMessage, key string, v types.Map) diag.Diagnostics {
	var diags diag.Diagnostics

	if v.IsNull() || v.IsUnknown() {
		return diags
	}

	var values map[string]string
	if diagsExtend(&diags, v.ElementsAs(ctx, &values, false)); diags.HasError() {
		return diags
	}

	b, _ := json.Marshal(values)
	m[key] = b

	return diags
}

// getJSONString reads a string value from the JSON object map.
// Returns null when the key is missing or the JSON value is null.
func getJSONString(m map[string]json.RawMessage, key string) types.String {
	raw, ok := m[key]
	if !ok {
		return types.StringNull()
	}

	if isJSONNull(raw) {
		return types.StringNull()
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return types.StringNull()
	}

	return types.StringValue(s)
}

// getJSONBool reads a bool value from the JSON object map.
// Returns null when the key is missing or the JSON value is null.
func getJSONBool(m map[string]json.RawMessage, key string) types.Bool {
	raw, ok := m[key]
	if !ok {
		return types.BoolNull()
	}

	if isJSONNull(raw) {
		return types.BoolNull()
	}

	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return types.BoolNull()
	}

	return types.BoolValue(b)
}

// getJSONInt64 reads an int64 value from the JSON object map.
// Returns null when the key is missing or the JSON value is null.
func getJSONInt64(m map[string]json.RawMessage, key string) types.Int64 {
	raw, ok := m[key]
	if !ok {
		return types.Int64Null()
	}

	if isJSONNull(raw) {
		return types.Int64Null()
	}

	var i int64
	if err := json.Unmarshal(raw, &i); err != nil {
		return types.Int64Null()
	}

	return types.Int64Value(i)
}

// getJSONFloat64 reads a float64 value from the JSON object map.
// Returns null when the key is missing or the JSON value is null.
func getJSONFloat64(m map[string]json.RawMessage, key string) types.Float64 {
	raw, ok := m[key]
	if !ok {
		return types.Float64Null()
	}

	if isJSONNull(raw) {
		return types.Float64Null()
	}

	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return types.Float64Null()
	}

	return types.Float64Value(f)
}

// getJSONStringList reads a list of strings from the JSON object map.
// Returns null when the key is missing or the JSON value is null.
func getJSONStringList(ctx context.Context, m map[string]json.RawMessage, key string) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics

	raw, ok := m[key]
	if !ok {
		return types.ListNull(types.StringType), diags
	}

	if isJSONNull(raw) {
		return types.ListNull(types.StringType), diags
	}

	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return types.ListNull(types.StringType), diags
	}

	elements := make([]types.String, len(values))
	for i, v := range values {
		elements[i] = types.StringValue(v)
	}

	list, listDiags := types.ListValueFrom(ctx, types.StringType, elements)
	return list, append(diags, listDiags...)
}

// getJSONInt64List reads a list of int64 from the JSON object map.
// Returns null when the key is missing or the JSON value is null.
func getJSONInt64List(ctx context.Context, m map[string]json.RawMessage, key string) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics

	raw, ok := m[key]
	if !ok {
		return types.ListNull(types.Int64Type), diags
	}

	if isJSONNull(raw) {
		return types.ListNull(types.Int64Type), diags
	}

	var values []int64
	if err := json.Unmarshal(raw, &values); err != nil {
		return types.ListNull(types.Int64Type), diags
	}

	elements := make([]types.Int64, len(values))
	for i, v := range values {
		elements[i] = types.Int64Value(v)
	}

	list, listDiags := types.ListValueFrom(ctx, types.Int64Type, elements)
	return list, append(diags, listDiags...)
}

// getJSONStringMap reads a map of strings from the JSON object map.
// Returns null when the key is missing or the JSON value is null.
func getJSONStringMap(ctx context.Context, m map[string]json.RawMessage, key string) (types.Map, diag.Diagnostics) {
	var diags diag.Diagnostics

	raw, ok := m[key]
	if !ok {
		return types.MapNull(types.StringType), diags
	}

	if isJSONNull(raw) {
		return types.MapNull(types.StringType), diags
	}

	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		return types.MapNull(types.StringType), diags
	}

	result, mapDiags := types.MapValueFrom(ctx, types.StringType, values)
	return result, append(diags, mapDiags...)
}

// isJSONNull reports whether raw is the JSON literal null (after trimming whitespace).
func isJSONNull(raw json.RawMessage) bool {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}

	return v == nil
}

// diagsExtend appends diagnostics from d to diags.
func diagsExtend(diags *diag.Diagnostics, d diag.Diagnostics) {
	*diags = append(*diags, d...)
}

// diagsHasError returns true if the diagnostics contain any errors.
func diagsHasError(diags diag.Diagnostics) bool {
	return diags.HasError()
}
