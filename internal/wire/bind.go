// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type Mode int

const (
	ModeSent Mode = iota
	ModeIdentity
	ModeNeverSent
	ModeElsewhere
	ModeLegacy
)

func (m Mode) String() string {
	switch m {
	case ModeSent:
		return "sent"
	case ModeIdentity:
		return "identity"
	case ModeNeverSent:
		return "never sent"
	case ModeElsewhere:
		return "elsewhere"
	case ModeLegacy:
		return "legacy"
	}
	return fmt.Sprintf("Mode(%d)", int(m))
}

func (m Mode) hasKey() bool { return m == ModeSent || m == ModeLegacy }

type Field struct {
	// Path is the dotted attribute path from the resource root.
	Path   string
	Name   string
	Type   attr.Type
	Mode   Mode
	Reason string
	// KeyPath leads from the enclosing binding's object to the key, and is
	// empty for a mode without a key. Object holds its last key.
	KeyPath []string
	Object  string
	Prop    Prop
	Codec   Codec
	// NullClears comes from Optional && !Computed: a null value then clears
	// the server's, as JSON null in a merged object and by leaving the key
	// out of a rebuilt one.
	NullClears bool
	// ReadOnly comes from Computed && !Optional: the server owns the value,
	// so it is read but never written.
	ReadOnly    bool
	Since       string
	Until       string
	ReadMissing attr.Value
	// Elem binds the object of a nested attribute: each list element, or the
	// single object.
	Elem     *Binding
	MergeKey string
	CarryKey string
	CarryBy  string
	Document bool
}

func (f *Field) key() string { return f.KeyPath[len(f.KeyPath)-1] }

func (f *Field) isList() bool {
	_, ok := f.Type.(types.ListType)
	return ok
}

type Binding struct {
	Object    string
	AttrTypes map[string]attr.Type
	Fields    []*Field

	docs      []docField
	nodes     *node
	documents map[string]bool
	instances []*instance
}

// docField places a field in a document: attrPath leads to its value from the
// binding's object type, keyPath to its key from the document's root.
type docField struct {
	attrPath []string
	keyPath  []string
	f        *Field
}

// instance is one JSON object a binding writes: the document, a nested object
// or the elements of a nested list, named by its key path from the document.
type instance struct {
	keyPath    string
	object     string
	rebuilt    bool
	viaKeyPath bool
	claimed    map[string]string
	unmanaged  map[string]string
	unclaimed  []string
}

// Bind maps every attribute of s to a key of the golden object root. An
// attribute resolves to the one property whose key equals its name when case
// and underscores are ignored, unless an option says otherwise; Bind fails,
// naming every attribute it cannot map, rather than guess.
func Bind(s schema.Schema, root string, opts ...Option) (*Binding, error) {
	return embedded().bind(s, root, opts...)
}

type binder struct {
	c         *catalog
	o         *options
	errs      []string
	instances map[string]*instance
	order     []string
}

func (c *catalog) bind(s schema.Schema, root string, opts ...Option) (*Binding, error) {
	o := &options{attrs: map[string]*attrOption{}, unmanaged: map[string]map[string]string{}, documents: map[string]bool{}}
	for _, opt := range opts {
		opt(o)
	}
	if _, ok := c.pinned[root]; !ok {
		return nil, fmt.Errorf("the goldens have no schema %q", root)
	}
	bb := &binder{c: c, o: o, errs: slices.Clone(o.errs), instances: map[string]*instance{}}
	b := bb.object(root, "", s.Attributes, false, "", "")
	bb.check()
	if len(bb.errs) > 0 {
		sort.Strings(bb.errs)
		return nil, errors.New(strings.Join(bb.errs, "\n"))
	}
	b.documents = o.documents
	for _, k := range bb.order {
		b.instances = append(b.instances, bb.instances[k])
	}
	return b, nil
}

func (bb *binder) errorf(format string, args ...any) {
	bb.errs = append(bb.errs, fmt.Sprintf(format, args...))
}

func (bb *binder) instance(keyPath, object string, rebuilt, viaKeyPath bool) *instance {
	inst, ok := bb.instances[keyPath]
	if !ok {
		inst = &instance{keyPath: keyPath, object: object, rebuilt: rebuilt, viaKeyPath: viaKeyPath, claimed: map[string]string{}, unmanaged: map[string]string{}}
		bb.instances[keyPath] = inst
		bb.order = append(bb.order, keyPath)
		return inst
	}
	if inst.object != object {
		bb.errorf("%s is bound as both %s and %s", displayKeyPath(keyPath), inst.object, object)
	}
	if rebuilt && inst.viaKeyPath || viaKeyPath && inst.rebuilt {
		bb.errorf("a key path writes into %s, which a nested attribute rebuilds, so the write would be lost", displayKeyPath(keyPath))
	}
	return inst
}

