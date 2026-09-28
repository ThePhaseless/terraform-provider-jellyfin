// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// OverlayModel is Overlay for model, a pointer to the resource's model struct.
func (b *Binding) OverlayModel(ctx context.Context, doc map[string]json.RawMessage, model any) diag.Diagnostics {
	obj, diags := types.ObjectValueFrom(ctx, b.AttrTypes, model)
	if diags.HasError() {
		return diags
	}
	return append(diags, b.Overlay(ctx, doc, obj)...)
}

// Overlay writes each known attribute of obj into doc, the document as the
// server serves it, so the keys no attribute claims keep their served values.
// A null attribute is left out unless it NullClears. Keys come from the
// goldens only. A served key spelled otherwise than the golden stays: the
// metadata, encoding, network and Live TV endpoints read keys case-sensitively,
// so the served spelling may be the only one the server reads, and dropping it
// would reset that setting to its default.
func (b *Binding) Overlay(ctx context.Context, doc map[string]json.RawMessage, obj types.Object) diag.Diagnostics {
	if doc == nil {
		return diag.Diagnostics{diag.NewErrorDiagnostic("Missing Jellyfin document",
			fmt.Sprintf("There is no %s document to write into. This is a bug in the provider.", b.Object))}
	}
	return b.overlay(ctx, doc, obj, true, path.Empty(), "")
}

func (b *Binding) overlay(ctx context.Context, doc map[string]json.RawMessage, obj types.Object, merged bool, at path.Path, trail string) diag.Diagnostics {
	var diags diag.Diagnostics
	if obj.IsNull() || obj.IsUnknown() {
		return diags
	}
	for _, d := range b.docs {
		v, ok := valueAt(obj, d.attrPath)
		if !ok {
			continue
		}
		diags.Append(writeField(ctx, doc, d, v, merged, attrPathOf(at, d.attrPath), trail)...)
		if len(d.f.Shares) > 0 && !diags.HasError() {
			diags.Append(b.writeShared(ctx, doc, obj, d, v, merged, attrPathOf(at, d.attrPath), trail)...)
		}
		if diags.HasError() {
			return diags
		}
	}
	return diags
}

// valueAt returns the attribute at attrPath, or false when an object on the
// way is null or unknown: its attributes are then not configured at all.
func valueAt(obj types.Object, attrPath []string) (attr.Value, bool) {
	cur := obj
	for i, name := range attrPath {
		v, ok := cur.Attributes()[name]
		if !ok {
			return nil, false
		}
		if i == len(attrPath)-1 {
			return v, true
		}
		next, ok := v.(basetypes.ObjectValue)
		if !ok || next.IsNull() || next.IsUnknown() {
			return nil, false
		}
		cur = next
	}
	return nil, false
}

func attrPathOf(at path.Path, attrPath []string) path.Path {
	for _, name := range attrPath {
		at = at.AtName(name)
	}
	return at
}

func trailOf(trail string, keyPath []string) string {
	return joinKeyPath(trail, strings.Join(keyPath, "."))
}

func writes(f *Field, v attr.Value, merged bool) bool {
	if !f.Mode.hasKey() || f.ReadOnly || v.IsUnknown() {
		return false
	}
	return !v.IsNull() || f.NullClears && merged
}

func writeField(ctx context.Context, doc map[string]json.RawMessage, d docField, v attr.Value, merged bool, at path.Path, trail string) diag.Diagnostics {
	f := d.f
	if !writes(f, v, merged) {
		return nil
	}
	keyTrail := trailOf(trail, d.keyPath)
	if v.IsNull() {
		tflog.Debug(ctx, "Clearing Jellyfin key", map[string]any{"attribute": at.String(), "key": keyTrail})
		return put(ctx, doc, d.keyPath, json.RawMessage("null"))
	}
	var raw json.RawMessage
	var diags diag.Diagnostics
	switch {
	case f.Elem != nil && f.isList():
		raw, diags = writeList(ctx, doc, d, v, at, keyTrail)
	case f.Elem != nil:
		raw, diags = writeObject(ctx, doc, d, v, at, keyTrail)
	default:
		raw, diags = f.Codec.Encode(ctx, v)
	}
	if diags.HasError() {
		return diags
	}
	tflog.Debug(ctx, "Writing Jellyfin key", map[string]any{"attribute": at.String(), "key": keyTrail})
	return append(diags, put(ctx, doc, d.keyPath, raw)...)
}

func writeObject(ctx context.Context, doc map[string]json.RawMessage, d docField, v attr.Value, at path.Path, keyTrail string) (json.RawMessage, diag.Diagnostics) {
	obj, ok := v.(basetypes.ObjectValue)
	if !ok {
		return nil, wrongType(v, "object")
	}
	base := map[string]json.RawMessage{}
	if d.f.Document {
		served, err := servedObject(ctx, doc, d.keyPath, at)
		if err != nil {
			return nil, parseError(keyTrail, err)
		}
		base = served
	}
	diags := d.f.Elem.overlay(ctx, base, obj, d.f.Document, at, keyTrail)
	if diags.HasError() {
		return nil, diags
	}
	raw, d2 := marshal(base)
	return raw, append(diags, d2...)
}

