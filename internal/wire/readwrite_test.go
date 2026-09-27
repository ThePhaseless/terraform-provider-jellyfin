// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflogtest"
)

func doc(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	out := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func canonical(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var x any
	if err := json.Unmarshal(raw, &x); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// at returns the value at a path such as "types[0].images[1].limit".
func at(t *testing.T, v attr.Value, p string) attr.Value {
	t.Helper()
	for _, part := range strings.Split(p, ".") {
		name, index, hasIndex := strings.Cut(strings.TrimSuffix(part, "]"), "[")
		obj, ok := v.(basetypes.ObjectValue)
		if !ok {
			t.Fatalf("%s: %s is not an object", p, name)
		}
		v = obj.Attributes()[name]
		if hasIndex {
			i, _ := strconv.Atoi(index)
			l, ok := v.(basetypes.ListValue)
			if !ok || i >= len(l.Elements()) {
				t.Fatalf("%s: no element %d of %s", p, i, name)
			}
			v = l.Elements()[i]
		}
	}
	return v
}

func with(t *testing.T, obj attr.Value, p string, v attr.Value) attr.Value {
	t.Helper()
	ctx := context.Background()
	head, rest, nested := strings.Cut(p, ".")
	name, index, hasIndex := strings.Cut(strings.TrimSuffix(head, "]"), "[")
	o, ok := obj.(basetypes.ObjectValue)
	if !ok {
		t.Fatalf("%s is not in an object", p)
	}
	attrs := map[string]attr.Value{}
	for k, old := range o.Attributes() {
		attrs[k] = old
	}
	switch {
	case hasIndex:
		i, _ := strconv.Atoi(index)
		l, _ := attrs[name].(basetypes.ListValue)
		elems := append([]attr.Value(nil), l.Elements()...)
		if nested {
			elems[i] = with(t, elems[i], rest, v)
		} else {
			elems[i] = v
		}
		attrs[name] = types.ListValueMust(l.ElementType(ctx), elems)
	case nested:
		attrs[name] = with(t, attrs[name], rest, v)
	default:
		attrs[name] = v
	}
	out, d := types.ObjectValue(o.AttributeTypes(ctx), attrs)
	if d.HasError() {
		t.Fatal(d)
	}
	return out
}

func object(t *testing.T, v attr.Value) types.Object {
	t.Helper()
	o, ok := v.(basetypes.ObjectValue)
	if !ok {
		t.Fatalf("%T is not an object", v)
	}
	return o
}

const testServed = `{
	"Name": "n", "Count": 3, "Ratio": 1.5, "Enabled": false,
	"Tags": ["a", "b"], "Sizes": [1, 2], "Joined": "x,y", "Limit": 7,
	"Fresh": "f", "Stamp": "abc", "Gone": "g", "OldPath": "//old", "Kept": "k",
	"Hosts": [{"Url": "u0", "Kind": "k0", "Extra": 5, "Created": "c0"}, {"Url": "u1", "Kind": "k1", "Created": "c1"}],
	"Opts": {"Level": 2, "Mode": "m"},
	"Sub": {"Flag": true, "Other": false, "Kept": "sk", "Limit": 9},
	"Types": [{"Type": "Movie", "Fetchers": ["f"], "Images": [{"Type": "Primary", "Limit": 1}], "Extra": "e"}]
}`

func TestUnitFlattenReadsEachAttribute(t *testing.T) {
	ctx := context.Background()
	b := testBinding(t)
	nullPrior := types.ObjectNull(b.AttrTypes)
	priorWith := func(values map[string]attr.Value) types.Object {
		attrs := map[string]attr.Value{}
		for name, typ := range b.AttrTypes {
			attrs[name] = nullOf(ctx, typ)
		}
		for name, v := range values {
			attrs[name] = v
		}
		return types.ObjectValueMust(b.AttrTypes, attrs)
	}
	got, d := b.Flatten(ctx, doc(t, testServed), priorWith(map[string]attr.Value{
		"id": types.StringValue("the-id"), "secret": types.StringValue("pw"), "stamp": types.StringValue("ABC"),
	}))
	if d.HasError() {
		t.Fatal(d)
	}
	for p, want := range map[string]string{
		"id":                 `"the-id"`,
		"secret":             `"pw"`,
		"name":               `"n"`,
		"count":              `3`,
		"ratio":              `1.500000`,
		"disabled":           `true`,
		"tags":               `["a","b"]`,
		"sizes":              `[1,2]`,
		"joined":             `["x","y"]`,
		"limit":              `7`,
		"fresh":              `"f"`,
		"stamp":              `"ABC"`,
		"gone":               `<null>`,
		"legacy":             `"//old"`,
		"hoisted":            `false`,
		"hosts[0].created":   `"c0"`,
		"hosts[1].kind":      `"k1"`,
		"opts.mode":          `"m"`,
		"sub.flag":           `true`,
		"sub.limit":          `9`,
		"types[0].fetchers":  `["f"]`,
		"types[0].images[0]": `{"limit":1,"type":"Primary"}`,
	} {
		if s := at(t, got, p).String(); s != want {
			t.Errorf("%s = %s, want %s", p, s, want)
		}
	}

	sparse, d := b.Flatten(ctx, doc(t, `{"Name": 5, "Tags": null, "Joined": ""}`), nullPrior)
	if d.HasError() {
		t.Fatal(d)
	}
	for p, want := range map[string]string{
		"name":   "<null>",
		"tags":   "<null>",
		"joined": "[]",
		"count":  "0",
		"hosts":  "<null>",
		"id":     "<null>",
	} {
		if s := at(t, sparse, p).String(); s != want {
			t.Errorf("sparse %s = %s, want %s", p, s, want)
		}
	}
}

func TestUnitFlattenReadsAKeySpelledOtherwise(t *testing.T) {
	var logs bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &logs)
	b := testBinding(t)
	got, d := b.Flatten(ctx, doc(t, `{"name": "lower", "RATIO": 2.5, "Tags": ["exact"], "t_a_g_s": ["other"], "Si_zes": [1], "sizes": [2]}`), types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	for p, want := range map[string]string{"name": `"lower"`, "ratio": "2.500000", "tags": `["exact"]`, "sizes": "<null>"} {
		if s := at(t, got, p).String(); s != want {
			t.Errorf("%s = %s, want %s", p, s, want)
		}
	}
	if !strings.Contains(logs.String(), `"served":"name"`) || !strings.Contains(logs.String(), "Reading a Jellyfin key spelled otherwise than the golden") {
		t.Errorf("the fallback read is not logged:\n%s", logs.String())
	}
}

func TestUnitFlattenRejectsAnUnreadableNestedValue(t *testing.T) {
	b := testBinding(t)
	for _, raw := range []string{`{"Hosts": "x"}`, `{"Opts": 3}`, `{"Types": [1]}`} {
		if _, d := b.Flatten(context.Background(), doc(t, raw), types.ObjectNull(b.AttrTypes)); !d.HasError() {
			t.Errorf("%s reads without an error", raw)
		}
	}
}

func TestUnitOverlayWritesTheConfiguredValues(t *testing.T) {
	var logs bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &logs)
	b := testBinding(t)
	model, d := b.Flatten(ctx, doc(t, testServed), types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	m := with(t, model, "id", types.StringValue("the-id"))
	m = with(t, m, "gone", types.StringValue("never"))
	m = with(t, m, "secret", types.StringValue("pw"))
	m = with(t, m, "name", types.StringValue("renamed"))
	m = with(t, m, "count", types.Int64Unknown())
	m = with(t, m, "ratio", types.Float64Null())
	m = with(t, m, "limit", types.Int64Null())
	m = with(t, m, "hoisted", types.BoolValue(true))
	m = with(t, m, "opts.mode", types.StringNull())
	m = with(t, m, "hosts[0].created", types.StringValue("mine"))
	m = with(t, m, "types[0].type", types.StringValue("MOVIE"))
	m = with(t, m, "types[0].fetchers", types.ListValueMust(types.StringType, []attr.Value{types.StringValue("g")}))
	m = with(t, m, "types[0].images[0].limit", types.Int64Value(4))
	m = with(t, m, "sub.limit", types.Int64Null())

	served := doc(t, testServed)
	served["Types"] = json.RawMessage(`[{"Type": "Other"}, {"Type": "movie", "Extra": "e", "Images": [{"Type": "primary", "MinWidth": 3}]}]`)
	if d := b.Overlay(ctx, served, object(t, m)); d.HasError() {
		t.Fatal(d)
	}
	want := `{"Count":3,"Enabled":false,"Fresh":"f","Gone":"g","Hosts":[{"Created":"c0","Kind":"k0","Url":"u0"},{"Created":"c1","Kind":"k1","Url":"u1"}],` +
		`"Joined":"x,y","Kept":"k","Limit":null,"Name":"renamed","OldPath":"//old","Opts":{"Level":2},"Ratio":1.5,"Sizes":[1,2],"Stamp":"abc",` +
		`"Sub":{"Flag":true,"Kept":"sk","Limit":null,"Other":true},"Tags":["a","b"],` +
		`"Types":[{"Extra":"e","Fetchers":["g"],"Images":[{"Limit":4,"MinWidth":3,"Type":"Primary"}],"Type":"MOVIE"}]}`
	if got := canonical(t, served); got != want {
		t.Errorf("overlay wrote\n%s\nwant\n%s", got, want)
	}
	if !strings.Contains(logs.String(), `"key":"Name"`) || !strings.Contains(logs.String(), `"key":"Sub.Other"`) {
		t.Errorf("the writes are not logged:\n%s", logs.String())
	}
}

func TestUnitOverlayKeepsAServedKeySpelledOtherwise(t *testing.T) {
	ctx := context.Background()
	b := testBinding(t)
	model, _ := b.Flatten(ctx, doc(t, testServed), types.ObjectNull(b.AttrTypes))
	served := doc(t, `{"name": "shadow"}`)
	if d := b.Overlay(ctx, served, object(t, with(t, model, "name", types.StringValue("renamed")))); d.HasError() {
		t.Fatal(d)
	}
	if got, want := string(served["name"])+" "+string(served["Name"]), `"shadow" "renamed"`; got != want {
		t.Errorf("name and Name are %s, want %s", got, want)
	}
}

func TestUnitSelectWritesAndReadsOnlyTheNamedAttributes(t *testing.T) {
	ctx := context.Background()
	b := testBinding(t)
	sel, err := b.Select("name", "hoisted")
	if err != nil {
		t.Fatal(err)
	}
	model, _ := b.Flatten(ctx, doc(t, testServed), types.ObjectNull(b.AttrTypes))
	m := with(t, model, "name", types.StringValue("renamed"))
	m = with(t, m, "hoisted", types.BoolValue(true))
	m = with(t, m, "count", types.Int64Value(9))
	m = with(t, m, "limit", types.Int64Null())
	m = with(t, m, "sub.flag", types.BoolValue(false))
	m = with(t, m, "hosts[0].url", types.StringValue("elsewhere"))

	served := doc(t, testServed)
	if d := sel.Overlay(ctx, served, object(t, m)); d.HasError() {
		t.Fatal(d)
	}
	want := doc(t, testServed)
	want["Name"] = json.RawMessage(`"renamed"`)
	want["Sub"] = json.RawMessage(`{"Flag": true, "Other": true, "Kept": "sk", "Limit": 9}`)
	if got, want := canonical(t, served), canonical(t, want); got != want {
		t.Errorf("select wrote\n%s\nwant\n%s", got, want)
	}

	read, d := sel.Flatten(ctx, doc(t, `{"Name": "read", "Count": 1, "Limit": 2, "Sub": {"Flag": true, "Other": false}}`), object(t, m))
	if d.HasError() {
		t.Fatal(d)
	}
	for p, want := range map[string]string{"name": `"read"`, "hoisted": "false", "count": "9", "sub.flag": "false", "limit": "<null>", "hosts[0].url": `"elsewhere"`} {
		if s := at(t, read, p).String(); s != want {
			t.Errorf("select read %s = %s, want %s", p, s, want)
		}
	}
}

func subDocument(t *testing.T) (*Binding, types.Object) {
	t.Helper()
	b := testBinding(t)
	sub, err := b.Document("Sub")
	if err != nil {
		t.Fatal(err)
	}
	model, d := b.Flatten(context.Background(), doc(t, testServed), types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	return sub, model
}

func TestUnitDocumentWriteLeavesTheAttributesOfANullObjectOut(t *testing.T) {
	sub, model := subDocument(t)
	subType, _ := sub.AttrTypes["sub"].(types.ObjectType)
	m := with(t, model, "sub", types.ObjectNull(subType.AttrTypes))
	m = with(t, m, "hoisted", types.BoolValue(true))
	served := doc(t, `{"Flag": false, "Limit": 9}`)
	if d := sub.Overlay(context.Background(), served, object(t, m)); d.HasError() {
		t.Fatal(d)
	}
	if got, want := canonical(t, served), `{"Flag":false,"Limit":9,"Other":true}`; got != want {
		t.Errorf("overlay wrote %s, want %s", got, want)
	}
}

func TestUnitDocumentReadKeepsTheAttributesOutsideIt(t *testing.T) {
	sub, model := subDocument(t)
	prior := object(t, with(t, model, "name", types.StringValue("kept")))
	read, d := sub.Flatten(context.Background(), doc(t, `{"Flag": true, "Other": false, "Limit": 1}`), prior)
	if d.HasError() {
		t.Fatal(d)
	}
	for p, want := range map[string]string{"name": `"kept"`, "sub.flag": "true", "sub.limit": "1", "hoisted": "false", "count": "3"} {
		if s := at(t, read, p).String(); s != want {
			t.Errorf("document read %s = %s, want %s", p, s, want)
		}
	}
}

func TestUnitAnUnreadableServedListFailsAMergeButNotACarry(t *testing.T) {
	ctx := context.Background()
	b := testBinding(t)
	model, _ := b.Flatten(ctx, doc(t, testServed), types.ObjectNull(b.AttrTypes))
	if d := b.Overlay(ctx, doc(t, `{"Types": "x"}`), model); !d.HasError() {
		t.Error("a merged list over an unreadable served value writes")
	}
	if d := b.Overlay(ctx, doc(t, `{"Hosts": "x"}`), model); d.HasError() {
		t.Errorf("a carried key blocks the write: %v", d)
	}
}

func TestUnitOverlayRejectsAnUnknownOrNullListElement(t *testing.T) {
	ctx := context.Background()
	b := testBinding(t)
	model, _ := b.Flatten(ctx, doc(t, testServed), types.ObjectNull(b.AttrTypes))
	hosts, _ := at(t, model, "hosts").(basetypes.ListValue)
	et, _ := hosts.ElementType(ctx).(basetypes.ObjectType)
	for name, elem := range map[string]attr.Value{"unknown": types.ObjectUnknown(et.AttrTypes), "null": types.ObjectNull(et.AttrTypes)} {
		d := b.Overlay(ctx, doc(t, testServed), object(t, with(t, model, "hosts[1]", elem)))
		if !d.HasError() {
			t.Errorf("a list with a %s element writes", name)
		}
	}
}

func TestUnitKeepPlannedNullsInsideListElements(t *testing.T) {
	ctx := context.Background()
	b := testBinding(t)
	full, _ := b.Flatten(ctx, doc(t, testServed), types.ObjectNull(b.AttrTypes))
	planned := with(t, full, "hosts[1].kind", types.StringNull())
	planned = with(t, planned, "types[0].fetchers", types.ListNull(types.StringType))
	planned = with(t, planned, "types[0].images[0].type", types.StringNull())
	planned = with(t, planned, "name", types.StringNull())
	planned = with(t, planned, "tags", types.ListNull(types.StringType))
	planned = with(t, planned, "opts.mode", types.StringNull())
	got := with(t, full, "hosts[1].kind", types.StringValue(""))
	got = with(t, got, "types[0].fetchers", types.ListValueMust(types.StringType, nil))
	got = with(t, got, "types[0].images[0].type", types.StringValue(""))
	got = with(t, got, "name", types.StringValue(""))
	got = with(t, got, "tags", types.ListValueMust(types.StringType, nil))
	got = with(t, got, "opts.mode", types.StringValue(""))

	kept := KeepPlannedNulls(object(t, planned), object(t, got))
	for p, want := range map[string]string{
		"hosts[1].kind": "<null>", "types[0].fetchers": "<null>", "types[0].images[0].type": "<null>",
		"name": `""`, "tags": "[]", "opts.mode": `""`, "hosts[0].kind": `"k0"`,
	} {
		if s := at(t, kept, p).String(); s != want {
			t.Errorf("%s = %s, want %s", p, s, want)
		}
	}

	hosts, _ := at(t, got, "hosts").(basetypes.ListValue)
	shorter := with(t, got, "hosts", types.ListValueMust(hosts.ElementType(ctx), []attr.Value{at(t, got, "hosts[1]")}))
	if s := at(t, KeepPlannedNulls(object(t, planned), object(t, shorter)), "hosts[0].kind").String(); s != `""` {
		t.Errorf("elements of lists of different lengths were paired: kind = %s", s)
	}
}

func TestUnitDroppedNamesEachAttributeReadBackAsNull(t *testing.T) {
	ctx := context.Background()
	b := testBinding(t)
	planned, _ := b.Flatten(ctx, doc(t, testServed), types.ObjectNull(b.AttrTypes))
	got := with(t, planned, "fresh", types.StringNull())
	got = with(t, got, "name", types.StringNull())
	got = with(t, got, "hosts[1].created", types.StringNull())
	got = with(t, got, "types[0].images[0].limit", types.Int64Null())
	got = with(t, got, "sub.flag", types.BoolNull())
	plannedWithUnknown := with(t, planned, "opts.mode", types.StringUnknown())
	got = with(t, got, "opts.mode", types.StringNull())

	diags := b.Dropped(object(t, plannedWithUnknown), object(t, got))
	var paths []string
	for _, e := range diags {
		pe, ok := e.(diag.DiagnosticWithPath)
		if !ok {
			t.Fatalf("%v has no path", e)
		}
		paths = append(paths, pe.Path().String())
		if pe.Path().String() == "fresh" && !strings.Contains(e.Detail(), "needs Jellyfin 2.0 or later") {
			t.Errorf("fresh does not name the version it needs: %s", e.Detail())
		}
	}
	if got, want := strings.Join(paths, " "), "fresh name sub.flag types[0].images[0].limit"; got != want {
		t.Errorf("Dropped reports %s, want %s", got, want)
	}
}

func configOf(t *testing.T, attrs map[string]schema.Attribute, v attr.Value) tfsdk.Config {
	t.Helper()
	raw, err := v.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return tfsdk.Config{Schema: schema.Schema{Attributes: attrs}, Raw: raw}
}

func versionErrorLines(diags diag.Diagnostics) []string {
	var out []string
	for _, e := range diags {
		if pe, ok := e.(diag.DiagnosticWithPath); ok {
			out = append(out, pe.Path().String()+" | "+e.Summary()+" | "+e.Detail())
		}
	}
	return out
}

func TestUnitVersionErrors(t *testing.T) {
	ctx := context.Background()
	b := testBinding(t)
	full, _ := b.Flatten(ctx, doc(t, testServed), types.ObjectNull(b.AttrTypes))
	config := func(v attr.Value) tfsdk.Config { return configOf(t, testAttrs(), v) }
	unset := with(t, with(t, full, "fresh", types.StringNull()), "legacy", types.StringNull())

	for _, c := range []struct {
		name    string
		cfg     attr.Value
		version string
		want    string
		calls   int
	}{
		{"both on an old server", full, "1.9.3", "fresh", 1},
		{"both on a new server", full, "2.0.0", "legacy", 1},
		{"a version without digits", full, "unstable", "", 1},
		{"neither", unset, "1.0", "", 0},
		{"an unknown value", with(t, unset, "fresh", types.StringUnknown()), "1.0", "", 0},
	} {
		calls := 0
		diags := b.VersionErrors(ctx, config(c.cfg), func() (string, error) {
			calls++
			return c.version, nil
		})
		var paths []string
		for _, e := range diags {
			if pe, ok := e.(diag.DiagnosticWithPath); ok {
				paths = append(paths, pe.Path().String())
			}
		}
		if got := strings.Join(paths, " "); got != c.want || calls != c.calls {
			t.Errorf("%s: errors at %q after %d version reads, want %q after %d", c.name, got, calls, c.want, c.calls)
		}
	}
	diags := b.VersionErrors(ctx, config(full), func() (string, error) { return "", fmt.Errorf("offline") })
	if !diags.HasError() || !strings.Contains(diags[0].Summary(), "Jellyfin version") {
		t.Errorf("a failed version read is not reported: %v", diags)
	}

	for version, want := range map[string]string{
		"1.9.3": "fresh | Unsupported Jellyfin server version | fresh requires Jellyfin 2.0 or later: the server runs Jellyfin 1.9.3, which has no Fresh field, so it would discard the value. Remove fresh from the configuration or upgrade the server.",
		"2.0.0": "legacy | Unsupported Jellyfin server version | The server runs Jellyfin 2.0.0. Jellyfin 2.0 removed it. Remove legacy from the configuration.",
	} {
		got := versionErrorLines(b.VersionErrors(ctx, config(full), func() (string, error) { return version, nil }))
		if len(got) != 1 || got[0] != want {
			t.Errorf("on %s: %q\nwant %q", version, got, want)
		}
	}
}

func TestUnitVersionErrorsUseTheDeclaredMessage(t *testing.T) {
	ctx := context.Background()
	message := func(g VersionGap) (string, string) {
		return "No " + g.Key, fmt.Sprintf("%s since=%s until=%s server=%s", g.Path, g.Since, g.Until, g.ServerVersion)
	}
	b, err := testCatalog().bind(schema.Schema{Attributes: testAttrs()}, "Doc",
		append(testOptions(), VersionMessage("fresh", message), VersionMessage("legacy", message))...)
	if err != nil {
		t.Fatal(err)
	}
	full, _ := b.Flatten(ctx, doc(t, testServed), types.ObjectNull(b.AttrTypes))
	for version, want := range map[string]string{
		"1.9.3": "fresh | No Fresh | fresh since=2.0 until= server=1.9.3",
		"2.0.0": "legacy | No OldPath | legacy since= until=2.0 server=2.0.0",
	} {
		got := versionErrorLines(b.VersionErrors(ctx, configOf(t, testAttrs(), full), func() (string, error) { return version, nil }))
		if len(got) != 1 || got[0] != want {
			t.Errorf("on %s: %q\nwant %q", version, got, want)
		}
	}
}

func TestUnitVersionErrorsGateANewNestedAttributeAsAWhole(t *testing.T) {
	ctx := context.Background()
	c := &catalog{
		pinned:      parseAPIGolden("schema Doc.Name: string\nschema Doc.News: []#New\nschema New.Title: string\n"),
		floor:       parseAPIGolden("schema Doc.Name: string\n"),
		unversioned: map[string]bool{}, floorVer: "1.9", sinceVer: "2.0",
	}
	attrs := map[string]schema.Attribute{
		"name": optString(),
		"news": schema.ListNestedAttribute{Optional: true, Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"title": optString(),
		}}},
	}
	b, err := c.bind(schema.Schema{Attributes: attrs}, "Doc")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(b.Describe(), "\n"), "name -> Doc.Name string\nnews -> Doc.News []#New since=2.0\nnews.title -> New.Title string"; got != want {
		t.Errorf("Describe:\n%s\nwant:\n%s", got, want)
	}
	cfg, d := b.Flatten(ctx, doc(t, `{"Name": "n", "News": [{"Title": "t"}]}`), types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	var paths []string
	for _, line := range versionErrorLines(b.VersionErrors(ctx, configOf(t, attrs, cfg), func() (string, error) { return "1.9", nil })) {
		p, _, _ := strings.Cut(line, " | ")
		paths = append(paths, p)
	}
	if got := strings.Join(paths, " "); got != "news" {
		t.Errorf("errors at %q, want news only", got)
	}
	message := VersionMessage("news.title", func(VersionGap) (string, string) { return "", "" })
	if _, err := c.bind(schema.Schema{Attributes: attrs}, "Doc", message); err == nil || !strings.Contains(err.Error(), "news.title: VersionMessage names an attribute VersionErrors never reports") {
		t.Errorf("a version message inside the gated list binds: %v", err)
	}
}

func TestUnitCodecs(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		codec Codec
		raw   string
		want  string
	}{
		{delimitedCodec{sep: ","}, `""`, "[]"},
		{delimitedCodec{sep: ","}, `"a,,b"`, `["a","","b"]`},
		{delimitedCodec{sep: " "}, `3`, "<null>"},
		{stringListCodec{}, `["a", null]`, `["a",""]`},
		{stringListCodec{}, `[1]`, "<null>"},
		{int64ListCodec{}, `[1.5]`, "<null>"},
		{boolCodec{inverted: true}, `true`, "false"},
		{int64Codec{}, `1.0`, "<null>"},
	} {
		v, _ := c.codec.Decode(ctx, json.RawMessage(c.raw), nil, nil)
		if v.String() != c.want {
			t.Errorf("%T decodes %s as %s, want %s", c.codec, c.raw, v, c.want)
		}
	}
	raw, _ := delimitedCodec{sep: " "}.Encode(ctx, types.ListValueMust(types.StringType, []attr.Value{types.StringValue("a"), types.StringUnknown()}))
	if string(raw) != `"a "` {
		t.Errorf("delimited encodes %s", raw)
	}
	if got, want := canonical(t, json.RawMessage(mustEncode(t, boolCodec{inverted: true}, types.BoolValue(false)))), "true"; got != want {
		t.Errorf("inverted encodes %s, want %s", got, want)
	}
}

func mustEncode(t *testing.T, c Codec, v attr.Value) []byte {
	t.Helper()
	raw, d := c.Encode(context.Background(), v)
	if d.HasError() {
		t.Fatal(d)
	}
	return raw
}
