// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
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
	ModeComplement
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
	case ModeComplement:
		return "complement"
	}
	return fmt.Sprintf("Mode(%d)", int(m))
}

func (m Mode) hasKey() bool { return m == ModeSent }

type Field struct {
	// Path is the dotted attribute path from the resource root.
	Path   string
	Name   string
	Type   attr.Type
	Mode   Mode
	Reason string
	// KeyPath leads from the enclosing binding's object to the key, and is
	// empty for a mode without a key. Object names the JSON object that holds
	// its last key.
	KeyPath []string
	Object  string
	Codec   Codec
	// NullClears comes from Optional && !Computed: a null value then clears
	// the server's, as JSON null in a merged object and by leaving the key
	// out of a rebuilt one.
	NullClears bool
	// ReadOnly comes from Computed && !Optional: the server owns the value,
	// so it is read but never written.
	ReadOnly    bool
	ReadMissing attr.Value
	// Elem binds the object of a nested attribute: each list element, or the
	// single object.
	Elem     *Binding
	MergeKey string
	CarryKey string
	CarryBy  string
	Document bool
	// Shares holds the attributes of the same object whose keys Orders or
	// Complement also writes, while those attributes have no value to write.
	Shares []*Field
	// Offered and Scope are what a Complement or Orders asks its
	// AvailableFunc for.
	Offered string
	Scope   *Field
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

// fieldNamed returns nil when b has no field for the attribute name.
func (b *Binding) fieldNamed(name string) *Field {
	for _, f := range b.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
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
}

// Bind maps every attribute of s to a key of the JSON object named root, the
// name Jellyfin's API document gives it. An attribute maps to its name in
// PascalCase, as KeyOf spells it, unless an option says otherwise; Bind fails,
// naming every option it cannot apply. Nothing here knows which keys Jellyfin
// has: TestAccWireKeysMatchTheServer checks Objects against a live server.
func Bind(s schema.Schema, root string, opts ...Option) (*Binding, error) {
	o := &options{attrs: map[string]*attrOption{}, unmanaged: map[string]map[string]string{}, documents: map[string]bool{}}
	for _, opt := range opts {
		opt(o)
	}
	bb := &binder{root: root, o: o, errs: slices.Clone(o.errs), instances: map[string]*instance{}, shared: map[*Field]string{}}
	b := bb.object("", s.Attributes, false, "")
	bb.check()
	if len(bb.errs) > 0 {
		slices.Sort(bb.errs)
		return nil, errors.New(strings.Join(bb.errs, "\n"))
	}
	b.documents = o.documents
	for _, k := range bb.order {
		b.instances = append(b.instances, bb.instances[k])
	}
	return b, nil
}

type binder struct {
	root      string
	o         *options
	errs      []string
	instances map[string]*instance
	order     []string
	// shared maps each attribute whose key Orders or Complement also writes
	// to the attribute that does.
	shared map[*Field]string
}

func (bb *binder) errorf(format string, args ...any) {
	bb.errs = append(bb.errs, fmt.Sprintf(format, args...))
}

func (bb *binder) instance(keyPath string, rebuilt, viaKeyPath bool) *instance {
	inst, ok := bb.instances[keyPath]
	if !ok {
		inst = &instance{keyPath: keyPath, object: bb.objectName(keyPath), rebuilt: rebuilt, viaKeyPath: viaKeyPath, claimed: map[string]string{}, unmanaged: map[string]string{}}
		bb.instances[keyPath] = inst
		bb.order = append(bb.order, keyPath)
		return inst
	}
	if rebuilt && inst.viaKeyPath || viaKeyPath && inst.rebuilt {
		bb.errorf("a key path writes into %s, which a nested attribute rebuilds, so the write would be lost", displayKeyPath(keyPath))
	}
	return inst
}

// objectName names the object at keyPath in messages: the root's name, or
// the key path from it.
func (bb *binder) objectName(keyPath string) string {
	if keyPath == "" {
		return bb.root
	}
	return bb.root + "." + keyPath
}

func (bb *binder) claim(keyPath, key, attrPath string, viaKeyPath bool) {
	inst := bb.instance(keyPath, false, viaKeyPath)
	if prev, ok := inst.claimed[key]; ok {
		bb.errorf("%s and %s both map to %s.%s", prev, attrPath, inst.object, key)
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

func (bb *binder) object(prefix string, attrs map[string]schema.Attribute, rebuilt bool, inst string) *Binding {
	b := &Binding{Object: bb.objectName(inst), AttrTypes: map[string]attr.Type{}}
	bb.instance(inst, rebuilt, false)
	for _, name := range slices.Sorted(maps.Keys(attrs)) {
		a := attrs[name]
		f := bb.field(joinKeyPath(prefix, name), name, a, inst)
		b.Fields = append(b.Fields, f)
		b.AttrTypes[name] = a.GetType()
		b.docs = append(b.docs, docField{attrPath: []string{name}, keyPath: f.KeyPath, f: f})
	}
	for _, f := range b.Fields {
		if opt := bb.o.attrs[f.Path]; opt != nil && opt.shares != nil {
			bb.share(b, f, opt)
		}
	}
	b.index()
	return b
}

// index orders the binding's docs, as writes follow them, and arranges them
// in the trie reads follow.
func (b *Binding) index() {
	slices.SortStableFunc(b.docs, func(x, y docField) int {
		return cmp.Or(cmp.Compare(len(x.keyPath), len(y.keyPath)), strings.Compare(strings.Join(x.attrPath, "."), strings.Join(y.attrPath, ".")))
	})
	b.nodes = trieOf(b.docs)
}

func (bb *binder) field(path, name string, a schema.Attribute, inst string) *Field {
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

	f.KeyPath = []string{KeyOf(name)}
	if opt.key != "" {
		f.KeyPath = strings.Split(opt.key, ".")
		if _, inverted := opt.codec.(boolCodec); !inverted && opt.key == KeyOf(name) {
			bb.errorf("%s: Key(%q) is what the name maps to anyway; drop it", path, opt.key)
		}
	}
	container := joinKeyPath(inst, strings.Join(f.KeyPath[:len(f.KeyPath)-1], "."))
	f.Object = bb.objectName(container)
	bb.claim(container, f.key(), path, len(f.KeyPath) > 1)

	if _, nested := a.(schema.NestedAttribute); nested && opt.codec != nil {
		bb.errorf("%s is a nested attribute, whose own attributes take their codecs, so it takes no Inverted, Delimited or WithCodec; ReadMissingAs gives it a read default", path)
	}
	switch na := a.(type) {
	case schema.ListNestedAttribute:
		if opt.mergeKey != "" && opt.carryKey != "" {
			bb.errorf("%s: an element is either merged or rebuilt, so it cannot take both MergeByKey and CarryServed", path)
		}
		elemInst := joinKeyPath(inst, strings.Join(f.KeyPath, ".")) + "[]"
		f.Elem = bb.object(path, na.NestedObject.Attributes, opt.mergeKey == "", elemInst)
		f.MergeKey, f.CarryKey, f.CarryBy = opt.mergeKey, opt.carryKey, opt.carryBy
		bb.checkListOptions(f, elemInst)
	case schema.SingleNestedAttribute:
		if opt.mergeKey != "" || opt.carryKey != "" {
			bb.errorf("%s: MergeByKey and CarryServed apply to lists of objects only", path)
		}
		kp := joinKeyPath(inst, strings.Join(f.KeyPath, "."))
		f.Document = inst == "" && bb.o.documents[kp]
		f.Elem = bb.object(path, na.Attributes, !f.Document, kp)
	case schema.NestedAttribute:
		bb.errorf("%s: %T is not supported", path, a)
	default:
		if opt.mergeKey != "" || opt.carryKey != "" {
			bb.errorf("%s: MergeByKey and CarryServed apply to lists of objects only", path)
		}
		f.Codec = opt.codec
		if f.Codec == nil {
			var err error
			if f.Codec, err = defaultCodec(f.Type); err != nil {
				bb.errorf("%s -> %s.%s: %v", path, f.Object, f.key(), err)
			}
		} else if msg := checkCodec(f); msg != "" {
			bb.errorf("%s -> %s.%s: %s", path, f.Object, f.key(), msg)
		}
	}
	return f
}

func checkCodec(f *Field) string {
	switch c := f.Codec.(type) {
	case boolCodec:
		if c.inverted && !f.Type.Equal(boolType) {
			return "Inverted needs a bool attribute"
		}
	case delimitedCodec:
		if !f.Type.Equal(stringListType) {
			return "Delimited needs a list of strings"
		}
	}
	return ""
}

func (bb *binder) checkListOptions(f *Field, elemInst string) {
	isStringKey := func(e *Field) bool {
		_, ok := e.Codec.(stringCodec)
		return e.Mode == ModeSent && len(e.KeyPath) == 1 && ok
	}
	if f.MergeKey != "" {
		if e := f.Elem.fieldNamed(f.MergeKey); e == nil || !isStringKey(e) {
			bb.errorf("%s: MergeByKey needs %q to be a string attribute of the element with a key of its own", f.Path, f.MergeKey)
		}
	}
	if f.CarryKey == "" {
		return
	}
	if e := f.Elem.fieldNamed(f.CarryBy); e == nil || !isStringKey(e) {
		bb.errorf("%s: CarryServed needs %q to be a string attribute of the element with a key of its own", f.Path, f.CarryBy)
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
	for _, p := range slices.Sorted(maps.Keys(bb.o.attrs)) {
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
	for kp, keys := range bb.o.unmanaged {
		inst, ok := bb.instances[kp]
		if !ok || !inst.rebuilt {
			bb.errorf("Unmanaged(%q, ...): no attribute rebuilds %s, so its unclaimed keys are kept anyway", kp, displayKeyPath(kp))
			continue
		}
		for key, reason := range keys {
			if by, claimed := inst.claimed[key]; claimed {
				bb.errorf("Unmanaged(%q, %q): %s claims it", kp, key, by)
			}
			inst.unmanaged[key] = reason
		}
	}
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
	v.index()
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
	v.index()
	return v, nil
}
