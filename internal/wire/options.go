// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
)

// Option declares what Bind cannot derive from the schema and the goldens.
// Attribute paths are dotted attribute names from the resource root, without
// list indexes, such as "library_options.type_options.image_options".
type Option func(*options)

type options struct {
	attrs map[string]*attrOption
	// unmanaged maps an object to the keys a rebuilt element of it may leave
	// out, with the reason.
	unmanaged map[string]map[string]string
	documents map[string]bool
	errs      []string
}

type attrOption struct {
	mode        Mode
	modeSet     bool
	reason      string
	key         string
	until       string
	codec       Codec
	mergeKey    string
	carryKey    string
	carryBy     string
	readMissing attr.Value
	used        bool
}

func (o *options) attr(path string) *attrOption {
	if o.attrs[path] == nil {
		o.attrs[path] = &attrOption{}
	}
	return o.attrs[path]
}

func (o *options) setMode(path string, m Mode, reason string) *attrOption {
	a := o.attr(path)
	if a.modeSet && a.mode != m {
		o.errs = append(o.errs, fmt.Sprintf("%s is declared both %s and %s", path, a.mode, m))
	}
	a.mode, a.modeSet = m, true
	if m != ModeSent && m != ModeIdentity && strings.TrimSpace(reason) == "" {
		o.errs = append(o.errs, fmt.Sprintf("%s is declared %s without a reason", path, m))
	}
	a.reason = reason
	return a
}

func (o *options) setKey(path, key string) {
	a := o.attr(path)
	if a.key != "" && a.key != key {
		o.errs = append(o.errs, fmt.Sprintf("%s is given two keys, %s and %s", path, a.key, key))
	}
	a.key = key
}

func (o *options) setCodec(path string, c Codec) {
	a := o.attr(path)
	if a.codec != nil {
		o.errs = append(o.errs, fmt.Sprintf("%s is given two codecs", path))
	}
	a.codec = c
}

// Key maps the attribute to a key path other than the one its name resolves
// to, such as "Policy.IsAdministrator" for an attribute that sits at the
// resource root but lives in the user's policy.
func Key(attrPath, keyPath string) Option {
	return func(o *options) { o.setKey(attrPath, keyPath) }
}

// Inverted maps a bool attribute to a boolean key that means its opposite.
func Inverted(attrPath, key string) Option {
	return func(o *options) {
		o.setKey(attrPath, key)
		o.setCodec(attrPath, boolCodec{inverted: true})
	}
}

// Delimited maps a list of strings to one string joined with sep.
func Delimited(attrPath, sep string) Option {
	return func(o *options) { o.setCodec(attrPath, delimitedCodec{sep: sep}) }
}

// WithCodec replaces the codec the attribute's type and key would get. A
// codec that implements fmt.Stringer is named by it in the bindings golden.
func WithCodec(attrPath string, c Codec) Option {
	return func(o *options) { o.setCodec(attrPath, c) }
}

// Identity marks attributes the resource sets itself, such as its id: they
// are never written, and a read keeps their prior value.
func Identity(attrPaths ...string) Option {
	return func(o *options) {
		for _, p := range attrPaths {
			o.setMode(p, ModeIdentity, "")
		}
	}
}

// NeverSent marks an attribute with no Jellyfin key behind it: it is never
// written and always reads as null.
func NeverSent(attrPath, reason string) Option {
	return func(o *options) { o.setMode(attrPath, ModeNeverSent, reason) }
}

// Elsewhere marks an attribute the resource writes through another request,
// such as a password: the binding neither writes it nor reads it, so a read
// keeps its prior value.
func Elsewhere(attrPath, reason string) Option {
	return func(o *options) { o.setMode(attrPath, ModeElsewhere, reason) }
}

// Legacy maps an attribute to a key that only Jellyfin releases before until
// have, so no golden can check it. VersionErrors rejects a configured value on
// until and later.
func Legacy(attrPath, key, until, reason string) Option {
	return func(o *options) {
		a := o.setMode(attrPath, ModeLegacy, reason)
		o.setKey(attrPath, key)
		a.until = until
	}
}

// MergeByKey writes each element of a nested list over the served element
// whose keyAttr value matches it, ignoring case, instead of rebuilding it, so
// the element keeps what the attributes leave unset.
func MergeByKey(listPath, keyAttr string) Option {
	return func(o *options) { o.attr(listPath).mergeKey = keyAttr }
}

// CarryServed copies jsonKey into each rebuilt element of a nested list from
// the served element with the same byAttr value. The attribute bound to
// jsonKey, if any, must be computed only: the server owns the value.
func CarryServed(listPath, jsonKey, byAttr string) Option {
	return func(o *options) {
		a := o.attr(listPath)
		a.carryKey, a.carryBy = jsonKey, byAttr
	}
}

// ReadMissingAs gives the value a read returns when the key is missing, null
// or of the wrong type.
func ReadMissingAs(attrPath string, v attr.Value) Option {
	return func(o *options) { o.attr(attrPath).readMissing = v }
}

// Unmanaged lets a rebuilt element of object leave out key, which no
// attribute claims; rebuilding it otherwise drops the key, so each one must
// be declared.
func Unmanaged(object, key, reason string) Option {
	return func(o *options) {
		if strings.TrimSpace(reason) == "" {
			o.errs = append(o.errs, fmt.Sprintf("Unmanaged(%s, %s) needs a reason", object, key))
		}
		if o.unmanaged[object] == nil {
			o.unmanaged[object] = map[string]string{}
		}
		o.unmanaged[object][key] = reason
	}
}

// Document declares that the object at keyPath is written on its own, as a
// document of its own endpoint. A write merges into it instead of rebuilding
// it, and Binding.Document returns the binding of just that document.
func Document(keyPath string) Option {
	return func(o *options) { o.documents[keyPath] = true }
}
