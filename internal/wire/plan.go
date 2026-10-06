// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// KeepPlannedNulls returns got with each attribute inside a nested list
// element, at any depth, set back to null where planned has null and got an
// empty list or string. Terraform plans an attribute left unset inside an
// element from the prior state, where null is a known value, so reading it
// back empty would fail the apply as an inconsistent result.
func KeepPlannedNulls(planned, got types.Object) types.Object {
	return keepNulls(context.Background(), planned, got, false)
}

func keepNulls(ctx context.Context, planned, got basetypes.ObjectValue, inElement bool) basetypes.ObjectValue {
	if !known(planned) || !known(got) {
		return got
	}
	plannedAttrs := planned.Attributes()
	out := got.Attributes()
	changed := false
	for name, gv := range out {
		pv, ok := plannedAttrs[name]
		if !ok {
			continue
		}
		nv := gv
		switch g := gv.(type) {
		case basetypes.ListValue:
			if inElement && pv.IsNull() && known(g) && len(g.Elements()) == 0 {
				nv = types.ListNull(g.ElementType(ctx))
			} else {
				nv = zipElements(ctx, pv, g, func(pe, ge basetypes.ObjectValue) basetypes.ObjectValue {
					return keepNulls(ctx, pe, ge, true)
				})
			}
		case basetypes.StringValue:
			if inElement && pv.IsNull() && known(g) && g.ValueString() == "" {
				nv = types.StringNull()
			}
		case basetypes.ObjectValue:
			if p, ok := pv.(basetypes.ObjectValue); ok {
				nv = keepNulls(ctx, p, g, inElement)
			}
		}
		if !nv.Equal(gv) {
			out[name], changed = nv, true
		}
	}
	if !changed {
		return got
	}
	obj, diags := types.ObjectValue(got.AttributeTypes(ctx), out)
	if diags.HasError() {
		return got
	}
	return obj
}

// zipElements returns got unchanged unless planned is a list of the same
// length and both lists are known.
func zipElements(ctx context.Context, planned attr.Value, got basetypes.ListValue, keep func(p, g basetypes.ObjectValue) basetypes.ObjectValue) basetypes.ListValue {
	pl, ok := planned.(basetypes.ListValue)
	if !ok || !known(pl) || !known(got) || len(pl.Elements()) != len(got.Elements()) {
		return got
	}
	plannedElems, elems := pl.Elements(), got.Elements()
	for i, ge := range elems {
		g, ok1 := ge.(basetypes.ObjectValue)
		p, ok2 := plannedElems[i].(basetypes.ObjectValue)
		if ok1 && ok2 {
			elems[i] = keep(p, g)
		}
	}
	out, diags := types.ListValue(got.ElementType(ctx), elems)
	if diags.HasError() {
		return got
	}
	return out
}

// keepUnwrittenComplements returns got with each Complement that the write
// left alone set back to its planned value.
func keepUnwrittenComplements(ctx context.Context, n *node, planned, got basetypes.ObjectValue) basetypes.ObjectValue {
	if !known(planned) || !known(got) {
		return got
	}
	plannedAttrs := planned.Attributes()
	out := got.Attributes()
	changed := false
	for name, child := range n.children {
		pv, gv := plannedAttrs[name], out[name]
		if pv == nil || gv == nil {
			continue
		}
		nv := gv
		switch {
		case child.field == nil:
			if p, ok := pv.(basetypes.ObjectValue); ok {
				if g, ok := gv.(basetypes.ObjectValue); ok {
					nv = keepUnwrittenComplements(ctx, child, p, g)
				}
			}
		case child.field.f.Mode == ModeComplement:
			if !pv.IsUnknown() && !complementWrites(child.field.f, plannedAttrs) {
				nv = pv
			}
		case child.field.f.Elem != nil:
			nv = keepUnwrittenInElements(ctx, child.field.f.Elem.nodes, pv, gv)
		}
		if !nv.Equal(gv) {
			out[name], changed = nv, true
		}
	}
	if !changed {
		return got
	}
	obj, diags := types.ObjectValue(got.AttributeTypes(ctx), out)
	if diags.HasError() {
		return got
	}
	return obj
}

