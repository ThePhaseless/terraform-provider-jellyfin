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
// Complement or Orders option gives, and scope, the value of its scope
// attribute or "". An error wrapping ErrNotOffered says the server does not
// list them there.
type AvailableFunc func(ctx context.Context, offered, scope string) ([]string, error)

// ErrNotOffered is what an AvailableFunc wraps when the server does not list
// the names it offers for a scope: a Complement then reads null and fails to
// write, as it cannot tell which names Jellyfin enables, while Orders orders
// the names without the offered spellings.
var ErrNotOffered = errors.New("the server does not list the names it offers")

type availableKey struct{}

// WithAvailable returns ctx carrying available, which Complement and Orders
// attributes ask on the reads and writes that take ctx.
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
		s := b.fieldNamed(name)
		if s == nil {
			bb.errorf("%s: %s %q is no attribute of the same object", f.Path, role, name)
			return nil
		}
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
	}
	switch {
	case f.Mode == ModeComplement && strings.TrimSpace(opt.offered) == "":
		bb.errorf("%s: Complement needs the name of the offered list", f.Path)
	case opt.offered == "" && opt.scope != "":
		bb.errorf("%s: the scope attribute %q scopes no offered list", f.Path, opt.scope)
		return
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
	if !known(v) {
		return nil
	}
	targets := make([]docField, len(f.Shares))
	leftToUs := make([]bool, len(f.Shares))
	for i, s := range f.Shares {
		sd, ok := b.docOf(s)
		if !ok {
			return missingShare(f)
		}
		sv, ok := valueAt(obj, sd.attrPath)
		targets[i], leftToUs[i] = sd, !ok || !writes(s, sv, merged)
	}
	if !slices.Contains(leftToUs, true) {
		return nil
	}
	names, diags := stringsOf(ctx, v)
	if diags.HasError() {
		return diags
	}

	scope, scoped, diags := b.scopeOf(obj, f)
	if diags.HasError() {
		return diags
	}
	var values [][]string
	if f.Mode == ModeComplement {
		if !scoped {
			return diag.Diagnostics{diag.NewAttributeErrorDiagnostic(at, "Missing "+f.Scope.Name,
				fmt.Sprintf("%s is written for the %s next to it, so set %s.", at, f.Scope.Name, f.Scope.Name))}
		}
		offered, listed, err := offeredNames(ctx, f, scope)
		if err != nil {
			return offeredNamesError(at, err)
		}
		if !listed {
			return diag.Diagnostics{notListed(at, f, scope)}
		}
		for _, n := range names {
			if !slices.Contains(offered, n) {
				return diag.Diagnostics{notOffered(at, f, n, scope, offered)}
			}
		}
		order, disabled := complementOf(names, offered)
		values = [][]string{order, disabled}
	} else {
		var offered []string
		if f.Offered != "" && scoped {
			var err error
			if offered, _, err = offeredNames(ctx, f, scope); err != nil {
				return offeredNamesError(at, err)
			}
		}
		served, _ := servedList(ctx, doc, targets[0].keyPath, at)
		values = [][]string{ordered(names, served, offered)}
	}

	for i, sd := range targets {
		if !leftToUs[i] {
			continue
		}
		raw, diags := marshal(values[i])
		if diags.HasError() {
			return diags
		}
		tflog.Debug(ctx, "Writing Jellyfin key", map[string]any{"attribute": at.String(), "key": trailOf(trail, sd.keyPath)})
		if diags := put(ctx, doc, sd.keyPath, raw); diags.HasError() {
			return diags
		}
	}
	return nil
}

// scopeOf returns the value of f's scope attribute in obj, or "" when f has
// none; scoped is false while that attribute is null, unknown or empty, as
// the server lists no names for an empty scope.
func (b *Binding) scopeOf(obj types.Object, f *Field) (scope string, scoped bool, diags diag.Diagnostics) {
	if f.Scope == nil {
		return "", true, nil
	}
	sd, ok := b.docOf(f.Scope)
	if !ok {
		return "", false, missingShare(f)
	}
	sv, _ := valueAt(obj, sd.attrPath)
	s, ok := sv.(types.String)
	if !ok || !known(s) || s.ValueString() == "" {
		return "", false, nil
	}
	return s.ValueString(), true, nil
}

// errNoAvailableFunc is what offeredNames returns when the context carries no
// AvailableFunc, which is a bug in the provider.
var errNoAvailableFunc = errors.New("the provider did not ask for the names the Jellyfin server offers")