func (bb *binder) claim(keyPath, object, key, attrPath string, viaKeyPath bool) {
	inst := bb.instance(keyPath, object, false, viaKeyPath)
	if prev, ok := inst.claimed[key]; ok {
		bb.errorf("%s and %s both map to %s.%s", prev, attrPath, object, key)
		return
	}
	inst.claimed[key] = attrPath
}

func joinKeyPath(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ".")
}

func displayKeyPath(kp string) string {
	if kp == "" {
		return "the document"
	}
	return kp
}

func (bb *binder) object(object, prefix string, attrs map[string]schema.Attribute, rebuilt bool, inst, since string) *Binding {
	b := &Binding{Object: object, AttrTypes: map[string]attr.Type{}}
	bb.instance(inst, object, rebuilt, false)
	names := make([]string, 0, len(attrs))
	for n := range attrs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		a := attrs[name]
		f := bb.field(object, joinKeyPath(prefix, name), name, a, inst, since)
		b.Fields = append(b.Fields, f)
		b.AttrTypes[name] = a.GetType()
		b.docs = append(b.docs, docField{attrPath: []string{name}, keyPath: f.KeyPath, f: f})
	}
	sortDocs(b.docs)
	b.nodes = trieOf(b.docs)
	return b
}

func sortDocs(docs []docField) {
	sort.SliceStable(docs, func(i, j int) bool {
		if len(docs[i].keyPath) != len(docs[j].keyPath) {
			return len(docs[i].keyPath) < len(docs[j].keyPath)
		}
		return strings.Join(docs[i].attrPath, ".") < strings.Join(docs[j].attrPath, ".")
	})
}

