// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

const testPinned = `schema Doc.Name: string
schema Doc.Count: integer:int32
schema Doc.Ratio: number:double
schema Doc.Enabled: boolean
schema Doc.Tags: []string
schema Doc.Sizes: []integer:int32
schema Doc.Joined: string
schema Doc.Limit: integer:int32
schema Doc.Fresh: string
schema Doc.Stamp: string
schema Doc.Hosts: []#Host
schema Doc.Opts: #Opts
schema Doc.Sub: #Sub
schema Doc.Types: []#TypeOpt
schema Doc.Kept: string
schema Host.Url: string
schema Host.Kind: string
schema Host.Extra: integer:int32
schema Host.Created: string
schema Opts.Level: integer:int32
schema Opts.Mode: string
schema Sub.Flag: boolean
schema Sub.Other: boolean
schema Sub.Kept: string
schema Sub.Limit: integer:int32
schema TypeOpt.Type: string
schema TypeOpt.Fetchers: []string
schema TypeOpt.Images: []#Image
schema Image.Type: string
schema Image.Limit: integer:int32
`

// testFloor lacks Doc.Fresh, and has Doc.Renamed, which the pinned golden
// dropped.
var testFloor = strings.Replace(testPinned, "schema Doc.Fresh: string\n", "schema Doc.Renamed: string\n", 1)

func testCatalog() *catalog {
	return &catalog{pinned: parseAPIGolden(testPinned), floor: parseAPIGolden(testFloor), unversioned: map[string]bool{}, floorVer: "1.9", sinceVer: "2.0"}
}

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
		"legacy":   optString(),
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
	return append(optionsWithoutUnmanaged(), Unmanaged("Host", "Extra", "the server numbers hosts"))
}