// offeredNames asks the context's AvailableFunc; listed is false when the
// server does not list the names it offers for scope.
func offeredNames(ctx context.Context, f *Field, scope string) (offered []string, listed bool, err error) {
	available, ok := ctx.Value(availableKey{}).(AvailableFunc)
	if !ok || available == nil {
		return nil, false, errNoAvailableFunc
	}
	offered, err = available(ctx, f.Offered, scope)
	switch {
	case err == nil:
		return offered, true, nil
	case errors.Is(err, ErrNotOffered):
		return nil, false, nil
	}
	return nil, false, err
}

func offeredNamesError(at path.Path, err error) diag.Diagnostics {
	if errors.Is(err, errNoAvailableFunc) {
		return diag.Diagnostics{diag.NewErrorDiagnostic("Missing offered names",
			fmt.Sprintf("%s needs the names the Jellyfin server offers, and the provider did not ask for them. This is a bug in the provider.", at))}
	}
	return diag.Diagnostics{diag.NewAttributeErrorDiagnostic(at, "Failed to read the names Jellyfin offers", err.Error())}
}

func forScope(scope string) string {
	if scope == "" {
		return ""
	}
	return " for " + scope
}

func notListed(at path.Path, f *Field, scope string) diag.Diagnostic {
	var owners []string
	for _, s := range f.Shares {
		owners = append(owners, s.Name)
	}
	return diag.NewAttributeErrorDiagnostic(at, "Offered names unknown",
		fmt.Sprintf("The Jellyfin server does not list the %s it offers%s, so %s cannot tell which to disable. Set %s instead.", f.Offered, forScope(scope), at, strings.Join(owners, " and ")))
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
// serves neither of its keys or does not list the names it offers. When the
// server fails to say which names it offers, the value keeps prior with a
// warning, so that a refresh still reads every other attribute.
func (b *Binding) readComplement(ctx context.Context, d docField, doc map[string]json.RawMessage, prior attr.Value, t attr.Type, at path.Path) (attr.Value, diag.Diagnostics) {
	f := d.f
	lists := make([][]string, len(f.Shares))
	found := false
	for i, s := range f.Shares {
		sd, ok := b.docOf(s)
		if !ok {
			return nullOf(ctx, t), missingShare(f)
		}
		if names, ok := servedList(ctx, doc, sd.keyPath, at); ok {
			lists[i], found = names, true
		}
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
	offered, listed, err := offeredNames(ctx, f, scope)
	switch {
	case errors.Is(err, errNoAvailableFunc):
		return nullOf(ctx, t), offeredNamesError(at, err)
	case err != nil:
		// A value planned unknown has no previous value to keep, and a read
		// may not leave it unknown.
		kept := prior
		if kept.IsUnknown() {
			kept = nullOf(ctx, t)
		}
		return kept, diag.Diagnostics{diag.NewWarningDiagnostic("Failed to read the names Jellyfin offers",
			"The Jellyfin server did not say which providers it offers, so the lists of enabled providers keep their previous values: "+err.Error())}
	case !listed:
		return nullOf(ctx, t), nil
	}
	return stringList(enabledOf(lists[0], lists[1], offered)), nil
}

// servedList returns the list of strings the server serves at keyPath, or
// false when it serves none there.
func servedList(ctx context.Context, doc map[string]json.RawMessage, keyPath []string, at path.Path) ([]string, bool) {
	raw, ok := lookupPath(ctx, doc, keyPath, at)
	var names []string
	if !ok || isNull(raw) || json.Unmarshal(raw, &names) != nil {
		return nil, false
	}
	return names, true
}

// Jellyfin matches enabled and disabled names ignoring case, but ranks by the
// exact name.
func containsFold(names []string, name string) bool {
	return slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, name) })
}

// ordered returns names followed by the served names it leaves out, so the
// names keep their priority over every name the order held before. A served
// or offered name that matches one of names only ignoring case follows it, as
// it may be the spelling Jellyfin ranks the provider by.
func ordered(names, served, offered []string) []string {
	out := []string{}
	for _, n := range names {
		out = append(out, n)
		for _, s := range slices.Concat(served, offered) {
			if strings.EqualFold(s, n) && !slices.Contains(names, s) && !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	for _, s := range served {
		if !slices.Contains(out, s) {
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
// the ordered names it does not disable, then the offered names it does not
// disable that the order does not spell exactly, which it asks last.
func enabledOf(order, disabled, offered []string) []string {
	out := []string{}
	for _, o := range order {
		if !containsFold(disabled, o) {
			out = append(out, o)
		}
	}
	for _, o := range offered {
		if !containsFold(disabled, o) && !slices.Contains(order, o) {
			out = append(out, o)
		}
	}
	return out
}
