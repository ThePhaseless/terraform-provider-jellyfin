// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

func optString() schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Computed: true}
}

func testAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"name":     optString(),
		"count":    schema.Int64Attribute{Optional: true, Computed: true},
		"ratio":    schema.Float64Attribute{Optional: true, Computed: true},
		"disabled": schema.BoolAttribute{Optional: true, Computed: true},
		"tags":     schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true},
		"sizes":    schema.ListAttribute{ElementType: types.Int64Type, Optional: true, Computed: true},
		"joined":   schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true},
		"limit":    schema.Int64Attribute{Optional: true},
		"fresh":    optString(),
		"stamp":    optString(),
		"gone":     schema.StringAttribute{Optional: true},
		"secret":   schema.StringAttribute{Optional: true, Sensitive: true},
		"hoisted":  schema.BoolAttribute{Optional: true, Computed: true},
		"hosts": schema.ListNestedAttribute{Optional: true, Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"url":     optString(),
			"kind":    optString(),
			"created": schema.StringAttribute{Computed: true},
		}}},
		"opts": schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: map[string]schema.Attribute{
			"level": schema.Int64Attribute{Optional: true, Computed: true},
			"mode":  optString(),
		}},
		"sub": schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: map[string]schema.Attribute{
			"flag":  schema.BoolAttribute{Optional: true, Computed: true},
			"limit": schema.Int64Attribute{Optional: true},
		}},
		"types": schema.ListNestedAttribute{Optional: true, Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"type":     optString(),
			"fetchers": schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true},
			"images": schema.ListNestedAttribute{Optional: true, Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"type":  optString(),
				"limit": schema.Int64Attribute{Optional: true, Computed: true},
			}}},
		}}},
	}
}

// stampCodec reads a value back as its prior one when both are equal ignoring
// case, like the provider's date-time codec does for equal instants.
type stampCodec struct{}

func (stampCodec) Encode(ctx context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics) {
	return stringCodec{}.Encode(ctx, v)
}

func (stampCodec) Decode(ctx context.Context, raw json.RawMessage, prior attr.Value, t attr.Type) (attr.Value, diag.Diagnostics) {
	v, d := stringCodec{}.Decode(ctx, raw, prior, t)
	got, _ := v.(basetypes.StringValue)
	if p, ok := prior.(basetypes.StringValue); ok && !p.IsNull() && !p.IsUnknown() && strings.EqualFold(p.ValueString(), got.ValueString()) {
		return p, d
	}
	return got, d
}

func testOptions() []Option {
	return append(optionsWithoutUnmanaged(), Unmanaged("Hosts[]", "Extra", "the server numbers hosts"))
}

func optionsWithoutUnmanaged() []Option {
	return []Option{
		Identity("id"),
		Inverted("disabled", "Enabled"),
		Delimited("joined", ","),
		WithCodec("stamp", stampCodec{}),
		NeverSent("gone", "Jellyfin has no such setting"),
		Elsewhere("secret", "written by another request"),
		Key("hoisted", "Sub.Other"),
		Document("Sub"),
		CarryServed("hosts", "Created", "url"),
		MergeByKey("types", "type"),
		MergeByKey("types.images", "type"),
		ReadMissingAs("count", types.Int64Value(0)),
	}
}

func testBinding(t *testing.T) *Binding {
	t.Helper()
	b, err := Bind(schema.Schema{Attributes: testAttrs()}, "Doc", testOptions()...)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	return b
}

