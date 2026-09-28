// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// node arranges a binding's fields by attribute path, so a read can rebuild
// the object types that hold them.
type node struct {
	field    *docField
	children map[string]*node
}

func trieOf(docs []docField) *node {
	root := &node{children: map[string]*node{}}
	for i := range docs {
		n := root
		for _, name := range docs[i].attrPath {
			child, ok := n.children[name]
			if !ok {
				child = &node{children: map[string]*node{}}
				n.children[name] = child
			}
			n = child
		}
		n.field = &docs[i]
	}
	return root
}

// FlattenInto reads raw, the document as the server serves it, into model, a
// pointer to the resource's model struct, whose values serve as the prior
// values Flatten takes.
func (b *Binding) FlattenInto(ctx context.Context, raw string, model any) diag.Diagnostics {
	prior, diags := types.ObjectValueFrom(ctx, b.AttrTypes, model)
	if diags.HasError() {
		return diags
	}
	got, d := b.flattenRaw(ctx, raw, prior)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	return append(diags, got.As(ctx, model, basetypes.ObjectAsOptions{})...)
}

// FlattenAfterApply is FlattenInto for the read that follows a write, with
// model holding the plan: it keeps the planned nulls KeepPlannedNulls
// describes and reports what Dropped finds, and fills model either way.
func (b *Binding) FlattenAfterApply(ctx context.Context, raw string, model any) diag.Diagnostics {
	planned, diags := types.ObjectValueFrom(ctx, b.AttrTypes, model)
	if diags.HasError() {
		return diags
	}
	got, d := b.flattenRaw(ctx, raw, planned)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	got = KeepPlannedNulls(planned, got)
	diags.Append(b.Dropped(planned, got)...)
	return append(diags, got.As(ctx, model, basetypes.ObjectAsOptions{})...)
}

func (b *Binding) flattenRaw(ctx context.Context, raw string, prior types.Object) (types.Object, diag.Diagnostics) {
	doc := map[string]json.RawMessage{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			return prior, diag.Diagnostics{diag.NewErrorDiagnostic(fmt.Sprintf("Failed to parse the Jellyfin %s", b.Object), err.Error())}
		}
	}
	return b.Flatten(ctx, doc, prior)
}

// Flatten reads doc into an object of the binding's type. An attribute whose
// key is missing, null or of the wrong type reads as null, unless
// ReadMissingAs gives another value, and one the binding does not read keeps
// its value in prior. A key missing under its golden spelling is looked up
// ignoring case and underscores, so a server that spells it otherwise still
// reads.
func (b *Binding) Flatten(ctx context.Context, doc map[string]json.RawMessage, prior types.Object) (types.Object, diag.Diagnostics) {
	return b.flattenNode(ctx, b.trie(), b.AttrTypes, prior, doc, path.Empty(), "")
}

func (b *Binding) trie() *node {
	if b.nodes == nil {
		return trieOf(b.docs)
	}
	return b.nodes
}

func (b *Binding) flattenNode(ctx context.Context, n *node, attrTypes map[string]attr.Type, prior types.Object, doc map[string]json.RawMessage, at path.Path, trail string) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	var priorAttrs map[string]attr.Value
	if !prior.IsNull() && !prior.IsUnknown() {
		priorAttrs = prior.Attributes()
	}
	out := make(map[string]attr.Value, len(attrTypes))
	for name, t := range attrTypes {
		pv, ok := priorAttrs[name]
		if !ok {
			pv = nullOf(ctx, t)
		}
		child := n.children[name]
		switch {
		case child == nil:
			out[name] = pv
		case child.field != nil:
			v, d := b.readField(ctx, *child.field, doc, pv, t, at.AtName(name), trail)
			diags.Append(d...)
			out[name] = v
		default:
			ot, ok := t.(basetypes.ObjectType)
			if !ok {
				diags.Append(wrongType(pv, "object")...)
				return prior, diags
			}
			po, ok := pv.(basetypes.ObjectValue)
			if !ok {
				po = types.ObjectNull(ot.AttrTypes)
			}
			v, d := b.flattenNode(ctx, child, ot.AttrTypes, po, doc, at.AtName(name), trail)
			diags.Append(d...)
			out[name] = v
		}
	}
	obj, d := types.ObjectValue(attrTypes, out)
	return obj, append(diags, d...)
}

