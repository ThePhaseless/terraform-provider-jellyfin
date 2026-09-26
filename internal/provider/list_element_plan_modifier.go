// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// useStateForUnknownByKey returns a plan modifier for a list of objects that
// fills the attributes each planned element leaves unset in config from the
// prior element it matches. Key groups are tried in order: a planned element
// whose attributes in a group are all known and non-null matches the first
// unclaimed prior element with equal values in that group. A group is tried for
// every element before the next group, so an exact key claims its element before
// a looser one can. Unmatched elements keep their unknowns and take the server's
// values on apply.
//
// A value unknown in config stays unknown. While any key or element is unknown
// in config nothing is filled: once known at apply, that key could claim a
// different prior element than the one this plan copied from, and the final plan
// would contradict it. A value left unknown here may take any value then.
//
// It stands in for UseStateForUnknown on the nested attributes, which pairs
// elements by index: an inserted or reordered element would take the values of
// the element that held its index before, and an appended one would be planned
// null.
func useStateForUnknownByKey(keyGroups ...[]string) planmodifier.List {
	return useStateForUnknownByKeyModifier{keyGroups: keyGroups}
}

type useStateForUnknownByKeyModifier struct {
	keyGroups [][]string
}

func (m useStateForUnknownByKeyModifier) Description(_ context.Context) string {
	return "Attributes an element does not configure keep the value of the prior element with the same key."
}

func (m useStateForUnknownByKeyModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m useStateForUnknownByKeyModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() || req.PlanValue.IsNull() || req.PlanValue.IsUnknown() ||
		req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	objType, ok := req.PlanValue.ElementType(ctx).(types.ObjectType)
	if !ok {
		return
	}

	configured := knownObjects(req.ConfigValue.Elements())
	if len(configured) != len(req.PlanValue.Elements()) {
		return
	}
	for i, e := range req.ConfigValue.Elements() {
		if e.IsUnknown() || m.hasUnknownKey(configured[i]) {
			return
		}
	}

	planned := knownObjects(req.PlanValue.Elements())
	prior := knownObjects(req.StateValue.Elements())
	matches := make([]int, len(planned))
	for i := range matches {
		matches[i] = -1
	}
	claimed := make([]bool, len(prior))

	for _, group := range m.keyGroups {
		for i, p := range planned {
			if p == nil || matches[i] >= 0 || !hasKnownValues(p, group) {
				continue
			}
			for j, s := range prior {
				if s != nil && !claimed[j] && sameValues(p, s, group) {
					matches[i] = j
					claimed[j] = true
					break
				}
			}
		}
	}

	elements := req.PlanValue.Elements()
	changed := false
	for i, j := range matches {
		if j < 0 {
			continue
		}
		attrs := make(map[string]attr.Value, len(planned[i]))
		for name, v := range planned[i] {
			attrs[name] = v
			if cv, ok := configured[i][name]; ok && cv.IsNull() && v.IsUnknown() {
				if prev, ok := prior[j][name]; ok {
					attrs[name] = prev
					changed = true
				}
			}
		}
		obj, d := types.ObjectValue(objType.AttrTypes, attrs)
		resp.Diagnostics.Append(d...)
		if d.HasError() {
			return
		}
		elements[i] = obj
	}
	if !changed {
		return
	}

	list, d := types.ListValue(objType, elements)
	resp.Diagnostics.Append(d...)
	if d.HasError() {
		return
	}
	resp.PlanValue = list
}

// knownObjects returns the attributes of each element, or nil for an element
// that is not a known object.
func knownObjects(elements []attr.Value) []map[string]attr.Value {
	out := make([]map[string]attr.Value, len(elements))
	for i, e := range elements {
		obj, ok := e.(types.Object)
		if !ok || obj.IsNull() || obj.IsUnknown() {
			continue
		}
		out[i] = obj.Attributes()
	}
	return out
}

func (m useStateForUnknownByKeyModifier) hasUnknownKey(attrs map[string]attr.Value) bool {
	for _, group := range m.keyGroups {
		for _, name := range group {
			if v, ok := attrs[name]; ok && v.IsUnknown() {
				return true
			}
		}
	}
	return false
}

func hasKnownValues(attrs map[string]attr.Value, names []string) bool {
	for _, name := range names {
		v, ok := attrs[name]
		if !ok || v.IsNull() || v.IsUnknown() {
			return false
		}
	}
	return true
}

func sameValues(a, b map[string]attr.Value, names []string) bool {
	for _, name := range names {
		av, aok := a[name]
		bv, bok := b[name]
		if !aok || !bok || !av.Equal(bv) {
			return false
		}
	}
	return true
}