func writeList(ctx context.Context, doc map[string]json.RawMessage, d docField, v attr.Value, at path.Path, keyTrail string) (json.RawMessage, diag.Diagnostics) {
	f := d.f
	list, ok := v.(basetypes.ListValue)
	if !ok {
		return nil, wrongType(v, "list")
	}
	var served []map[string]json.RawMessage
	if f.MergeKey != "" || f.CarryKey != "" {
		if raw, found := lookupPath(ctx, doc, d.keyPath, at); found && !isNull(raw) {
			if err := json.Unmarshal(raw, &served); err != nil {
				if f.MergeKey != "" {
					return nil, parseError(keyTrail, err)
				}
				// A carried key is only a courtesy: rebuilding without it
				// still writes every configured value.
				served = nil
			}
		}
	}
	var carried map[string]string
	if f.CarryKey != "" {
		carried = carriedValues(ctx, served, elemKey(f, f.CarryBy), f.CarryKey)
	}
	var diags diag.Diagnostics
	entries := make([]map[string]json.RawMessage, 0, len(list.Elements()))
	for i, e := range list.Elements() {
		elem, ok := e.(basetypes.ObjectValue)
		if !ok || elem.IsNull() || elem.IsUnknown() {
			return nil, diag.Diagnostics{diag.NewAttributeErrorDiagnostic(at.AtListIndex(i), "Unknown list element",
				"Every element of the list must be known before it is written to Jellyfin.")}
		}
		base := map[string]json.RawMessage{}
		if f.MergeKey != "" {
			if match := servedWithKey(ctx, served, elemKey(f, f.MergeKey), elem.Attributes()[f.MergeKey]); match != nil {
				maps.Copy(base, match)
			}
		}
		diags.Append(f.Elem.overlay(ctx, base, elem, f.MergeKey != "", at.AtListIndex(i), fmt.Sprintf("%s[%d]", keyTrail, i))...)
		if diags.HasError() {
			return nil, diags
		}
		if f.CarryKey != "" {
			if by, ok := elem.Attributes()[f.CarryBy].(basetypes.StringValue); ok && !by.IsNull() {
				if value, ok := carried[by.ValueString()]; ok {
					raw, d := marshal(value)
					diags.Append(d...)
					diags.Append(put(ctx, base, []string{f.CarryKey}, raw)...)
				}
			}
		}
		entries = append(entries, base)
	}
	raw, d2 := marshal(entries)
	return raw, append(diags, d2...)
}

func elemKey(f *Field, attrName string) string {
	for _, e := range f.Elem.Fields {
		if e.Name == attrName && len(e.KeyPath) == 1 {
			return e.KeyPath[0]
		}
	}
	return attrName
}

// servedWithKey returns the first served element whose key matches want,
// ignoring case, the way Jellyfin looks such elements up.
func servedWithKey(ctx context.Context, served []map[string]json.RawMessage, key string, want attr.Value) map[string]json.RawMessage {
	w, ok := want.(basetypes.StringValue)
	if !ok || w.IsNull() || w.IsUnknown() {
		return nil
	}
	for _, s := range served {
		if got := servedString(ctx, s, key); !got.IsNull() && strings.EqualFold(got.ValueString(), w.ValueString()) {
			return s
		}
	}
	return nil
}

func carriedValues(ctx context.Context, served []map[string]json.RawMessage, byKey, key string) map[string]string {
	out := map[string]string{}
	for _, s := range served {
		by, value := servedString(ctx, s, byKey), servedString(ctx, s, key)
		if !by.IsNull() && !value.IsNull() {
			out[by.ValueString()] = value.ValueString()
		}
	}
	return out
}

func servedString(ctx context.Context, m map[string]json.RawMessage, key string) types.String {
	raw, ok := lookupKey(ctx, m, key, path.Empty())
	if !ok || isNull(raw) {
		return types.StringNull()
	}
	v, _ := stringCodec{}.Decode(ctx, raw, nil, types.StringType)
	s, _ := v.(basetypes.StringValue)
	return s
}

func servedObject(ctx context.Context, doc map[string]json.RawMessage, keyPath []string, at path.Path) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	raw, found := lookupPath(ctx, doc, keyPath, at)
	if !found || isNull(raw) {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]json.RawMessage{}
	}
	return out, nil
}

func put(ctx context.Context, doc map[string]json.RawMessage, keyPath []string, raw json.RawMessage) diag.Diagnostics {
	if len(keyPath) == 0 {
		return diag.Diagnostics{diag.NewErrorDiagnostic("Missing Jellyfin key", "A value has no key to write to. This is a bug in the provider.")}
	}
	key := keyPath[0]
	if len(keyPath) > 1 {
		sub, err := servedObject(ctx, doc, keyPath[:1], path.Empty())
		if err != nil {
			return parseError(key, err)
		}
		if diags := put(ctx, sub, keyPath[1:], raw); diags.HasError() {
			return diags
		}
		var diags diag.Diagnostics
		raw, diags = marshal(sub)
		if diags.HasError() {
			return diags
		}
	}
	doc[key] = raw
	return nil
}

func parseError(keyTrail string, err error) diag.Diagnostics {
	return diag.Diagnostics{diag.NewErrorDiagnostic("Failed to parse Jellyfin configuration", fmt.Sprintf("%s: %s", keyTrail, err))}
}

func isNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
