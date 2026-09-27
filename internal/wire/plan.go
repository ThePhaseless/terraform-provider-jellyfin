// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
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
	if planned.IsNull() || planned.IsUnknown() || got.IsNull() || got.IsUnknown() {
		return got
	}
	plannedAttrs := planned.Attributes()
	out := make(map[string]attr.Value, len(got.Attributes()))
	changed := false
	for name, gv := range got.Attributes() {
		out[name] = gv
		pv, ok := plannedAttrs[name]
		if !ok {
			continue
		}
		nv := gv
		switch g := gv.(type) {
		case basetypes.ListValue:
			if inElement && pv.IsNull() && !g.IsNull() && !g.IsUnknown() && len(g.Elements()) == 0 {
				nv = types.ListNull(g.ElementType(ctx))
			} else {
				nv = keepNullsInList(ctx, pv, g)
			}
		case basetypes.StringValue:
			if inElement && pv.IsNull() && !g.IsNull() && !g.IsUnknown() && g.ValueString() == "" {
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

func keepNullsInList(ctx context.Context, pv attr.Value, got basetypes.ListValue) basetypes.ListValue {
	planned, ok := pv.(basetypes.ListValue)
	if !ok || planned.IsNull() || planned.IsUnknown() || got.IsNull() || got.IsUnknown() || len(planned.Elements()) != len(got.Elements()) {
		return got
	}
	et, ok := got.ElementType(ctx).(basetypes.ObjectType)
	if !ok {
		return got
	}
	elems := make([]attr.Value, len(got.Elements()))
	for i, ge := range got.Elements() {
		elems[i] = ge
		g, ok := ge.(basetypes.ObjectValue)
		if !ok {
			continue
		}
		if p, ok := planned.Elements()[i].(basetypes.ObjectValue); ok {
			elems[i] = keepNulls(ctx, p, g, true)
		}
	}
	out, diags := types.ListValue(et, elems)
	if diags.HasError() {
		return got
	}
	return out
}

// Dropped reports each attribute planned with a value that the server read
// back as null after the write. Jellyfin drops the value of a setting it does
// not have, so this names the attribute where Terraform could only report an
// inconsistent result, which it cannot even name inside a sensitive object.
func (b *Binding) Dropped(planned, got types.Object) diag.Diagnostics {
	var diags diag.Diagnostics
	dropped(b.trie(), planned, got, path.Empty(), &diags)
	return diags
}

func dropped(n *node, planned, got basetypes.ObjectValue, at path.Path, diags *diag.Diagnostics) {
	if planned.IsNull() || planned.IsUnknown() || got.IsNull() || got.IsUnknown() {
		return
	}
	plannedAttrs, gotAttrs := planned.Attributes(), got.Attributes()
	for _, name := range sortedKeys(n.children) {
		child := n.children[name]
		pv, gv, p := plannedAttrs[name], gotAttrs[name], at.AtName(name)
		if pv == nil || pv.IsNull() || pv.IsUnknown() {
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
			diags.Append(droppedDiag(p, f))
			continue
		}
		if f.Elem == nil {
			continue
		}
		if po, ok := pv.(basetypes.ObjectValue); ok {
			if gov, ok := gv.(basetypes.ObjectValue); ok {
				dropped(f.Elem.trie(), po, gov, p, diags)
			}
			continue
		}
		pl, ok1 := pv.(basetypes.ListValue)
		gl, ok2 := gv.(basetypes.ListValue)
		if !ok1 || !ok2 || gl.IsUnknown() || len(pl.Elements()) != len(gl.Elements()) {
			continue
		}
		for i := range gl.Elements() {
			pe, ok1 := pl.Elements()[i].(basetypes.ObjectValue)
			ge, ok2 := gl.Elements()[i].(basetypes.ObjectValue)
			if ok1 && ok2 {
				dropped(f.Elem.trie(), pe, ge, p.AtListIndex(i), diags)
			}
		}
	}
}

func droppedDiag(p path.Path, f *Field) diag.Diagnostic {
	detail := fmt.Sprintf("The Jellyfin server read %s back as null after the write, so it did not keep the value. Jellyfin drops a value for a setting it does not have; check that the server's version has it.", p)
	if f.Since != "" {
		detail = fmt.Sprintf("The Jellyfin server read %s back as null after the write, so it did not keep the value. It needs Jellyfin %s or later; remove it from the configuration for older servers.", p, f.Since)
	}
	return diag.NewAttributeErrorDiagnostic(p, "Value not kept by Jellyfin", detail)
}

type gatedValue struct {
	p path.Path
	f *Field
}

// VersionGap is a configured value that the server's Jellyfin version lacks,
// as VersionErrors hands it to a VersionMessage.
type VersionGap struct {
	// Path leads to the value, with list indexes.
	Path path.Path
	Key  string
	// Since is set when the server is older than the field, Until when the
	// server is as new as the release that removed it.
	Since         string
	Until         string
	ServerVersion string
}

// VersionErrors rejects each configured value whose field the server's
// Jellyfin version lacks: a field with a Since version on older servers, and
// a Legacy field on its until version and later. It asks version for the
// server's version only when such a value is configured. It leaves out values
// unknown at plan time; Terraform plans again during apply, once they are
// known.
func (b *Binding) VersionErrors(ctx context.Context, cfg tfsdk.Config, version func() (string, error)) diag.Diagnostics {
	var diags diag.Diagnostics
	if cfg.Schema == nil || cfg.Raw.IsNull() {
		return diags
	}
	v, err := cfg.Schema.Type().ValueFromTerraform(ctx, cfg.Raw)
	if err != nil {
		diags.AddError("Failed to read the configuration", err.Error())
		return diags
	}
	obj, ok := v.(basetypes.ObjectValue)
	if !ok {
		return diags
	}
	var gated []gatedValue
	collectGated(b.trie(), obj, path.Empty(), &gated)
	if len(gated) == 0 {
		return diags
	}
	ver, err := version()
	if err != nil {
		diags.AddError("Failed to read the Jellyfin version", err.Error())
		return diags
	}
	if !hasLeadingDigit(ver) {
		return diags
	}
	for _, g := range gated {
		gap := VersionGap{Path: g.p, Key: g.f.key(), ServerVersion: ver}
		switch {
		case g.f.Since != "" && compareVersions(ver, g.f.Since) < 0:
			gap.Since = g.f.Since
		case g.f.Until != "" && compareVersions(ver, g.f.Until) >= 0:
			gap.Until = g.f.Until
		default:
			continue
		}
		message := g.f.VersionMessage
		if message == nil {
			message = g.f.genericVersionMessage
		}
		summary, detail := message(gap)
		diags.AddAttributeError(g.p, summary, detail)
	}
	return diags
}

func (f *Field) genericVersionMessage(g VersionGap) (summary, detail string) {
	if g.Since != "" {
		return "Unsupported Jellyfin server version",
			fmt.Sprintf("%s requires Jellyfin %s or later: the server runs Jellyfin %s, which has no %s field, so it would discard the value. Remove %s from the configuration or upgrade the server.", g.Path, g.Since, g.ServerVersion, g.Key, g.Path)
	}
	return "Unsupported Jellyfin server version",
		fmt.Sprintf("The server runs Jellyfin %s. %s Remove %s from the configuration.", g.ServerVersion, f.Reason, g.Path)
}

func collectGated(n *node, obj basetypes.ObjectValue, at path.Path, out *[]gatedValue) {
	if obj.IsNull() || obj.IsUnknown() {
		return
	}
	attrs := obj.Attributes()
	for _, name := range sortedKeys(n.children) {
		child, v, p := n.children[name], attrs[name], at.AtName(name)
		if v == nil || v.IsNull() || v.IsUnknown() {
			continue
		}
		if child.field == nil {
			if o, ok := v.(basetypes.ObjectValue); ok {
				collectGated(child, o, p, out)
			}
			continue
		}
		f := child.field.f
		if f.Since != "" || f.Until != "" {
			*out = append(*out, gatedValue{p: p, f: f})
			continue
		}
		if f.Elem == nil {
			continue
		}
		switch x := v.(type) {
		case basetypes.ObjectValue:
			collectGated(f.Elem.trie(), x, p, out)
		case basetypes.ListValue:
			for i, e := range x.Elements() {
				if o, ok := e.(basetypes.ObjectValue); ok {
					collectGated(f.Elem.trie(), o, p.AtListIndex(i), out)
				}
			}
		}
	}
}

func hasLeadingDigit(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && s[0] >= '0' && s[0] <= '9'
}

// compareVersions compares dotted versions segment by segment, each by its
// leading integer, with a missing segment counting as 0.
func compareVersions(a, b string) int {
	as, bs := strings.Split(strings.TrimSpace(a), "."), strings.Split(strings.TrimSpace(b), ".")
	for i := range max(len(as), len(bs)) {
		x, y := versionSegment(as, i), versionSegment(bs, i)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}
	return 0
}

func versionSegment(parts []string, i int) int64 {
	if i >= len(parts) {
		return 0
	}
	s := strings.TrimSpace(parts[i])
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, err := strconv.ParseInt(s[:end], 10, 64)
	if err != nil {
		return 0
	}
	return n
}
