// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"slices"

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
// A list of objects that a matched element configures is filled the same way,
// with the values each of its elements configures as the key.
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

	configElements, elements := req.ConfigValue.Elements(), req.PlanValue.Elements()
	if len(configElements) != len(elements) {
		return
	}
	configured := knownObjects(configElements)
	for i, e := range configElements {
		if e.IsUnknown() || m.hasUnknownKey(configured[i]) {
			return
		}
	}

	planned := knownObjects(elements)
	prior := knownObjects(req.StateValue.Elements())
	matches := slices.Repeat([]int{-1}, len(planned))
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

	changed := false
	for i, j := range matches {
		if j < 0 {
			continue
		}
		attrs, filled := fillUnset(ctx, configured[i], planned[i], prior[j])
		if !filled {
			continue
		}
		obj, d := types.ObjectValue(objType.AttrTypes, attrs)
		resp.Diagnostics.Append(d...)
		if d.HasError() {
			return
		}
		elements[i], changed = obj, true
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

// fillUnset returns planned with each attribute that configured leaves unset
// and planned leaves unknown taken from prior, and reports whether it took any.
// A list of objects that configured sets is filled element by element.
func fillUnset(ctx context.Context, configured, planned, prior map[string]attr.Value) (map[string]attr.Value, bool) {
	attrs := make(map[string]attr.Value, len(planned))
	filled := false
	for name, v := range planned {
		attrs[name] = v
		cv, ok := configured[name]
		if !ok {
			continue
		}
		prev, ok := prior[name]
		if !ok {
			continue
		}
		if cv.IsNull() && v.IsUnknown() {
			attrs[name], filled = prev, true
			continue
		}
		if list, ok := fillUnsetInList(ctx, cv, v, prev); ok {
			attrs[name], filled = list, true
		}
	}
	return attrs, filled
}

// fillUnsetInList fills a list of objects as useStateForUnknownByKey fills its
// own, with the values each element configures as its key: an element takes
// the unset values of an unclaimed prior element with the same configured
// values, preferring the one at its own index, so a reordered element keeps
// its own values. An element that configures nothing is left as planned, and
// nothing is filled while any configured value is unknown, as that value could
// pair the element with another prior element on apply.
func fillUnsetInList(ctx context.Context, configured, planned, prior attr.Value) (attr.Value, bool) {
	c, ok1 := configured.(types.List)
	p, ok2 := planned.(types.List)
	s, ok3 := prior.(types.List)
	if !ok1 || !ok2 || !ok3 || p.IsNull() || p.IsUnknown() || s.IsNull() || s.IsUnknown() {
		return nil, false
	}
	objType, ok := p.ElementType(ctx).(types.ObjectType)
	if !ok || len(c.Elements()) != len(p.Elements()) || !fullyKnown(ctx, c) {
		return nil, false
	}
	cs, ps, ss := knownObjects(c.Elements()), knownObjects(p.Elements()), knownObjects(s.Elements())
	claimed := make([]bool, len(ss))
	elements := p.Elements()
	filled := false
	for k := range elements {
		key := nonNullNames(cs[k])
		if ps[k] == nil || len(key) == 0 {
			continue
		}
		j := unclaimedMatch(cs[k], k, ss, claimed, key)
		if j < 0 {
			continue
		}
		claimed[j] = true
		attrs, ok := fillUnset(ctx, cs[k], ps[k], ss[j])
		if !ok {
			continue
		}
		obj, d := types.ObjectValue(objType.AttrTypes, attrs)
		if d.HasError() {
			return nil, false
		}
		elements[k], filled = obj, true
	}
	if !filled {
		return nil, false
	}
	list, d := types.ListValue(objType, elements)
	if d.HasError() {
		return nil, false
	}
	return list, true
}

func fullyKnown(ctx context.Context, v attr.Value) bool {
	tv, err := v.ToTerraformValue(ctx)
	return err == nil && tv.IsFullyKnown()
}

func nonNullNames(attrs map[string]attr.Value) []string {
	var names []string
	for name, v := range attrs {
		if !v.IsNull() {
			names = append(names, name)
		}
	}
	return names
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
