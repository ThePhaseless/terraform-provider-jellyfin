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
// whose attributes in a group are all known and non-null matches an unclaimed
// prior element with equal values in that group, the one at its own index if
// that qualifies, else the first. A group is tried for every element before the
// next group, so an exact key claims its element before a looser one can.
// Unmatched elements keep their unknowns and take the server's values on apply.
//
// The final plan must not contradict this one. A value unknown in config stays
// unknown, and while any key or element is unknown in config nothing is filled:
// once known at apply, that key could claim a different prior element than the
// one this plan copied from. The final plan can also skip this modifier: when
// the resolved config leaves every element as it was, the framework marks
// nothing unknown and keeps Terraform's values, which pair elements by index.
// Every configured key then equals the prior one at its index, so preferring
// that element makes this plan pair them by index as well, even where two prior
// elements share a key.
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
			if j := unclaimedMatch(p, i, prior, claimed, group); j >= 0 {
				matches[i] = j
				claimed[j] = true
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

// unclaimedMatch returns the index of an unclaimed prior element with the same
// values in group as attrs, preferring index i, or -1 if there is none.
func unclaimedMatch(attrs map[string]attr.Value, i int, prior []map[string]attr.Value, claimed []bool, group []string) int {
	available := func(j int) bool {
		return prior[j] != nil && !claimed[j] && sameValues(attrs, prior[j], group)
	}
	if i < len(prior) && available(i) {
		return i
	}
	for j := range prior {
		if available(j) {
			return j
		}
	}
	return -1
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