func (bb *binder) field(object, path, name string, a schema.Attribute, inst, since string) *Field {
	opt := bb.o.attrs[path]
	if opt == nil {
		opt = &attrOption{}
	} else {
		opt.used = true
	}
	f := &Field{
		Path:        path,
		Name:        name,
		Type:        a.GetType(),
		Mode:        opt.mode,
		Reason:      opt.reason,
		NullClears:  a.IsOptional() && !a.IsComputed(),
		ReadOnly:    a.IsComputed() && !a.IsOptional() && !a.IsRequired(),
		ReadMissing: opt.readMissing,
	}
	if opt.readMissing != nil && !opt.readMissing.Type(context.Background()).Equal(f.Type) {
		bb.errorf("%s: ReadMissingAs gives a %s, but the attribute is a %s", path, opt.readMissing.Type(context.Background()), f.Type)
	}
	if !f.Mode.hasKey() {
		if opt.key != "" || opt.codec != nil || opt.mergeKey != "" || opt.carryKey != "" || opt.readMissing != nil {
			bb.errorf("%s is %s, so it takes no key, codec, list or read option", path, f.Mode)
		}
		return f
	}

	var err error
	switch {
	case f.Mode == ModeLegacy:
		f.KeyPath, f.Object = []string{opt.key}, object
		if _, ok := bb.c.pinned[object][opt.key]; ok {
			bb.errorf("%s: the pinned golden has %s.%s, so map it with Key instead of Legacy", path, object, opt.key)
		}
		f.Prop = bb.c.floor[object][opt.key]
		f.Until = opt.until
		if !hasLeadingDigit(opt.until) {
			bb.errorf("%s: Legacy needs the version that removed the key, not %q", path, opt.until)
		}
	case opt.key != "":
		f.KeyPath = strings.Split(opt.key, ".")
		f.Prop, f.Object, err = bb.c.pinned.lookup(object, f.KeyPath)
		if err != nil {
			bb.errorf("%s: %v", path, err)
			return f
		}
		if _, inverted := opt.codec.(boolCodec); !inverted && len(f.KeyPath) == 1 {
			if derived, err := bb.c.pinned.Resolve(object, name); err == nil && derived.Key == f.KeyPath[0] {
				bb.errorf("%s: Key(%q) is what the name resolves to anyway; drop it", path, opt.key)
			}
		}
	default:
		f.Prop, err = bb.c.pinned.Resolve(object, name)
		if err != nil {
			if fp, ferr := bb.c.floor.Resolve(object, name); ferr == nil {
				bb.errorf("%s: %v; the floor golden (Jellyfin %s) has %s.%s, so Jellyfin removed or renamed it: declare it Legacy or map it with Key", path, err, bb.c.floorVer, object, fp.Key)
			} else {
				bb.errorf("%s: %v", path, err)
			}
			return f
		}
		f.KeyPath, f.Object = []string{f.Prop.Key}, object
	}
	container := joinKeyPath(inst, strings.Join(f.KeyPath[:len(f.KeyPath)-1], "."))
	bb.claim(container, f.Object, f.key(), path, len(f.KeyPath) > 1)

	if f.Mode == ModeSent && since == "" && !bb.c.unversioned[f.Object] {
		switch floorProps, ok := bb.c.floor[f.Object]; {
		case !ok:
			bb.errorf("%s: the floor golden has no schema %s; regenerate it (see schema_guard_test.go)", path, f.Object)
		case !hasProp(floorProps, f.key()):
			f.Since = bb.c.sinceVer
		}
	}
	nestedSince := since
	if nestedSince == "" {
		nestedSince = f.Since
	}

	if _, nested := a.(schema.NestedAttribute); nested && opt.codec != nil {
		bb.errorf("%s is a nested attribute, whose own attributes take their codecs, so it takes no Inverted, Delimited or WithCodec; ReadMissingAs gives it a read default", path)
	}
	switch na := a.(type) {
	case schema.ListNestedAttribute:
		if !f.Prop.List || f.Prop.Ref == "" {
			bb.errorf("%s is a list of objects, but %s.%s is %s", path, f.Object, f.key(), f.Prop.Sig)
			return f
		}
		if opt.mergeKey != "" && opt.carryKey != "" {
			bb.errorf("%s: an element is either merged or rebuilt, so it cannot take both MergeByKey and CarryServed", path)
		}
		elemInst := joinKeyPath(inst, strings.Join(f.KeyPath, ".")) + "[]"
		f.Elem = bb.object(f.Prop.Ref, path, na.NestedObject.Attributes, opt.mergeKey == "", elemInst, nestedSince)
		f.MergeKey, f.CarryKey, f.CarryBy = opt.mergeKey, opt.carryKey, opt.carryBy
		bb.checkListOptions(f, elemInst)
	case schema.SingleNestedAttribute:
		if f.Prop.List || f.Prop.Ref == "" {
			bb.errorf("%s is an object, but %s.%s is %s", path, f.Object, f.key(), f.Prop.Sig)
			return f
		}
		if opt.mergeKey != "" || opt.carryKey != "" {
			bb.errorf("%s: MergeByKey and CarryServed apply to lists of objects only", path)
		}
		kp := joinKeyPath(inst, strings.Join(f.KeyPath, "."))
		f.Document = inst == "" && bb.o.documents[kp]
		f.Elem = bb.object(f.Prop.Ref, path, na.Attributes, !f.Document, kp, nestedSince)
	case schema.NestedAttribute:
		bb.errorf("%s: %T is not supported", path, a)
	default:
		if opt.mergeKey != "" || opt.carryKey != "" {
			bb.errorf("%s: MergeByKey and CarryServed apply to lists of objects only", path)
		}
		f.Codec = opt.codec
		if f.Codec == nil {
			if f.Codec, err = defaultCodec(f.Type, f.Prop, f.Mode == ModeLegacy); err != nil {
				bb.errorf("%s -> %s.%s: %v", path, f.Object, f.key(), err)
			}
		} else if msg := checkCodec(f); msg != "" {
			bb.errorf("%s -> %s.%s: %s", path, f.Object, f.key(), msg)
		}
	}
	return f
}

func hasProp(props map[string]Prop, key string) bool {
	_, ok := props[key]
	return ok
}

func checkCodec(f *Field) string {
	switch c := f.Codec.(type) {
	case boolCodec:
		if c.inverted && (!f.Type.Equal(boolType) || f.Prop.List || f.Prop.Scalar != "boolean") {
			return "Inverted needs a bool attribute and a boolean key"
		}
	case delimitedCodec:
		if !f.Type.Equal(stringListType) || f.Prop.List || f.Prop.Scalar != "string" {
			return "Delimited needs a list of strings and a string key"
		}
	}
	return ""
}

func (bb *binder) checkListOptions(f *Field, elemInst string) {
	elemField := func(name string) *Field {
		for _, e := range f.Elem.Fields {
			if e.Name == name {
				return e
			}
		}
		return nil
	}
	isStringKey := func(e *Field) bool {
		_, ok := e.Codec.(stringCodec)
		return e.Mode == ModeSent && len(e.KeyPath) == 1 && ok
	}
	if f.MergeKey != "" {
		if e := elemField(f.MergeKey); e == nil || !isStringKey(e) {
			bb.errorf("%s: MergeByKey needs %q to be a string attribute of the element with a key of its own", f.Path, f.MergeKey)
		}
	}
	if f.CarryKey == "" {
		return
	}
	if e := elemField(f.CarryBy); e == nil || !isStringKey(e) {
		bb.errorf("%s: CarryServed needs %q to be a string attribute of the element with a key of its own", f.Path, f.CarryBy)
	}
	if !hasProp(bb.c.pinned[f.Elem.Object], f.CarryKey) {
		bb.errorf("%s: CarryServed names %s.%s, which the golden does not have", f.Path, f.Elem.Object, f.CarryKey)
		return
	}
	for _, e := range f.Elem.Fields {
		if e.Mode.hasKey() && len(e.KeyPath) == 1 && e.KeyPath[0] == f.CarryKey && !e.ReadOnly {
			bb.errorf("%s: %s is carried from the served element, so it must be computed only", f.Path, e.Path)
		}
	}
	inst := bb.instances[elemInst]
	if _, claimed := inst.claimed[f.CarryKey]; !claimed {
		inst.claimed[f.CarryKey] = f.Path + " (carried)"
	}
}

