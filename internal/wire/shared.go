// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// AvailableFunc returns the names the server offers for offered, the name a
// Complement option gives, and scope, the value of its scope attribute or "".
// An error wrapping ErrNotOffered says the server does not list them there.
type AvailableFunc func(ctx context.Context, offered, scope string) ([]string, error)

// ErrNotOffered is what an AvailableFunc wraps when the server does not list
// the names it offers for a scope: a read then takes the names the keys hold,
// and a write fails, as it cannot tell which names to disable.
var ErrNotOffered = errors.New("the server does not list the names it offers")

type availableKey struct{}

// WithAvailable returns ctx carrying available, which Complement attributes
// ask on the reads and writes that take ctx.
func WithAvailable(ctx context.Context, available AvailableFunc) context.Context {
	return context.WithValue(ctx, availableKey{}, available)
}

func (bb *binder) share(b *Binding, f *Field, opt *attrOption) {
	if f.Mode != ModeSent && f.Mode != ModeComplement {
		bb.errorf("%s is %s, so it takes no Orders option", f.Path, f.Mode)
		return
	}
	if !f.Type.Equal(stringListType) {
		bb.errorf("%s: Orders and Complement need a list of strings", f.Path)
		return
	}
	sibling := func(name, role string, want attr.Type) *Field {
		i := slices.IndexFunc(b.Fields, func(s *Field) bool { return s.Name == name })
		if i < 0 {
			bb.errorf("%s: %s %q is no attribute of the same object", f.Path, role, name)
			return nil
		}
		s := b.Fields[i]
		if s.Mode != ModeSent || s.ReadOnly || len(s.KeyPath) != 1 || !s.Type.Equal(want) {
			bb.errorf("%s: %s %s must be a configurable %s with a key of its own", f.Path, role, s.Path, want)
			return nil
		}
		return s
	}
	for i, name := range opt.shares {
		role := "the order attribute"
		if i == 1 {
			role = "the disabled attribute"
		}
		s := sibling(name, role, stringListType)
		if s == nil {
			continue
		}
		if _, ok := s.Codec.(stringListCodec); !ok {
			bb.errorf("%s: %s %s takes a codec of its own, which Orders and Complement cannot follow", f.Path, role, s.Path)
		}
		if by, taken := bb.shared[s]; taken {
			bb.errorf("%s and %s both write the key of %s", by, f.Path, s.Path)
		}
		bb.shared[s] = f.Path
		f.Shares = append(f.Shares, s)
		if f.Mode == ModeComplement && f.Since == "" {
			f.Since = s.Since
		}
	}
	if f.Mode != ModeComplement {
		return
	}
	if strings.TrimSpace(opt.offered) == "" {
		bb.errorf("%s: Complement needs the name of the offered list", f.Path)
	}
	f.Offered = opt.offered
	if opt.scope != "" {
		f.Scope = sibling(opt.scope, "the scope attribute", types.StringType)
	}
}

func (b *Binding) docOf(f *Field) (docField, bool) {
	for _, d := range b.docs {
		if d.f == f {
			return d, true
		}
	}
	return docField{}, false
}

func missingShare(f *Field) diag.Diagnostics {
	return diag.Diagnostics{diag.NewErrorDiagnostic("Missing Jellyfin key",
		fmt.Sprintf("%s shares a key with an attribute this binding does not cover. This is a bug in the provider.", f.Path))}
}

// writeShared writes the keys of d's shared attributes that those attributes
// leave to it, from v, the value of d in obj.
func (b *Binding) writeShared(ctx context.Context, doc map[string]json.RawMessage, obj types.Object, d docField, v attr.Value, merged bool, at path.Path, trail string) diag.Diagnostics {
	f := d.f
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	targets := make([]docField, len(f.Shares))
	left := false
	for i, s := range f.Shares {
		sd, ok := b.docOf(s)
		if !ok {
			return missingShare(f)
		}
		targets[i] = sd
		if sv, ok := valueAt(obj, sd.attrPath); !ok || !writes(s, sv, merged) {
			left = true
		} else {
			targets[i].f = nil
		}
	}
	if !left {
		return nil
	}
	names, diags := stringsOf(ctx, v)
	if diags.HasError() {
		return diags
	}

	var values [][]string
	if f.Mode == ModeComplement {
		scope, d := b.writtenScope(obj, f, at)
		if d.HasError() {
			return d
		}
		offered, d := b.offeredNames(ctx, f, scope, at, true)
		if d.HasError() {
			return d
		}
		for _, n := range names {
			if !slices.Contains(offered, n) {
				return diag.Diagnostics{notOffered(at, f, n, scope, offered)}
			}
		}
		order, disabled := complementOf(names, offered)
		values = [][]string{order, disabled}
	} else {
		values = [][]string{ordered(names, servedStrings(ctx, doc, targets[0].keyPath, at))}
	}

	for i, sd := range targets {
		if sd.f == nil {
			continue
		}
		raw, d := marshal(values[i])
		if d.HasError() {
			return d
		}
		tflog.Debug(ctx, "Writing Jellyfin key", map[string]any{"attribute": at.String(), "key": trailOf(trail, sd.keyPath)})
		if d := put(ctx, doc, sd.keyPath, raw); d.HasError() {
			return d
		}
	}
	return nil
}