func (b *Binding) readField(ctx context.Context, d docField, doc map[string]json.RawMessage, prior attr.Value, t attr.Type, at path.Path, trail string) (attr.Value, diag.Diagnostics) {
	f := d.f
	switch f.Mode {
	case ModeIdentity, ModeElsewhere:
		return prior, nil
	case ModeNeverSent:
		return nullOf(ctx, t), nil
	case ModeComplement:
		return b.readComplement(ctx, d, doc, prior, t, at)
	}
	keyTrail := trailOf(trail, d.keyPath)
	var diags diag.Diagnostics
	v := nullOf(ctx, t)
	raw, found := lookupPath(ctx, doc, d.keyPath, at)
	switch {
	case !found || isNull(raw):
	case f.Elem != nil && f.isList():
		v, diags = readList(ctx, f, raw, prior, t, at, keyTrail)
	case f.Elem != nil:
		v, diags = readObject(ctx, f, raw, prior, t, at, keyTrail)
	default:
		v, diags = f.Codec.Decode(ctx, raw, prior, t)
	}
	if v.IsNull() && f.ReadMissing != nil {
		v = f.ReadMissing
	}
	return v, diags
}

func readList(ctx context.Context, f *Field, raw json.RawMessage, prior attr.Value, t attr.Type, at path.Path, keyTrail string) (attr.Value, diag.Diagnostics) {
	lt, ok := t.(basetypes.ListType)
	if !ok {
		return nullOf(ctx, t), wrongType(prior, "list")
	}
	et, ok := lt.ElemType.(basetypes.ObjectType)
	if !ok {
		return nullOf(ctx, t), wrongType(prior, "list of objects")
	}
	var elems []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nullOf(ctx, t), parseError(keyTrail, err)
	}
	// The prior element at the same index is the one a codec compares with,
	// but only while the list keeps its length.
	var priorElems []attr.Value
	if pl, ok := prior.(basetypes.ListValue); ok && !pl.IsNull() && !pl.IsUnknown() && len(pl.Elements()) == len(elems) {
		priorElems = pl.Elements()
	}
	var diags diag.Diagnostics
	vals := make([]attr.Value, len(elems))
	for i, e := range elems {
		pe := types.ObjectNull(et.AttrTypes)
		if priorElems != nil {
			if po, ok := priorElems[i].(basetypes.ObjectValue); ok {
				pe = po
			}
		}
		o, d := f.Elem.flattenNode(ctx, f.Elem.trie(), et.AttrTypes, pe, e, at.AtListIndex(i), fmt.Sprintf("%s[%d]", keyTrail, i))
		diags.Append(d...)
		vals[i] = o
	}
	l, d := types.ListValue(et, vals)
	return l, append(diags, d...)
}

func readObject(ctx context.Context, f *Field, raw json.RawMessage, prior attr.Value, t attr.Type, at path.Path, keyTrail string) (attr.Value, diag.Diagnostics) {
	ot, ok := t.(basetypes.ObjectType)
	if !ok {
		return nullOf(ctx, t), wrongType(prior, "object")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nullOf(ctx, t), parseError(keyTrail, err)
	}
	po, ok := prior.(basetypes.ObjectValue)
	if !ok {
		po = types.ObjectNull(ot.AttrTypes)
	}
	return f.Elem.flattenNode(ctx, f.Elem.trie(), ot.AttrTypes, po, m, at, keyTrail)
}

func lookupPath(ctx context.Context, doc map[string]json.RawMessage, keyPath []string, at path.Path) (json.RawMessage, bool) {
	cur := doc
	for i, key := range keyPath {
		raw, ok := lookupKey(ctx, cur, key, at)
		if !ok {
			return nil, false
		}
		if i == len(keyPath)-1 {
			return raw, true
		}
		var next map[string]json.RawMessage
		if isNull(raw) || json.Unmarshal(raw, &next) != nil {
			return nil, false
		}
		cur = next
	}
	return nil, false
}

// lookupKey finds key as the golden spells it, else the one served key that
// matches it ignoring case and underscores.
func lookupKey(ctx context.Context, m map[string]json.RawMessage, key string, at path.Path) (json.RawMessage, bool) {
	if raw, ok := m[key]; ok {
		return raw, true
	}
	want := norm(key)
	var hits []string
	for served := range m {
		if norm(served) == want {
			hits = append(hits, served)
		}
	}
	switch len(hits) {
	case 0:
		return nil, false
	case 1:
		tflog.Debug(ctx, "Reading a Jellyfin key spelled otherwise than the golden", map[string]any{"attribute": at.String(), "key": key, "served": hits[0]})
		return m[hits[0]], true
	default:
		tflog.Debug(ctx, "Several served keys match a Jellyfin key; reading none", map[string]any{"attribute": at.String(), "key": key, "served": hits})
		return nil, false
	}
}

func nullOf(ctx context.Context, t attr.Type) attr.Value {
	v, err := t.ValueFromTerraform(ctx, tftypes.NewValue(t.TerraformType(ctx), nil))
	if err != nil {
		panic(fmt.Sprintf("no null value for %s: %v", t, err))
	}
	return v
}
