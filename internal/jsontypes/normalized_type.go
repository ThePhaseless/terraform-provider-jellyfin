// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package jsontypes

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var _ basetypes.StringTypable = (*NormalizedType)(nil)

// NormalizedType is a string attribute type with semantic equality for JSON payloads.
type NormalizedType struct {
	basetypes.StringType
}

func (t NormalizedType) String() string {
	return "jsontypes.NormalizedType"
}

func (t NormalizedType) ValueType(context.Context) attr.Value {
	return Normalized{}
}

func (t NormalizedType) Equal(o attr.Type) bool {
	other, ok := o.(NormalizedType)
	if !ok {
		return false
	}

	return t.StringType.Equal(other.StringType)
}

func (t NormalizedType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return Normalized{StringValue: in}, nil
}

func (t NormalizedType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}

	stringValue, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type of %T", attrValue)
	}

	stringValuable, diags := t.ValueFromString(ctx, stringValue)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting StringValue to StringValuable: %v", diags)
	}

	return stringValuable, nil
}
