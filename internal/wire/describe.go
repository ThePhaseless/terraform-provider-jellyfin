// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"fmt"
	"strings"
)

// Describe lists, one line each, every attribute with the key it maps to and
// what it derived or was declared, then each object's keys no attribute
// claims, which a merged object keeps as served and a rebuilt one omits.
func (b *Binding) Describe() []string {
	var out []string
	var walk func(*Binding)
	walk = func(x *Binding) {
		for _, f := range x.Fields {
			out = append(out, describeField(x, f))
			if f.Elem != nil {
				walk(f.Elem)
			}
		}
	}
	walk(b)
	for _, inst := range b.instances {
		if len(inst.unclaimed) == 0 {
			continue
		}
		verb := "keeps"
		if inst.rebuilt {
			verb = "omits"
		}
		where := inst.keyPath
		if where == "" {
			where = "(document)"
		}
		out = append(out, fmt.Sprintf("%s %s %s %s", where, inst.object, verb, strings.Join(inst.unclaimed, ", ")))
	}
	return out
}

func describeField(x *Binding, f *Field) string {
	switch f.Mode {
	case ModeIdentity, ModeNeverSent, ModeElsewhere:
		return fmt.Sprintf("%s -> %s", f.Path, f.Mode)
	case ModeComplement:
		keys := make([]string, len(f.Shares))
		for i, s := range f.Shares {
			keys[i] = s.key()
		}
		line := fmt.Sprintf("%s -> %s.%s complement-of=%s", f.Path, x.Object, strings.Join(keys, "+"), f.Offered)
		if f.Scope != nil {
			line += "/" + f.Scope.Name
		}
		if f.Since != "" {
			line += " since=" + f.Since
		}
		return line
	}
	line := fmt.Sprintf("%s -> %s.%s", f.Path, x.Object, strings.Join(f.KeyPath, "."))
	var flags []string
	if f.Mode == ModeLegacy {
		flags = append(flags, "legacy until="+f.Until)
	} else {
		flags = append(flags, f.Prop.Sig)
	}
	if f.Codec != nil {
		if name := codecName(f.Codec); name != "" {
			flags = append(flags, "codec="+name)
		}
	}
	if f.ReadOnly {
		flags = append(flags, "read-only")
	}
	if f.NullClears {
		flags = append(flags, "null-clears")
	}
	if f.ReadMissing != nil {
		flags = append(flags, "read-missing-as="+f.ReadMissing.String())
	}
	if f.Since != "" {
		flags = append(flags, "since="+f.Since)
	}
	if f.MergeKey != "" {
		flags = append(flags, "merge-by="+f.MergeKey)
	}
	if f.CarryKey != "" {
		flags = append(flags, fmt.Sprintf("carries=%s/%s", f.CarryKey, f.CarryBy))
	}
	for _, s := range f.Shares {
		flags = append(flags, "orders="+s.key())
	}
	if f.Offered != "" {
		spelt := "spelt-as=" + f.Offered
		if f.Scope != nil {
			spelt += "/" + f.Scope.Name
		}
		flags = append(flags, spelt)
	}
	if f.Document {
		flags = append(flags, "document")
	}
	return line + " " + strings.Join(flags, " ")
}
