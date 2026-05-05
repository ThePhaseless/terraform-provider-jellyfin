// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package jsontypes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/attr/xattr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var (
	_ basetypes.StringValuable                   = (*Normalized)(nil)
	_ basetypes.StringValuableWithSemanticEquals = (*Normalized)(nil)
	_ xattr.ValidateableAttribute                = (*Normalized)(nil)
	_ function.ValidateableParameter             = (*Normalized)(nil)
)

// Normalized represents a valid JSON string with semantic equality that ignores formatting noise.
type Normalized struct {
	basetypes.StringValue
}

func (v Normalized) Type(context.Context) attr.Type {
	return NormalizedType{}
}

func (v Normalized) Equal(o attr.Value) bool {
	other, ok := o.(Normalized)
	if !ok {
		return false
	}

	return v.StringValue.Equal(other.StringValue)
}

func (v Normalized) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	newValue, ok := newValuable.(Normalized)
	if !ok {
		diags.AddError(
			"Semantic Equality Check Error",
			"An unexpected value type was received while performing semantic equality checks. "+
				"Please report this to the provider developers.\n\n"+
				"Expected Value Type: "+fmt.Sprintf("%T", v)+"\n"+
				"Got Value Type: "+fmt.Sprintf("%T", newValuable),
		)

		return false, diags
	}

	result, err := jsonEqual(newValue.ValueString(), v.ValueString())
	if err != nil {
		diags.AddError(
			"Semantic Equality Check Error",
			"An unexpected error occurred while performing semantic equality checks. "+
				"Please report this to the provider developers.\n\n"+
				"Error: "+err.Error(),
		)

		return false, diags
	}

	return result, diags
}

func jsonEqual(s1, s2 string) (bool, error) {
	s1, err := normalizeJSONString(s1)
	if err != nil {
		return false, err
	}

	s2, err = normalizeJSONString(s2)
	if err != nil {
		return false, err
	}

	return s1 == s2, nil
}

func normalizeJSONString(jsonStr string) (string, error) {
	dec := json.NewDecoder(strings.NewReader(jsonStr))
	dec.UseNumber()

	var temp any
	if err := dec.Decode(&temp); err != nil {
		return "", err
	}

	jsonBytes, err := json.Marshal(&temp)
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}

func (v Normalized) ValidateAttribute(_ context.Context, req xattr.ValidateAttributeRequest, resp *xattr.ValidateAttributeResponse) {
	if v.IsUnknown() || v.IsNull() {
		return
	}

	if ok := json.Valid([]byte(v.ValueString())); !ok {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid JSON String Value",
			"A string value was provided that is not valid JSON string format (RFC 7159).\n\n"+
				"Given Value: "+v.ValueString()+"\n",
		)
	}
}

func (v Normalized) ValidateParameter(_ context.Context, req function.ValidateParameterRequest, resp *function.ValidateParameterResponse) {
	if v.IsUnknown() || v.IsNull() {
		return
	}

	if ok := json.Valid([]byte(v.ValueString())); !ok {
		resp.Error = function.NewArgumentFuncError(
			req.Position,
			"Invalid JSON String Value: "+
				"A string value was provided that is not valid JSON string format (RFC 7159).\n\n"+
				"Given Value: "+v.ValueString()+"\n",
		)
	}
}

func (v Normalized) Unmarshal(target any) diag.Diagnostics {
	var diags diag.Diagnostics

	if v.IsNull() {
		diags.Append(diag.NewErrorDiagnostic("Normalized JSON Unmarshal Error", "json string value is null"))
		return diags
	}

	if v.IsUnknown() {
		diags.Append(diag.NewErrorDiagnostic("Normalized JSON Unmarshal Error", "json string value is unknown"))
		return diags
	}

	if err := json.Unmarshal([]byte(v.ValueString()), target); err != nil {
		diags.Append(diag.NewErrorDiagnostic("Normalized JSON Unmarshal Error", err.Error()))
	}

	return diags
}

func NewNormalizedNull() Normalized {
	return Normalized{StringValue: basetypes.NewStringNull()}
}

func NewNormalizedUnknown() Normalized {
	return Normalized{StringValue: basetypes.NewStringUnknown()}
}

func NewNormalizedValue(value string) Normalized {
	return Normalized{StringValue: basetypes.NewStringValue(value)}
}

func NewNormalizedPointerValue(value *string) Normalized {
	return Normalized{StringValue: basetypes.NewStringPointerValue(value)}
}
