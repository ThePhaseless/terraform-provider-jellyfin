// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Codec converts between an attribute value and its JSON.
//
// Encode gets only known, non-null values. Decode gets only a present value
// other than JSON null, and returns a null value for JSON it cannot read, so
// a value of the wrong type reads as null rather than failing the read.
// prior is the attribute's value before the read: the plan after an apply, the
// state on a refresh.
type Codec interface {
	Encode(ctx context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics)
	Decode(ctx context.Context, raw json.RawMessage, prior attr.Value, t attr.Type) (attr.Value, diag.Diagnostics)
}

func marshal(v any) (json.RawMessage, diag.Diagnostics) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("Failed to encode a Jellyfin value", err.Error())}
	}
	return b, nil
}

func wrongType(v attr.Value, want string) diag.Diagnostics {
	return diag.Diagnostics{diag.NewErrorDiagnostic("Unexpected attribute value",
		fmt.Sprintf("Expected a %s value, got %T. This is a bug in the provider.", want, v))}
}

var (
	boolType       = types.BoolType
	stringListType = types.ListType{ElemType: types.StringType}
)

type stringCodec struct{}

func (stringCodec) Encode(ctx context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics) {
	s, ok := v.(basetypes.StringValuable)
	if !ok {
		return nil, wrongType(v, "string")
	}
	sv, d := s.ToStringValue(ctx)
	if d.HasError() {
		return nil, d
	}
	return marshal(sv.ValueString())
}

func (stringCodec) Decode(_ context.Context, raw json.RawMessage, _ attr.Value, _ attr.Type) (attr.Value, diag.Diagnostics) {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return types.StringNull(), nil
	}
	return types.StringValue(s), nil
}

type boolCodec struct{ inverted bool }

func (c boolCodec) String() string {
	if c.inverted {
		return "inverted"
	}
	return ""
}

func (c boolCodec) Encode(ctx context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics) {
	b, ok := v.(basetypes.BoolValuable)
	if !ok {
		return nil, wrongType(v, "bool")
	}
	bv, d := b.ToBoolValue(ctx)
	if d.HasError() {
		return nil, d
	}
	return marshal(bv.ValueBool() != c.inverted)
}

func (c boolCodec) Decode(_ context.Context, raw json.RawMessage, _ attr.Value, _ attr.Type) (attr.Value, diag.Diagnostics) {
	var b bool
	if json.Unmarshal(raw, &b) != nil {
		return types.BoolNull(), nil
	}
	return types.BoolValue(b != c.inverted), nil
}

type int64Codec struct{}

func (int64Codec) Encode(ctx context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics) {
	i, ok := v.(basetypes.Int64Valuable)
	if !ok {
		return nil, wrongType(v, "int64")
	}
	iv, d := i.ToInt64Value(ctx)
	if d.HasError() {
		return nil, d
	}
	return marshal(iv.ValueInt64())
}

func (int64Codec) Decode(_ context.Context, raw json.RawMessage, _ attr.Value, _ attr.Type) (attr.Value, diag.Diagnostics) {
	var i int64
	if json.Unmarshal(raw, &i) != nil {
		return types.Int64Null(), nil
	}
	return types.Int64Value(i), nil
}

type float64Codec struct{}

func (float64Codec) Encode(ctx context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics) {
	f, ok := v.(basetypes.Float64Valuable)
	if !ok {
		return nil, wrongType(v, "float64")
	}
	fv, d := f.ToFloat64Value(ctx)
	if d.HasError() {
		return nil, d
	}
	return marshal(fv.ValueFloat64())
}

func (float64Codec) Decode(_ context.Context, raw json.RawMessage, _ attr.Value, _ attr.Type) (attr.Value, diag.Diagnostics) {
	var f float64
	if json.Unmarshal(raw, &f) != nil {
		return types.Float64Null(), nil
	}
	return types.Float64Value(f), nil
}

func listElements(ctx context.Context, v attr.Value) ([]attr.Value, diag.Diagnostics) {
	l, ok := v.(basetypes.ListValuable)
	if !ok {
		return nil, wrongType(v, "list")
	}
	lv, d := l.ToListValue(ctx)
	if d.HasError() {
		return nil, d
	}
	return lv.Elements(), nil
}

// stringsOf reads each element as its string, so an unknown element becomes "".
func stringsOf(ctx context.Context, v attr.Value) ([]string, diag.Diagnostics) {
	elems, d := listElements(ctx, v)
	if d.HasError() {
		return nil, d
	}
	out := make([]string, len(elems))
	for i, e := range elems {
		s, ok := e.(basetypes.StringValuable)
		if !ok {
			return nil, wrongType(e, "string")
		}
		sv, d := s.ToStringValue(ctx)
		if d.HasError() {
			return nil, d
		}
		out[i] = sv.ValueString()
	}
	return out, nil
}

