// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const sharedPinned = `schema Lib.Name: string
schema Lib.Order: []string
schema Lib.Disabled: []string
schema Lib.Types: []#TypeOpt
schema Lib.Opts: []#Opt
schema TypeOpt.Type: string
schema TypeOpt.Fetchers: []string
schema TypeOpt.FetcherOrder: []string
schema Opt.ItemType: string
schema Opt.FetcherOrder: []string
schema Opt.DisabledFetchers: []string
`

func sharedCatalog() *catalog {
	return &catalog{
		pinned:      parseAPIGolden(sharedPinned),
		floor:       parseAPIGolden(strings.Replace(sharedPinned, "schema Opt.FetcherOrder: []string\n", "", 1)),
		unversioned: map[string]bool{}, floorVer: "1.9", sinceVer: "2.0",
	}
}

func optList() schema.ListAttribute {
	return schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true}
}

func sharedAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name":     optString(),
		"order":    optList(),
		"disabled": optList(),
		"enabled":  optList(),
		"types": schema.ListNestedAttribute{Optional: true, Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"type":          optString(),
			"fetchers":      optList(),
			"fetcher_order": optList(),
		}}},
		"opts": schema.ListNestedAttribute{Optional: true, Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"item_type":         optString(),
			"fetcher_order":     optList(),
			"disabled_fetchers": optList(),
			"fetchers":          optList(),
		}}},
	}
}

func sharedOptions() []Option {
	return []Option{
		MergeByKey("types", "type"),
		Orders("types.fetchers", "fetcher_order"),
		Complement("enabled", "order", "disabled", "Fetchers", ""),
		Complement("opts.fetchers", "fetcher_order", "disabled_fetchers", "Fetchers", "item_type"),
	}
}

func sharedBinding(t *testing.T) *Binding {
	t.Helper()
	b, err := sharedCatalog().bind(schema.Schema{Attributes: sharedAttrs()}, "Lib", sharedOptions()...)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	return b
}

// offering answers for Fetchers with the names scope maps to, and records each
// scope it is asked for.
func offering(offered map[string][]string, asked *[]string) AvailableFunc {
	return func(_ context.Context, list, scope string) ([]string, error) {
		*asked = append(*asked, list+"/"+scope)
		names, ok := offered[scope]
		if !ok {
			return nil, fmt.Errorf("scope %q: %w", scope, ErrNotOffered)
		}
		return names, nil
	}
}

func strs(values ...string) types.List {
	elems := make([]attr.Value, len(values))
	for i, v := range values {
		elems[i] = types.StringValue(v)
	}
	return types.ListValueMust(types.StringType, elems)
}

func TestUnitSharedKeysDescribe(t *testing.T) {
	want := strings.Join([]string{
		"disabled -> Lib.Disabled []string",
		"enabled -> Lib.Order+Disabled complement-of=Fetchers",
		"name -> Lib.Name string",
		"opts -> Lib.Opts []#Opt",
		"opts.disabled_fetchers -> Opt.DisabledFetchers []string",
		"opts.fetcher_order -> Opt.FetcherOrder []string since=2.0",
		"opts.fetchers -> Opt.FetcherOrder+DisabledFetchers complement-of=Fetchers/item_type since=2.0",
		"opts.item_type -> Opt.ItemType string",
		"order -> Lib.Order []string",
		"types -> Lib.Types []#TypeOpt merge-by=type",
		"types.fetcher_order -> TypeOpt.FetcherOrder []string",
		"types.fetchers -> TypeOpt.Fetchers []string orders=FetcherOrder",
		"types.type -> TypeOpt.Type string",
	}, "\n")
	if got := strings.Join(sharedBinding(t).Describe(), "\n"); got != want {
		t.Errorf("Describe:\n%s\nwant:\n%s", got, want)
	}
}

func TestUnitOrdersWritesTheOrderKeyOnlyWhileItsAttributeIsUnknown(t *testing.T) {
	b := sharedBinding(t)
	served := `{"Types": [{"Type": "Movie", "Fetchers": ["A", "B"], "FetcherOrder": ["b", "C", "A"]}]}`
	model, d := b.Flatten(context.Background(), doc(t, served), types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	for _, c := range []struct {
		name  string
		order attr.Value
		want  string
	}{
		{"unknown", types.ListUnknown(types.StringType), `{"FetcherOrder":["A","b","C"],"Fetchers":["A"],"Type":"Movie"}`},
		{"null", types.ListNull(types.StringType), `{"FetcherOrder":["A","b","C"],"Fetchers":["A"],"Type":"Movie"}`},
		{"known", strs("C"), `{"FetcherOrder":["C"],"Fetchers":["A"],"Type":"Movie"}`},
	} {
		m := with(t, model, "types[0].fetchers", strs("A"))
		m = with(t, m, "types[0].fetcher_order", c.order)
		got := doc(t, served)
		if d := b.Overlay(context.Background(), got, object(t, m)); d.HasError() {
			t.Fatalf("%s: %v", c.name, d)
		}
		if s := canonical(t, jsonValue(t, got["Types"], 0)); s != c.want {
			t.Errorf("with the order %s, the entry is %s, want %s", c.name, s, c.want)
		}
	}
}

func jsonValue(t *testing.T, raw []byte, i int) any {
	t.Helper()
	var list []any
	if err := json.Unmarshal(raw, &list); err != nil || i >= len(list) {
		t.Fatalf("%s holds no element %d (%v)", raw, i, err)
	}
	return list[i]
}

func TestUnitComplementWritesEachKeyItsAttributeLeavesToIt(t *testing.T) {
	b := sharedBinding(t)
	var asked []string
	ctx := WithAvailable(context.Background(), offering(map[string][]string{"": {"A", "B", "C"}}, &asked))
	model, d := b.Flatten(ctx, doc(t, `{"Name": "n"}`), types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	for _, c := range []struct {
		name            string
		order, disabled attr.Value
		want            string
	}{
		{"both unknown", types.ListUnknown(types.StringType), types.ListUnknown(types.StringType), `{"Disabled":["C"],"Name":"n","Order":["B","A","C"]}`},
		{"order known", strs("X"), types.ListUnknown(types.StringType), `{"Disabled":["C"],"Name":"n","Order":["X"]}`},
		{"both known", strs("X"), strs("Y"), `{"Disabled":["Y"],"Name":"n","Order":["X"]}`},
	} {
		m := with(t, model, "enabled", strs("B", "A"))
		m = with(t, m, "order", c.order)
		m = with(t, m, "disabled", c.disabled)
		got := doc(t, `{"Name": "n"}`)
		if d := b.Overlay(ctx, got, object(t, m)); d.HasError() {
			t.Fatalf("%s: %v", c.name, d)
		}
		if s := canonical(t, got); s != c.want {
			t.Errorf("%s: wrote %s, want %s", c.name, s, c.want)
		}
	}
	if strings.Join(asked, " ") != "Fetchers/ Fetchers/" {
		t.Errorf("asked for %q, want the offered names once for each write that needs them", asked)
	}
}

func TestUnitComplementReadsTheEnabledNamesInOrder(t *testing.T) {
	b := sharedBinding(t)
	for _, c := range []struct {
		name, served string
		offered      map[string][]string
		want         string
		asks         int
	}{
		{"ordered, disabled and new names", `{"Order": ["B", "A", "C"], "Disabled": ["c"]}`, map[string][]string{"": {"A", "B", "C", "E"}}, `["B","A","E"]`, 1},
		{"a name the server stopped offering", `{"Order": ["Gone", "A"], "Disabled": []}`, map[string][]string{"": {"A"}}, `["Gone","A"]`, 1},
		{"neither key", `{"Name": "n"}`, nil, `<null>`, 0},
		{"names not listed", `{"Order": ["B", "A"], "Disabled": ["A"]}`, nil, `["B"]`, 1},
	} {
		var asked []string
		ctx := WithAvailable(context.Background(), offering(c.offered, &asked))
		got, d := b.Flatten(ctx, doc(t, c.served), types.ObjectNull(b.AttrTypes))
		if d.HasError() {
			t.Fatalf("%s: %v", c.name, d)
		}
		if s := at(t, got, "enabled").String(); s != c.want || len(asked) != c.asks {
			t.Errorf("%s: enabled = %s after %d lookups, want %s after %d", c.name, s, len(asked), c.want, c.asks)
		}
	}

	failing := WithAvailable(context.Background(), func(context.Context, string, string) ([]string, error) { return nil, errors.New("offline") })
	if _, d := b.Flatten(failing, doc(t, `{"Order": []}`), types.ObjectNull(b.AttrTypes)); !d.HasError() || !strings.Contains(d[0].Detail(), "offline") {
		t.Errorf("a failed lookup is not reported: %v", d)
	}
	if _, d := b.Flatten(context.Background(), doc(t, `{"Order": []}`), types.ObjectNull(b.AttrTypes)); !d.HasError() || !strings.Contains(d[0].Detail(), "bug in the provider") {
		t.Errorf("a read without an AvailableFunc is not reported: %v", d)
	}
}

func TestUnitComplementRoundTripsInARebuiltElement(t *testing.T) {
	b := sharedBinding(t)
	var asked []string
	ctx := WithAvailable(context.Background(), offering(map[string][]string{"Movie": {"A", "B", "C"}}, &asked))
	model, d := b.Flatten(ctx, doc(t, `{"Opts": [{"ItemType": "Movie", "FetcherOrder": [], "DisabledFetchers": []}]}`), types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	if s := at(t, model, "opts[0].fetchers").String(); s != `["A","B","C"]` {
		t.Fatalf("a fresh entry reads %s, want every offered name", s)
	}
	m := with(t, model, "opts[0].fetchers", strs("C", "A"))
	m = with(t, m, "opts[0].fetcher_order", types.ListUnknown(types.StringType))
	m = with(t, m, "opts[0].disabled_fetchers", types.ListUnknown(types.StringType))
	written := doc(t, `{}`)
	if d := b.Overlay(ctx, written, object(t, m)); d.HasError() {
		t.Fatal(d)
	}
	if s := canonical(t, written); s != `{"Opts":[{"DisabledFetchers":["B"],"FetcherOrder":["C","A","B"],"ItemType":"Movie"}]}` {
		t.Errorf("wrote %s", s)
	}
	read, d := b.Flatten(ctx, written, types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	if s := at(t, read, "opts[0].fetchers").String(); s != `["C","A"]` {
		t.Errorf("reads back %s, want the written names", s)
	}
	if strings.Join(asked, " ") != "Fetchers/Movie Fetchers/Movie Fetchers/Movie" {
		t.Errorf("asked for %q, want the entry's item type each time", asked)
	}
}

func TestUnitComplementRejectsWhatItCannotWrite(t *testing.T) {
	b := sharedBinding(t)
	var asked []string
	ctx := WithAvailable(context.Background(), offering(map[string][]string{"Movie": {"A", "B"}}, &asked))
	model, d := b.Flatten(ctx, doc(t, `{"Opts": [{"ItemType": "Movie", "FetcherOrder": ["A"]}]}`), types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	unknown := func(m attr.Value) attr.Value {
		m = with(t, m, "opts[0].fetcher_order", types.ListUnknown(types.StringType))
		return with(t, m, "opts[0].disabled_fetchers", types.ListUnknown(types.StringType))
	}
	for _, c := range []struct {
		name string
		m    attr.Value
		want string
	}{
		{"a name not offered", with(t, unknown(model), "opts[0].fetchers", strs("A", "a")),
			`opts[0].fetchers lists "a", which is not one of the Fetchers the Jellyfin server offers for Movie: "A", "B".`},
		{"a scope the server does not list", with(t, with(t, unknown(model), "opts[0].fetchers", strs("A")), "opts[0].item_type", types.StringValue("Person")),
			"does not list the Fetchers it offers for Person, so opts[0].fetchers cannot tell which to disable. Set fetcher_order and disabled_fetchers instead."},
		{"an unknown scope", with(t, with(t, unknown(model), "opts[0].fetchers", strs("A")), "opts[0].item_type", types.StringUnknown()),
			"opts[0].fetchers is written for the item_type next to it, so set item_type."},
	} {
		d := b.Overlay(ctx, doc(t, `{}`), object(t, c.m))
		if !d.HasError() || !strings.Contains(d[0].Detail(), c.want) {
			t.Errorf("%s: %v\nwant an error containing %q", c.name, d, c.want)
		}
		if pe, ok := d[0].(diag.DiagnosticWithPath); !ok || pe.Path().String() != "opts[0].fetchers" {
			t.Errorf("%s: the error is not at opts[0].fetchers: %v", c.name, d[0])
		}
	}
}

func TestUnitComplementVersionErrorNamesTheKeyItTakesItsVersionFrom(t *testing.T) {
	b := sharedBinding(t)
	model, d := b.Flatten(WithAvailable(context.Background(), offering(map[string][]string{"Movie": {"A"}}, new([]string))),
		doc(t, `{"Opts": [{"ItemType": "Movie", "FetcherOrder": ["A"], "DisabledFetchers": []}]}`), types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	m := with(t, model, "opts[0].fetcher_order", types.ListNull(types.StringType))
	m = with(t, m, "opts[0].disabled_fetchers", types.ListNull(types.StringType))
	m = with(t, m, "types", types.ListNull(b.AttrTypes["types"].(types.ListType).ElemType))
	diags := b.VersionErrors(context.Background(), configOf(t, sharedAttrs(), m), func() (string, error) { return "1.9", nil })
	want := "opts[0].fetchers | Unsupported Jellyfin server version | opts[0].fetchers requires Jellyfin 2.0 or later: the server runs Jellyfin 1.9, which has no FetcherOrder field, so it would discard the value. Remove opts[0].fetchers from the configuration or upgrade the server."
	if got := strings.Join(versionErrorLines(diags), "\n"); got != want {
		t.Errorf("VersionErrors:\n%s\nwant:\n%s", got, want)
	}
}

func TestUnitBindRejectsSharedKeysItCannotWrite(t *testing.T) {
	neither := []Option{MergeByKey("types", "type"), NeverSent("enabled", "unbound"), NeverSent("opts.fetchers", "unbound")}
	for name, c := range map[string]struct {
		opts []Option
		want string
	}{
		"an order attribute the object lacks": {
			opts: append(neither, Orders("types.fetchers", "nope")),
			want: `types.fetchers: the order attribute "nope" is no attribute of the same object`,
		},
		"an order attribute that is no list": {
			opts: append(neither, Orders("types.fetchers", "type")),
			want: "types.fetchers: the order attribute types.type must be a configurable",
		},
		"ordering a string": {
			opts: append(neither, Orders("name", "order")),
			want: "name: Orders and Complement need a list of strings",
		},
		"ordering twice": {
			opts: append(neither, Orders("types.fetchers", "fetcher_order"), Orders("types.fetchers", "fetcher_order")),
			want: "types.fetchers is given Orders or Complement twice",
		},
		"orders on a keyless attribute": {
			opts: []Option{MergeByKey("types", "type"), NeverSent("opts.fetchers", "unbound"), NeverSent("enabled", "unbound"), Orders("enabled", "order")},
			want: "enabled is never sent, so it takes no Orders option",
		},
		"two attributes on one key": {
			opts: []Option{MergeByKey("types", "type"), NeverSent("opts.fetchers", "unbound"),
				Complement("enabled", "order", "disabled", "Fetchers", ""), Orders("disabled", "order")},
			want: "disabled and enabled both write the key of order",
		},
		"a complement with a key of its own": {
			opts: append(sharedOptions(), Key("enabled", "Name")),
			want: "enabled is complement, so it takes no key",
		},
		"a complement without an offered list or a string scope": {
			opts: []Option{MergeByKey("types", "type"), NeverSent("enabled", "unbound"),
				Complement("opts.fetchers", "fetcher_order", "disabled_fetchers", " ", "fetcher_order")},
			want: "opts.fetchers: Complement needs the name of the offered list\nopts.fetchers: the scope attribute opts.fetcher_order must be a configurable basetypes.StringType",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := sharedCatalog().bind(schema.Schema{Attributes: sharedAttrs()}, "Lib", c.opts...)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %v\nwant it to contain %q", err, c.want)
			}
		})
	}
}

func TestUnitComplementFunctions(t *testing.T) {
	order, disabled := complementOf([]string{"b", "A"}, []string{"A", "B", "C"})
	if got := fmt.Sprint(order, disabled); got != "[b A C] [C]" {
		t.Errorf("complementOf = %s", got)
	}
	if got := fmt.Sprint(enabledOf(order, disabled, []string{"A", "B", "C"})); got != "[b A]" {
		t.Errorf("enabledOf(complementOf) = %s, want the names back", got)
	}
	if got := fmt.Sprint(ordered([]string{"C"}, []string{"a", "c", "B"})); got != "[C a B]" {
		t.Errorf("ordered = %s", got)
	}
	if got, _ := marshal(ordered(nil, nil)); string(got) != "[]" {
		t.Errorf("an empty order encodes as %s", got)
	}
}