func (b *Binding) writtenScope(obj types.Object, f *Field, at path.Path) (string, diag.Diagnostics) {
	if f.Scope == nil {
		return "", nil
	}
	sd, ok := b.docOf(f.Scope)
	if !ok {
		return "", missingShare(f)
	}
	sv, _ := valueAt(obj, sd.attrPath)
	s, ok := sv.(types.String)
	if !ok || s.IsNull() || s.IsUnknown() {
		return "", diag.Diagnostics{diag.NewAttributeErrorDiagnostic(at, "Missing "+f.Scope.Name,
			fmt.Sprintf("%s is written for the %s next to it, so set %s.", at, f.Scope.Name, f.Scope.Name))}
	}
	return s.ValueString(), nil
}

// offeredNames asks the context's AvailableFunc. A read takes a server that
// does not list the names as offering none, which a write cannot.
func (b *Binding) offeredNames(ctx context.Context, f *Field, scope string, at path.Path, write bool) ([]string, diag.Diagnostics) {
	available, ok := ctx.Value(availableKey{}).(AvailableFunc)
	if !ok || available == nil {
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("Missing offered names",
			fmt.Sprintf("%s needs the names the Jellyfin server offers, and the provider did not ask for them. This is a bug in the provider.", at))}
	}
	offered, err := available(ctx, f.Offered, scope)
	switch {
	case err == nil:
		return offered, nil
	case !errors.Is(err, ErrNotOffered):
		return nil, diag.Diagnostics{diag.NewAttributeErrorDiagnostic(at, "Failed to read the names Jellyfin offers", err.Error())}
	case write:
		var owners []string
		for _, s := range f.Shares {
			owners = append(owners, s.Name)
		}
		return nil, diag.Diagnostics{diag.NewAttributeErrorDiagnostic(at, "Offered names unknown",
			fmt.Sprintf("The Jellyfin server does not list the %s it offers%s, so %s cannot tell which to disable. Set %s instead.", f.Offered, forScope(scope), at, strings.Join(owners, " and ")))}
	}
	return nil, nil
}

func forScope(scope string) string {
	if scope == "" {
		return ""
	}
	return " for " + scope
}

func notOffered(at path.Path, f *Field, name, scope string, offered []string) diag.Diagnostic {
	list := "none"
	if len(offered) > 0 {
		quoted := make([]string, len(offered))
		for i, o := range offered {
			quoted[i] = strconv.Quote(o)
		}
		list = strings.Join(quoted, ", ")
	}
	return diag.NewAttributeErrorDiagnostic(at, "Name not offered by Jellyfin",
		fmt.Sprintf("%s lists %q, which is not one of the %s the Jellyfin server offers%s: %s. Jellyfin orders them by their exact name, so each name must match one of these exactly.", at, name, f.Offered, forScope(scope), list))
}

// readComplement reads what d's Complement writes, or null when the server
// serves neither of its keys.
func (b *Binding) readComplement(ctx context.Context, d docField, doc map[string]json.RawMessage, t attr.Type, at path.Path) (attr.Value, diag.Diagnostics) {
	f := d.f
	lists := make([][]string, len(f.Shares))
	found := false
	for i, s := range f.Shares {
		sd, ok := b.docOf(s)
		if !ok {
			return nullOf(ctx, t), missingShare(f)
		}
		raw, ok := lookupPath(ctx, doc, sd.keyPath, at)
		if !ok || isNull(raw) || json.Unmarshal(raw, &lists[i]) != nil {
			continue
		}
		found = true
	}
	if !found || len(lists) != 2 {
		return nullOf(ctx, t), nil
	}
	scope := ""
	if f.Scope != nil {
		sd, ok := b.docOf(f.Scope)
		if !ok {
			return nullOf(ctx, t), missingShare(f)
		}
		if raw, ok := lookupPath(ctx, doc, sd.keyPath, at); ok {
			_ = json.Unmarshal(raw, &scope)
		}
	}
	offered, diags := b.offeredNames(ctx, f, scope, at, false)
	if diags.HasError() {
		return nullOf(ctx, t), diags
	}
	return stringList(enabledOf(lists[0], lists[1], offered)), nil
}

func servedStrings(ctx context.Context, doc map[string]json.RawMessage, keyPath []string, at path.Path) []string {
	var out []string
	if raw, ok := lookupPath(ctx, doc, keyPath, at); ok && !isNull(raw) {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

// Jellyfin matches enabled and disabled names ignoring case.
func containsFold(names []string, name string) bool {
	return slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, name) })
}

// ordered returns names followed by the served names it leaves out, so the
// names keep their priority over every name the order held before.
func ordered(names, served []string) []string {
	out := append([]string{}, names...)
	for _, s := range served {
		if !containsFold(names, s) {
			out = append(out, s)
		}
	}
	return out
}

// complementOf returns the order and the disabled names that enable names in
// their order and disable every other offered name.
func complementOf(names, offered []string) (order, disabled []string) {
	disabled = []string{}
	for _, o := range offered {
		if !containsFold(names, o) {
			disabled = append(disabled, o)
		}
	}
	return append(append([]string{}, names...), disabled...), disabled
}

// enabledOf returns the names Jellyfin enables, in the order it asks them:
// the ordered names it does not disable, then the offered names that neither
// list holds, which it enables too and asks last.
func enabledOf(order, disabled, offered []string) []string {
	out := []string{}
	for _, o := range order {
		if !containsFold(disabled, o) {
			out = append(out, o)
		}
	}
	for _, o := range offered {
		if !containsFold(disabled, o) && !containsFold(order, o) {
			out = append(out, o)
		}
	}
	return out
}