func stringList(values []string) attr.Value {
	elems := make([]attr.Value, len(values))
	for i, s := range values {
		elems[i] = types.StringValue(s)
	}
	return types.ListValueMust(types.StringType, elems)
}

type stringListCodec struct{}

func (stringListCodec) Encode(ctx context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics) {
	values, d := stringsOf(ctx, v)
	if d.HasError() {
		return nil, d
	}
	return marshal(values)
}

func (stringListCodec) Decode(_ context.Context, raw json.RawMessage, _ attr.Value, _ attr.Type) (attr.Value, diag.Diagnostics) {
	var values []string
	if json.Unmarshal(raw, &values) != nil {
		return types.ListNull(types.StringType), nil
	}
	return stringList(values), nil
}

type int64ListCodec struct{}

func (int64ListCodec) Encode(ctx context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics) {
	elems, d := listElements(ctx, v)
	if d.HasError() {
		return nil, d
	}
	values := make([]int64, len(elems))
	for i, e := range elems {
		n, ok := e.(basetypes.Int64Valuable)
		if !ok {
			return nil, wrongType(e, "int64")
		}
		nv, d := n.ToInt64Value(ctx)
		if d.HasError() {
			return nil, d
		}
		values[i] = nv.ValueInt64()
	}
	return marshal(values)
}

func (int64ListCodec) Decode(_ context.Context, raw json.RawMessage, _ attr.Value, _ attr.Type) (attr.Value, diag.Diagnostics) {
	var values []int64
	if json.Unmarshal(raw, &values) != nil {
		return types.ListNull(types.Int64Type), nil
	}
	elems := make([]attr.Value, len(values))
	for i, n := range values {
		elems[i] = types.Int64Value(n)
	}
	return types.ListValueMust(types.Int64Type, elems), nil
}

// delimitedCodec stores a list of strings as one string. An empty string reads
// as an empty list, so an empty list round-trips.
type delimitedCodec struct{ sep string }

func (c delimitedCodec) String() string { return fmt.Sprintf("delimited(%q)", c.sep) }

func (c delimitedCodec) Encode(ctx context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics) {
	values, d := stringsOf(ctx, v)
	if d.HasError() {
		return nil, d
	}
	return marshal(strings.Join(values, c.sep))
}

func (c delimitedCodec) Decode(_ context.Context, raw json.RawMessage, _ attr.Value, _ attr.Type) (attr.Value, diag.Diagnostics) {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return types.ListNull(types.StringType), nil
	}
	if s == "" {
		return stringList(nil), nil
	}
	return stringList(strings.Split(s, c.sep)), nil
}

// defaultCodec picks the codec an attribute of type t gets for property p, or
// fails when the two do not match without a declared codec. A legacy key has
// no property in the pinned golden, so only its attribute type counts.
func defaultCodec(t attr.Type, p Prop, legacy bool) (Codec, error) {
	// fits reports whether the property is a list or not, as list says, of one
	// of the scalars; a legacy key fits whatever the attribute's type.
	fits := func(list bool, scalars ...string) bool {
		return legacy || p.List == list && p.Ref == "" && slices.Contains(scalars, p.Scalar)
	}
	// The security plugin golden types every number "number", with no format
	// telling integers apart, so an unformatted number takes an int64 too.
	integer := legacy || p.Scalar == "integer" || p.Scalar == "number" && p.Format == ""
	switch {
	case t.Equal(types.StringType) && fits(false, "string"):
		return stringCodec{}, nil
	case t.Equal(boolType) && fits(false, "boolean"):
		return boolCodec{}, nil
	case t.Equal(types.Int64Type) && integer && fits(false, "integer", "number"):
		return int64Codec{}, nil
	case t.Equal(types.Float64Type) && fits(false, "number"):
		return float64Codec{}, nil
	case t.Equal(stringListType) && fits(true, "string"):
		return stringListCodec{}, nil
	case t.Equal(types.ListType{ElemType: types.Int64Type}) && integer && fits(true, "integer", "number"):
		return int64ListCodec{}, nil
	}
	return nil, fmt.Errorf("attribute type %s and wire type %s have no default codec; declare one", t, sigOf(p, legacy))
}

func sigOf(p Prop, legacy bool) string {
	if legacy && p.Sig == "" {
		return "(legacy)"
	}
	return p.Sig
}

func codecName(c Codec) string {
	if s, ok := c.(fmt.Stringer); ok {
		return s.String()
	}
	switch c.(type) {
	case stringCodec, int64Codec, float64Codec, stringListCodec, int64ListCodec:
		return ""
	}
	return "custom"
}