func TestUnitKeyOf(t *testing.T) {
	for name, want := range map[string]string{
		"name":                                 "Name",
		"extract_trickplay_images_during_scan": "ExtractTrickplayImagesDuringScan",
		"h264_crf":                             "H264Crf",
		"enable_ipv6":                          "EnableIpv6",
	} {
		if got := KeyOf(name); got != want {
			t.Errorf("KeyOf(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestUnitBindListsTheObjectsItWrites(t *testing.T) {
	got := map[string]Object{}
	owners := map[string]KeyOwner{}
	for _, o := range testBinding(t).Objects() {
		for key, owner := range o.Attrs {
			owners[o.KeyPath+"/"+key] = owner
		}
		o.Attrs = nil
		got[o.KeyPath] = o
	}
	for at, want := range map[string]KeyOwner{
		"/Enabled":               {Attr: "disabled", KeyPath: "Enabled"},
		"Sub/Other":              {Attr: "hoisted", KeyPath: "Sub.Other"},
		"Types[].Images[]/Limit": {Attr: "types.images.limit", KeyPath: "Limit"},
	} {
		if owners[at] != want {
			t.Errorf("owner of %s = %+v, want %+v", at, owners[at], want)
		}
	}
	want := map[string]Object{
		"": {Keys: map[string]Kind{
			"Count": "integer", "Enabled": "boolean", "Fresh": "string", "Hosts": "[]object", "Joined": "string",
			"Limit": "integer", "Name": "string", "Opts": "object", "Ratio": "number", "Sizes": "[]integer",
			"Stamp": "", "Sub": "object", "Tags": "[]string", "Types": "[]object",
		}},
		"Hosts[]":          {Rebuilt: true, Keys: map[string]Kind{"Created": "string", "Kind": "string", "Url": "string"}, Unmanaged: []string{"Extra"}},
		"Opts":             {Rebuilt: true, Keys: map[string]Kind{"Level": "integer", "Mode": "string"}},
		"Sub":              {Keys: map[string]Kind{"Flag": "boolean", "Limit": "integer", "Other": "boolean"}},
		"Types[]":          {Keys: map[string]Kind{"Fetchers": "[]string", "Images": "[]object", "Type": "string"}},
		"Types[].Images[]": {Keys: map[string]Kind{"Limit": "integer", "Type": "string"}},
	}
	for kp, w := range want {
		w.KeyPath = kp
		if g, ok := got[kp]; !ok {
			t.Errorf("Objects lacks %q", kp)
		} else if !reflect.DeepEqual(g, w) {
			t.Errorf("Objects[%q] = %+v, want %+v", kp, g, w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("Objects lists %d objects, want %d: %v", len(got), len(want), got)
	}
}

func TestUnitBindRejects(t *testing.T) {
	withAttr := func(name string, a schema.Attribute) map[string]schema.Attribute {
		attrs := testAttrs()
		attrs[name] = a
		return attrs
	}
	without := func(names ...string) map[string]schema.Attribute {
		attrs := testAttrs()
		for _, n := range names {
			delete(attrs, n)
		}
		return attrs
	}
	for name, c := range map[string]struct {
		attrs map[string]schema.Attribute
		base  []Option
		opts  []Option
		want  string
	}{
		"a redundant key": {
			opts: []Option{Key("name", "Name")},
			want: `name: Key("Name") is what the name maps to anyway`,
		},
		"an option for no attribute": {
			opts: []Option{NeverSent("nothing", "because")},
			want: "an option names nothing, which Bind never reached",
		},
		"two attributes on one key": {
			opts: []Option{Key("tags", "Joined"), Delimited("tags", ",")},
			want: "joined and tags both map to Doc.Joined",
		},
		"a type without a codec": {
			attrs: withAttr("name", schema.ListAttribute{ElementType: types.BoolType, Optional: true}),
			want:  "name -> Doc.Name: attribute type types.ListType[basetypes.BoolType] has no default codec",
		},
		"inverting a string": {
			opts: []Option{Inverted("name", "Name")},
			want: "Inverted needs a bool attribute",
		},
		"delimiting a string": {
			opts: []Option{Delimited("name", ",")},
			want: "Delimited needs a list of strings",
		},
		"a mode without a reason": {
			opts: []Option{NeverSent("name", "")},
			want: "name is declared never sent without a reason",
		},
		"two modes": {
			opts: []Option{Identity("name"), NeverSent("name", "because")},
			want: "name is declared both identity and never sent",
		},
		"a key on a keyless mode": {
			opts: []Option{Key("gone", "Name")},
			want: "gone is never sent, so it takes no key",
		},
		"an unmanaged key without a reason": {
			base: optionsWithoutUnmanaged(),
			opts: []Option{Unmanaged("Hosts[]", "Extra", " ")},
			want: "Unmanaged(Hosts[], Extra) needs a reason",
		},
		"unmanaged on a merged object": {
			opts: []Option{Unmanaged("Types[]", "Fetchers", "because")},
			want: `Unmanaged("Types[]", ...): no attribute rebuilds Types[]`,
		},
		"a carried key the attribute writes": {
			attrs: withAttr("hosts", schema.ListNestedAttribute{Optional: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"url": optString(), "kind": optString(), "created": optString(),
			}}}),
			want: "hosts: hosts.created is carried from the served element, so it must be computed only",
		},
		"merging by a missing attribute": {
			opts: []Option{MergeByKey("types", "nope")},
			want: `types: MergeByKey needs "nope" to be a string attribute of the element`,
		},
		"a document nothing writes": {
			opts: []Option{Document("Nope")},
			want: `Document("Nope") names no object that an attribute writes into`,
		},
		"a key path into a rebuilt object": {
			attrs: without("hoisted"),
			opts:  []Option{Key("name", "Opts.Mode")},
			want:  "a key path writes into Opts, which a nested attribute rebuilds",
		},
		"a read value of another type": {
			opts: []Option{ReadMissingAs("name", types.BoolValue(false))},
			want: "name: ReadMissingAs gives a basetypes.BoolType, but the attribute is a basetypes.StringType",
		},
		"a codec on a list of objects": {
			opts: []Option{WithCodec("hosts", stampCodec{})},
			want: "hosts is a nested attribute, whose own attributes take their codecs",
		},
		"delimiting an object": {
			opts: []Option{Delimited("opts", ",")},
			want: "opts is a nested attribute, whose own attributes take their codecs",
		},
		"inverting a merged list": {
			opts: []Option{Inverted("types", "Types")},
			want: "types is a nested attribute, whose own attributes take their codecs",
		},
	} {
		t.Run(name, func(t *testing.T) {
			attrs := c.attrs
			if attrs == nil {
				attrs = testAttrs()
			}
			opts := c.base
			if opts == nil {
				opts = testOptions()
			}
			_, err := Bind(schema.Schema{Attributes: attrs}, "Doc", append(opts, c.opts...)...)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %v\nwant it to contain %q", err, c.want)
			}
		})
	}
}

func TestUnitDocumentViewCoversTheKeysInTheDocument(t *testing.T) {
	b := testBinding(t)
	if _, err := b.Document("Opts"); err == nil {
		t.Error("Document accepts a key path not declared with the Document option")
	}
	sub, err := b.Document("Sub")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, d := range sub.docs {
		paths = append(paths, strings.Join(d.attrPath, ".")+"="+strings.Join(d.keyPath, "."))
	}
	if got, want := strings.Join(paths, " "), "hoisted=Other sub.flag=Flag sub.limit=Limit"; got != want {
		t.Errorf("Document(Sub) covers %s, want %s", got, want)
	}
}

func TestUnitSelectRejectsAnAttributeTheBindingLacks(t *testing.T) {
	b := testBinding(t)
	if _, err := b.Select("name", "nope"); err == nil {
		t.Error("Select accepts an unknown attribute")
	}
	sub, err := b.Document("Sub")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sub.Select("name"); err == nil {
		t.Error("Select on the Sub document accepts an attribute outside it")
	}
}