func optionsWithoutUnmanaged() []Option {
	return []Option{
		Identity("id"),
		Inverted("disabled", "Enabled"),
		Delimited("joined", ","),
		WithCodec("stamp", stampCodec{}),
		NeverSent("gone", "Jellyfin has no such setting"),
		Elsewhere("secret", "written by another request"),
		Legacy("legacy", "OldPath", "2.0", "Jellyfin 2.0 removed it."),
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
	b, err := testCatalog().bind(schema.Schema{Attributes: testAttrs()}, "Doc", testOptions()...)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	return b
}

func TestUnitBindDescribesWhatItDerivesAndWhatIsDeclared(t *testing.T) {
	want := []string{
		"count -> Doc.Count integer:int32 read-missing-as=0",
		"disabled -> Doc.Enabled boolean codec=inverted",
		"fresh -> Doc.Fresh string since=2.0",
		"gone -> never sent",
		"hoisted -> Doc.Sub.Other boolean",
		"hosts -> Doc.Hosts []#Host carries=Created/url",
		"hosts.created -> Host.Created string read-only",
		"hosts.kind -> Host.Kind string",
		"hosts.url -> Host.Url string",
		"id -> identity",
		"joined -> Doc.Joined string codec=delimited(\",\")",
		"legacy -> Doc.OldPath legacy until=2.0",
		"limit -> Doc.Limit integer:int32 null-clears",
		"name -> Doc.Name string",
		"opts -> Doc.Opts #Opts",
		"opts.level -> Opts.Level integer:int32",
		"opts.mode -> Opts.Mode string",
		"ratio -> Doc.Ratio number:double",
		"secret -> elsewhere",
		"sizes -> Doc.Sizes []integer:int32",
		"stamp -> Doc.Stamp string codec=custom",
		"sub -> Doc.Sub #Sub document",
		"sub.flag -> Sub.Flag boolean",
		"sub.limit -> Sub.Limit integer:int32 null-clears",
		"tags -> Doc.Tags []string",
		"types -> Doc.Types []#TypeOpt merge-by=type",
		"types.fetchers -> TypeOpt.Fetchers []string",
		"types.images -> TypeOpt.Images []#Image merge-by=type",
		"types.images.limit -> Image.Limit integer:int32",
		"types.images.type -> Image.Type string",
		"types.type -> TypeOpt.Type string",
		"(document) Doc keeps Kept",
		"Sub Sub keeps Kept",
		"Hosts[] Host omits Extra",
	}
	got := testBinding(t).Describe()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Describe:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
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
		"a typo in a name": {
			attrs: withAttr("namex", optString()),
			want:  `namex: no property of Doc matches "namex" (nearest Name)`,
		},
		"a key Jellyfin renamed": {
			attrs: withAttr("renamed", optString()),
			want:  "the floor golden (Jellyfin 1.9) has Doc.Renamed, so Jellyfin removed or renamed it",
		},
		"a key that does not exist": {
			opts: []Option{Key("name", "Nope")},
			want: `name: Doc has no property "Nope"`,
		},
		"a redundant key": {
			opts: []Option{Key("name", "Name")},
			want: `name: Key("Name") is what the name resolves to anyway`,
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
			attrs: withAttr("name", schema.Int64Attribute{Optional: true}),
			want:  "name -> Doc.Name: attribute type basetypes.Int64Type and wire type string have no default codec",
		},
		"inverting a string": {
			opts: []Option{Inverted("name", "Name")},
			want: "Inverted needs a bool attribute and a boolean key",
		},
		"delimiting a list key": {
			opts: []Option{Delimited("tags", ",")},
			want: "Delimited needs a list of strings and a string key",
		},
		"a legacy key the pin has": {
			opts: []Option{Legacy("name", "Name", "2.0", "because")},
			want: "the pinned golden has Doc.Name, so map it with Key instead of Legacy",
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
		"a rebuilt element dropping a key": {
			base: optionsWithoutUnmanaged(),
			want: `rebuilding each Hosts[] drops Host.Extra, which no attribute claims; bind it or declare Unmanaged("Host", "Extra", reason)`,
		},
		"an unmanaged key without a reason": {
			base: optionsWithoutUnmanaged(),
			opts: []Option{Unmanaged("Host", "Extra", " ")},
			want: "Unmanaged(Host, Extra) needs a reason",
		},
		"a version message for an attribute every version has": {
			opts: []Option{VersionMessage("name", func(VersionGap) (string, string) { return "", "" })},
			want: "name: VersionMessage names an attribute VersionErrors never reports",
		},
		"a version message for a keyless attribute": {
			opts: []Option{VersionMessage("gone", func(VersionGap) (string, string) { return "", "" })},
			want: "gone is never sent, so it takes no key, codec, list, read or version option",
		},
		"unmanaged on a merged object": {
			opts: []Option{Unmanaged("TypeOpt", "Fetchers", "because")},
			want: `Unmanaged("TypeOpt", "Fetchers"): no attribute rebuilds TypeOpt`,
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
		"a nested attribute on a scalar key": {
			attrs: withAttr("name", schema.SingleNestedAttribute{Optional: true, Attributes: map[string]schema.Attribute{"x": optString()}}),
			want:  "name is an object, but Doc.Name is string",
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
			_, err := testCatalog().bind(schema.Schema{Attributes: attrs}, "Doc", append(opts, c.opts...)...)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %v\nwant it to contain %q", err, c.want)
			}
		})
	}
}

func TestUnitBindRejectsAnObjectMissingFromTheFloor(t *testing.T) {
	c := testCatalog()
	delete(c.floor, "Opts")
	_, err := c.bind(schema.Schema{Attributes: testAttrs()}, "Doc", testOptions()...)
	if err == nil || !strings.Contains(err.Error(), "the floor golden has no schema Opts") {
		t.Errorf("error %v, want the floor golden to be named", err)
	}
	c.unversioned["Opts"] = true
	if _, err := c.bind(schema.Schema{Attributes: testAttrs()}, "Doc", testOptions()...); err != nil {
		t.Errorf("an unversioned object still needs the floor: %v", err)
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