func keepUnwrittenInElements(ctx context.Context, n *node, pv, gv attr.Value) attr.Value {
	switch g := gv.(type) {
	case basetypes.ObjectValue:
		if p, ok := pv.(basetypes.ObjectValue); ok {
			return keepUnwrittenComplements(ctx, n, p, g)
		}
	case basetypes.ListValue:
		return zipElements(ctx, pv, g, func(pe, ge basetypes.ObjectValue) basetypes.ObjectValue {
			return keepUnwrittenComplements(ctx, n, pe, ge)
		})
	}
	return gv
}

// complementWrites reports whether writeShared writes f's keys: f has a value
// and a sharer has none.
func complementWrites(f *Field, attrs map[string]attr.Value) bool {
	if !known(attrs[f.Name]) {
		return false
	}
	return slices.ContainsFunc(f.Shares, func(s *Field) bool { return !known(attrs[s.Name]) })
}

// Dropped reports each attribute planned with a value that the server read
// back as null after the write. Jellyfin drops the value of a setting it does
// not have, so this names the attribute where Terraform could only report an
// inconsistent result, which it cannot even name inside a sensitive object.
func (b *Binding) Dropped(planned, got types.Object) diag.Diagnostics {
	var diags diag.Diagnostics
	dropped(b.nodes, planned, got, path.Empty(), &diags)
	return diags
}

func dropped(n *node, planned, got basetypes.ObjectValue, at path.Path, diags *diag.Diagnostics) {
	if !known(planned) || !known(got) {
		return
	}
	plannedAttrs, gotAttrs := planned.Attributes(), got.Attributes()
	for _, name := range slices.Sorted(maps.Keys(n.children)) {
		child := n.children[name]
		pv, gv, p := plannedAttrs[name], gotAttrs[name], at.AtName(name)
		if !known(pv) {
			continue
		}
		if child.field == nil {
			po, ok1 := pv.(basetypes.ObjectValue)
			gov, ok2 := gv.(basetypes.ObjectValue)
			if ok1 && ok2 {
				dropped(child, po, gov, p, diags)
			}
			continue
		}
		f := child.field.f
		if !f.Mode.hasKey() || f.ReadOnly {
			continue
		}
		if gv == nil || gv.IsNull() {
			diags.Append(droppedDiag(p))
			continue
		}
		if f.Elem == nil {
			continue
		}
		if po, ok := pv.(basetypes.ObjectValue); ok {
			if gov, ok := gv.(basetypes.ObjectValue); ok {
				dropped(f.Elem.nodes, po, gov, p, diags)
			}
			continue
		}
		pl, ok1 := pv.(basetypes.ListValue)
		gl, ok2 := gv.(basetypes.ListValue)
		if !ok1 || !ok2 || gl.IsUnknown() || len(pl.Elements()) != len(gl.Elements()) {
			continue
		}
		plannedElems := pl.Elements()
		for i, e := range gl.Elements() {
			pe, ok1 := plannedElems[i].(basetypes.ObjectValue)
			ge, ok2 := e.(basetypes.ObjectValue)
			if ok1 && ok2 {
				dropped(f.Elem.nodes, pe, ge, p.AtListIndex(i), diags)
			}
		}
	}
}

func droppedDiag(p path.Path) diag.Diagnostic {
	notKept := fmt.Sprintf("The Jellyfin server read %s back as null after the write, so it did not keep the value.", p)
	hint := "Jellyfin drops a value for a setting it does not have; check that the server runs the Jellyfin version this provider supports."
	return diag.NewAttributeErrorDiagnostic(p, "Value not kept by Jellyfin", notKept+" "+hint)
}