func (bb *binder) check() {
	paths := make([]string, 0, len(bb.o.attrs))
	for p := range bb.o.attrs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if !bb.o.attrs[p].used {
			bb.errorf("an option names %s, which Bind never reached: the schema has no such attribute, or it sits inside one without a key", p)
		}
	}
	for kp := range bb.o.documents {
		inst, ok := bb.instances[kp]
		if !ok || kp == "" || inst.rebuilt {
			bb.errorf("Document(%q) names no object that an attribute writes into", kp)
		}
	}
	for object, keys := range bb.o.unmanaged {
		var rebuilt []*instance
		for _, inst := range bb.instances {
			if inst.object == object && inst.rebuilt {
				rebuilt = append(rebuilt, inst)
			}
		}
		for key := range keys {
			switch {
			case len(rebuilt) == 0:
				bb.errorf("Unmanaged(%q, %q): no attribute rebuilds %s, so its unclaimed keys are kept anyway", object, key, object)
			case !hasProp(bb.c.pinned[object], key):
				bb.errorf("Unmanaged(%q, %q): the golden has no %s.%s", object, key, object, key)
			}
			for _, inst := range rebuilt {
				if by, claimed := inst.claimed[key]; claimed {
					bb.errorf("Unmanaged(%q, %q): %s claims it", object, key, by)
				}
				inst.unmanaged[key] = keys[key]
			}
		}
	}
	for _, kp := range bb.order {
		inst := bb.instances[kp]
		for _, key := range sortedKeys(bb.c.pinned[inst.object]) {
			if _, claimed := inst.claimed[key]; claimed {
				continue
			}
			inst.unclaimed = append(inst.unclaimed, key)
			if _, ok := inst.unmanaged[key]; inst.rebuilt && !ok {
				bb.errorf("rebuilding each %s drops %s.%s, which no attribute claims; bind it or declare Unmanaged(%q, %q, reason)", displayKeyPath(kp), inst.object, key, inst.object, key)
			}
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Document returns the binding of the object at keyPath, declared with the
// Document option, for the endpoint that writes it on its own. It takes and
// returns the same object type as b: attributes outside the document are
// neither written nor read, and a read keeps their prior values.
func (b *Binding) Document(keyPath ...string) (*Binding, error) {
	kp := strings.Join(keyPath, ".")
	if !b.documents[kp] {
		return nil, fmt.Errorf("%s is not declared with the Document option", kp)
	}
	v := &Binding{AttrTypes: b.AttrTypes}
	for _, inst := range b.instances {
		if inst.keyPath == kp {
			v.Object = inst.object
		}
	}
	for _, d := range b.docs {
		switch {
		case len(d.keyPath) > len(keyPath) && slices.Equal(d.keyPath[:len(keyPath)], keyPath):
			v.docs = append(v.docs, docField{attrPath: d.attrPath, keyPath: d.keyPath[len(keyPath):], f: d.f})
		case slices.Equal(d.keyPath, keyPath) && d.f.Elem != nil:
			for _, e := range d.f.Elem.docs {
				v.docs = append(v.docs, docField{attrPath: append(slices.Clone(d.attrPath), e.attrPath...), keyPath: e.keyPath, f: e.f})
			}
		}
	}
	sortDocs(v.docs)
	v.nodes = trieOf(v.docs)
	return v, nil
}

// Select returns the binding of just the named top-level attributes, for a
// request that writes only them into a document the rest of b also covers.
func (b *Binding) Select(names ...string) (*Binding, error) {
	v := &Binding{Object: b.Object, AttrTypes: b.AttrTypes}
	for _, name := range names {
		found := false
		for _, d := range b.docs {
			if len(d.attrPath) == 1 && d.attrPath[0] == name {
				v.docs = append(v.docs, d)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("%s is not a top-level attribute of the binding", name)
		}
	}
	sortDocs(v.docs)
	v.nodes = trieOf(v.docs)
	return v, nil
}
