// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

func TestUnitLibraryOptionsRoundTrip(t *testing.T) {
	ctx := context.Background()
	b := testUnitLibraryOptionsWire(t)
	fixture := `{
		"Enabled": false,
		"EnablePhotos": true,
		"EnableRealtimeMonitor": true,
		"ExtractChapterImagesDuringLibraryScan": true,
		"EnableChapterImageExtraction": false,
		"PathInfos": [
			{"Path": "/media", "NetworkPath": "\\\\server\\media"}
		],
		"PreferredMetadataLanguage": "en",
		"MetadataCountryCode": "US",
		"LocalMetadataReaderOrder": ["Nfo"],
		"DisabledSubtitleFetchers": ["OpenSubtitles"],
		"SubtitleFetcherOrder": ["OpenSubtitles"],
		"SaveLocalMetadata": true,
		"EnableAutomaticSeriesGrouping": false,
		"SeasonZeroDisplayName": "Specials",
		"TypeOptions": [
			{
				"Type": "Movie",
				"MetadataFetchers": ["TheMovieDb"],
				"MetadataFetcherOrder": ["TheMovieDb", "The Open Movie Database"],
				"ImageFetchers": ["TheMovieDb"],
				"ImageOptions": [{"Type": "Backdrop", "Limit": 3, "MinWidth": 1280}],
				"ImageFetcherOrder": ["TheMovieDb"],
				"SimilarItemProviders": ["Local Genre/Tag"],
				"SimilarItemProviderOrder": ["Local Genre/Tag", "TheMovieDb"]
			}
		]
	}`

	data := testUnitLibraryRead(t, b, fixture)
	if !data.LibraryOptions.Disabled.ValueBool() {
		t.Errorf("disabled = %v, want true for Enabled false", data.LibraryOptions.Disabled)
	}

	base := map[string]json.RawMessage{}
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("write: %v", d)
	}
	testUnitAssertJSONEqual(t, mustJSON(base), fixture)
}

func TestUnitTypeOptionsWriteKeepsUnsetServerValues(t *testing.T) {
	ctx := context.Background()
	b := testUnitLibraryOptionsWire(t)
	base := map[string]json.RawMessage{
		"TypeOptions": json.RawMessage(`[
			{
				"Type": "Trailer",
				"MetadataFetchers": ["TheMovieDb"]
			},
			{
				"Type": "Movie",
				"MetadataFetchers": ["The Open Movie Database"],
				"MetadataFetcherOrder": ["TheMovieDb", "The Open Movie Database"],
				"ImageFetchers": ["TheMovieDb"],
				"ImageFetcherOrder": ["TheMovieDb"],
				"ImageOptions": [{"Type": "Backdrop", "Limit": 1, "MinWidth": 1280}, {"Type": "Primary", "Limit": 1, "MinWidth": 0}],
				"SimilarItemProviders": ["Local Genre/Tag"],
				"SimilarItemProviderOrder": ["Local Genre/Tag", "TheMovieDb"]
			}
		]`),
	}

	data := testUnitLibraryRead(t, b, `{"TypeOptions": [{
		"Type": "movie",
		"MetadataFetchers": ["TheMovieDb"],
		"ImageOptions": [{"Type": "Backdrop", "Limit": 2}]
	}]}`)
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("write: %v", d)
	}

	want := `[{
		"Type": "movie",
		"MetadataFetchers": ["TheMovieDb"],
		"MetadataFetcherOrder": ["TheMovieDb", "The Open Movie Database"],
		"ImageFetchers": ["TheMovieDb"],
		"ImageFetcherOrder": ["TheMovieDb"],
		"ImageOptions": [{"Type": "Backdrop", "Limit": 2, "MinWidth": 1280}],
		"SimilarItemProviders": ["Local Genre/Tag"],
		"SimilarItemProviderOrder": ["Local Genre/Tag", "TheMovieDb"]
	}]`
	testUnitAssertJSONEqual(t, base["TypeOptions"], want)
}

func TestUnitTypeOptionsWithoutSimilarItemKeysReadAsNull(t *testing.T) {
	data := testUnitLibraryRead(t, testUnitLibraryOptionsWire(t), `{"TypeOptions": [{"Type": "Movie", "MetadataFetchers": ["TheMovieDb"]}]}`)

	var entries []TypeOptionsModel
	if d := data.LibraryOptions.TypeOptions.ElementsAs(context.Background(), &entries, false); d.HasError() {
		t.Fatalf("elements: %v", d)
	}
	for name, v := range map[string]types.List{
		"similar_item_providers":      entries[0].SimilarItemProviders,
		"similar_item_provider_order": entries[0].SimilarItemProviderOrder,
	} {
		if !v.IsNull() || v.IsUnknown() {
			t.Errorf("%s = %v, want a known null", name, v)
		}
	}
}

func TestUnitPlanTypeOptionsByTypeUsesPriorEntryOfSameType(t *testing.T) {
	ctx := context.Background()

	priorMovie := testUnitTypeOptions("Movie")
	priorMovie.MetadataFetchers = testUnitStringList(t, "The Open Movie Database")
	priorMovie.ImageFetchers = testUnitStringList(t, "TheMovieDb")
	priorMovie.SimilarItemProviders = testUnitStringList(t, "Local Genre/Tag")
	priorMovie.ImageOptions = testUnitList(t, imageOptionsObjectType(), []ImageOptionsModel{
		{Type: types.StringValue("Backdrop"), Limit: types.Int64Value(1), MinWidth: types.Int64Value(1280)},
	})
	state := testUnitList(t, typeOptionsObjectType(), []TypeOptionsModel{priorMovie})

	configTrailer := testUnitTypeOptions("Trailer")
	configMovie := testUnitTypeOptions("Movie")
	configMovie.MetadataFetchers = testUnitStringList(t, "TheMovieDb")
	configMovie.ImageOptions = testUnitList(t, imageOptionsObjectType(), []ImageOptionsModel{
		{Type: types.StringValue("Backdrop"), Limit: types.Int64Value(2), MinWidth: types.Int64Null()},
		{Type: types.StringValue("Logo"), Limit: types.Int64Value(1), MinWidth: types.Int64Null()},
	})
	config := testUnitList(t, typeOptionsObjectType(), []TypeOptionsModel{configTrailer, configMovie})

	// What UseStateForUnknown plans: the prior entry at the same index.
	planTrailer := priorMovie
	planTrailer.Type = types.StringValue("Trailer")
	planMovie := configMovie
	planMovie.ImageOptions = testUnitList(t, imageOptionsObjectType(), []ImageOptionsModel{
		{Type: types.StringValue("Backdrop"), Limit: types.Int64Value(2), MinWidth: types.Int64Value(1280)},
		{Type: types.StringValue("Logo"), Limit: types.Int64Value(1), MinWidth: types.Int64Null()},
	})
	plan := testUnitList(t, typeOptionsObjectType(), []TypeOptionsModel{planTrailer, planMovie})

	got, diags := planTypeOptionsByType(ctx, config, plan, state)
	if diags.HasError() {
		t.Fatalf("plan: %v", diags)
	}
	var entries []TypeOptionsModel
	if d := got.ElementsAs(ctx, &entries, false); d.HasError() {
		t.Fatalf("elements: %v", d)
	}

	if !entries[0].ImageFetchers.IsUnknown() || !entries[0].SimilarItemProviders.IsUnknown() || !entries[0].ImageOptions.IsUnknown() {
		t.Errorf("trailer entry without a prior entry of its type: got %+v, want unset attributes unknown", entries[0])
	}
	if !entries[1].MetadataFetchers.Equal(configMovie.MetadataFetchers) {
		t.Errorf("movie metadata_fetchers = %v, want the configured value", entries[1].MetadataFetchers)
	}
	if !entries[1].ImageFetchers.Equal(priorMovie.ImageFetchers) || !entries[1].SimilarItemProviders.Equal(priorMovie.SimilarItemProviders) {
		t.Errorf("movie entry: got %+v, want unset attributes from the prior movie entry", entries[1])
	}

	var images []ImageOptionsModel
	if d := entries[1].ImageOptions.ElementsAs(ctx, &images, false); d.HasError() {
		t.Fatalf("image options: %v", d)
	}
	if images[0].Limit.ValueInt64() != 2 || images[0].MinWidth.ValueInt64() != 1280 {
		t.Errorf("backdrop = %+v, want the configured limit and the prior min_width", images[0])
	}
	if !images[1].MinWidth.IsUnknown() {
		t.Errorf("logo min_width = %v, want unknown without a prior logo entry", images[1].MinWidth)
	}
}

func TestUnitLibraryReadAfterApplyKeepsPlannedNullTypeOptionsLists(t *testing.T) {
	ctx := context.Background()
	b := testUnitLibraryOptionsWire(t)
	data := testUnitLibraryRead(t, b, `{"TypeOptions": [{"Type": "Movie"}]}`)

	served := `{"TypeOptions": [{"Type": "Movie", "MetadataFetcherOrder": [], "SimilarItemProviders": [], "SimilarItemProviderOrder": ["TheMovieDb"]}]}`
	if d := b.FlattenAfterApply(ctx, served, &data); d.HasError() {
		t.Fatalf("read after apply: %v", d)
	}

	var entries []TypeOptionsModel
	if d := data.LibraryOptions.TypeOptions.ElementsAs(ctx, &entries, false); d.HasError() {
		t.Fatalf("elements: %v", d)
	}
	if !entries[0].MetadataFetcherOrder.IsNull() || !entries[0].SimilarItemProviders.IsNull() {
		t.Errorf("empty lists planned null: got %+v, want null", entries[0])
	}
	if entries[0].SimilarItemProviderOrder.IsNull() {
		t.Errorf("similar_item_provider_order = null, want the server's non-empty value kept")
	}
}

func TestUnitLibraryReadAfterApplyReportsDroppedSimilarItemSettings(t *testing.T) {
	ctx := context.Background()
	b := testUnitLibraryOptionsWire(t)
	planned := `{"TypeOptions": [{"Type": "Movie", "SimilarItemProviders": ["Local Genre/Tag"], "SimilarItemProviderOrder": ["Local Genre/Tag"]}]}`

	kept := testUnitLibraryRead(t, b, planned)
	if d := b.FlattenAfterApply(ctx, planned, &kept); d.HasError() {
		t.Fatalf("settings the server kept: %v", d)
	}

	dropped := testUnitLibraryRead(t, b, planned)
	d := b.FlattenAfterApply(ctx, `{"TypeOptions": [{"Type": "Movie"}]}`, &dropped)
	var paths []string
	for _, e := range d.Errors() {
		if withPath, ok := e.(diag.DiagnosticWithPath); ok {
			paths = append(paths, withPath.Path().String())
		}
	}
	want := []string{
		"library_options.type_options[0].similar_item_provider_order",
		"library_options.type_options[0].similar_item_providers",
	}
	if !slices.Equal(paths, want) {
		t.Errorf("settings the server dropped: errors at %v, want %v", paths, want)
	}
}

func TestUnitImageOptionTypeAcceptsOnlyJellyfinSpelling(t *testing.T) {
	typeAttr, ok := imageOptionsAttributes()["type"].(schema.StringAttribute)
	if !ok {
		t.Fatal("image_options type is not a string attribute")
	}
	testUnitAssertStringValidation(t, typeAttr, map[string]bool{
		"Backdrop": false,
		"BoxRear":  false,
		"backdrop": true,
		"Boxrear":  true,
		"Poster":   true,
	})
}

func TestUnitCollectionTypeAcceptsOnlyJellyfinSpelling(t *testing.T) {
	resp := resource.SchemaResponse{}
	NewLibraryResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	collectionType, ok := resp.Schema.Attributes["collection_type"].(schema.StringAttribute)
	if !ok {
		t.Fatal("collection_type is not a string attribute")
	}
	testUnitAssertStringValidation(t, collectionType, map[string]bool{
		"movies":      false,
		"musicvideos": false,
		"mixed":       false,
		"Movies":      true,
		"musicVideos": true,
		"photos":      true,
	})
}

func testUnitAssertStringValidation(t *testing.T, a schema.StringAttribute, expectError map[string]bool) {
	t.Helper()
	for value, want := range expectError {
		resp := validator.StringResponse{}
		for _, v := range a.Validators {
			v.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("value"),
				ConfigValue: types.StringValue(value),
			}, &resp)
		}
		if resp.Diagnostics.HasError() != want {
			t.Errorf("%q: expected error %t, got diagnostics: %v", value, want, resp.Diagnostics)
		}
	}
}

func TestUnitUnsupportedLibraryOptionsAreNotComputed(t *testing.T) {
	attrs := map[string]schema.Attribute{}
	maps.Copy(attrs, libraryOptionsAttributes())
	for name, a := range pathInfoAttributes() {
		attrs["path_infos."+name] = a
	}

	unsupported := 0
	for name, a := range attrs {
		if a.GetDeprecationMessage() != unsupportedLibraryOptionMessage {
			continue
		}
		unsupported++
		if a.IsComputed() {
			t.Errorf("%s is computed, so create plans show it as known after apply although it always reads as null", name)
		}
	}
	if unsupported == 0 {
		t.Fatal("found no unsupported library options")
	}
}

func TestUnitNetworkPathPlansPriorValueWhenUnset(t *testing.T) {
	a, ok := pathInfoAttributes()["network_path"].(schema.StringAttribute)
	if !ok {
		t.Fatal("network_path is not a string attribute")
	}

	tests := map[string]struct {
		config, plan, state, want types.String
	}{
		"unset without a prior value": {
			config: types.StringNull(), plan: types.StringUnknown(), state: types.StringNull(),
			want: types.StringNull(),
		},
		"unset with a prior value": {
			config: types.StringNull(), plan: types.StringUnknown(), state: types.StringValue("smb://nas/movies"),
			want: types.StringValue("smb://nas/movies"),
		},
		"configured": {
			config: types.StringValue("smb://nas/films"), plan: types.StringValue("smb://nas/films"), state: types.StringValue("smb://nas/movies"),
			want: types.StringValue("smb://nas/films"),
		},
	}
	for name, test := range tests {
		resp := planmodifier.StringResponse{PlanValue: test.plan}
		for _, m := range a.PlanModifiers {
			m.PlanModifyString(context.Background(), planmodifier.StringRequest{
				ConfigValue: test.config,
				PlanValue:   resp.PlanValue,
				StateValue:  test.state,
			}, &resp)
		}
		if !resp.PlanValue.Equal(test.want) {
			t.Errorf("%s: planned %v, want %v", name, resp.PlanValue, test.want)
		}
	}
}

func TestUnitFlattenCollectionTypeReadsMissingTypeAsMixed(t *testing.T) {
	for server, want := range map[string]string{
		"":       "mixed",
		"mixed":  "mixed",
		"movies": "movies",
	} {
		if got := flattenCollectionType(server); got != types.StringValue(want) {
			t.Errorf("flattenCollectionType(%q) = %v, want %q", server, got, want)
		}
	}
}

func TestUnitCollectionTypeChangeRequiresReplaceExceptEmptyToMixed(t *testing.T) {
	schemaResp := resource.SchemaResponse{}
	NewLibraryResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	collectionType, ok := schemaResp.Schema.Attributes["collection_type"].(schema.StringAttribute)
	if !ok {
		t.Fatal("collection_type is not a string attribute")
	}
	existing := tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{})

	for _, test := range []struct {
		state, plan string
		want        bool
	}{
		{state: "", plan: "mixed", want: false},
		{state: "", plan: "movies", want: true},
		{state: "movies", plan: "mixed", want: true},
		{state: "mixed", plan: "movies", want: true},
	} {
		resp := planmodifier.StringResponse{PlanValue: types.StringValue(test.plan)}
		for _, m := range collectionType.PlanModifiers {
			m.PlanModifyString(context.Background(), planmodifier.StringRequest{
				State:       tfsdk.State{Raw: existing},
				Plan:        tfsdk.Plan{Raw: existing},
				ConfigValue: types.StringValue(test.plan),
				PlanValue:   types.StringValue(test.plan),
				StateValue:  types.StringValue(test.state),
			}, &resp)
		}
		if resp.RequiresReplace != test.want {
			t.Errorf("%q -> %q: requires replace %t, want %t", test.state, test.plan, resp.RequiresReplace, test.want)
		}
	}
}

func TestUnitLibraryVersionErrorsFollowServerVersion(t *testing.T) {
	ctx := context.Background()
	similarItems := path.Root("library_options").AtName("type_options").AtListIndex(0).AtName("similar_item_providers")
	networkPath := path.Root("library_options").AtName("path_infos").AtListIndex(0).AtName("network_path")
	root, config := testUnitLibraryGatedConfig(t)

	tests := map[string]struct {
		version      string
		similarError bool
		networkError bool
	}{
		"10.9":        {version: "10.9.11", similarError: true},
		"10.10":       {version: "10.10.0", similarError: true, networkError: true},
		"10.11":       {version: "10.11.11", similarError: true, networkError: true},
		"12.1":        {version: "12.1.0", networkError: true},
		"unparseable": {version: "unknown"},
	}
	for name, test := range tests {
		diags := root.VersionErrors(ctx, config, func() (string, error) { return test.version, nil })
		gotSimilar, gotNetwork := false, false
		for _, d := range diags {
			withPath, ok := d.(diag.DiagnosticWithPath)
			if !ok {
				t.Errorf("%s: diagnostic without a path: %v", name, d)
				continue
			}
			gotSimilar = gotSimilar || withPath.Path().Equal(similarItems)
			gotNetwork = gotNetwork || withPath.Path().Equal(networkPath)
		}
		if gotSimilar != test.similarError || gotNetwork != test.networkError {
			t.Errorf("%s: similar item error %t, network path error %t; want %t, %t", name, gotSimilar, gotNetwork, test.similarError, test.networkError)
		}
	}
}

// CI runs the acceptance tests on Jellyfin 12, which never reports the similar
// item error, so no other test there sees its wording.
func TestUnitLibraryVersionErrorWording(t *testing.T) {
	root, config := testUnitLibraryGatedConfig(t)

	diags := root.VersionErrors(context.Background(), config, func() (string, error) { return "10.11.11", nil })
	var got []string
	for _, d := range diags {
		withPath, ok := d.(diag.DiagnosticWithPath)
		if !ok {
			t.Fatalf("diagnostic without a path: %v", d)
		}
		got = append(got, withPath.Path().String()+" | "+d.Summary()+" | "+d.Detail())
	}
	slices.Sort(got)
	want := []string{
		"library_options.path_infos[0].network_path | Network paths not supported | The server runs Jellyfin 10.11.11, and Jellyfin 10.10 removed network paths, so the server would drop the value. Remove library_options.path_infos[0].network_path from the configuration.",
		"library_options.type_options[0].similar_item_providers | Similar item settings not supported | The server runs Jellyfin 10.11.11, and similar item providers need Jellyfin 12 or later. Remove library_options.type_options[0].similar_item_providers for this server.",
	}
	if !slices.Equal(got, want) {
		t.Errorf("version errors:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// testUnitLibraryGatedConfig configures a similar item provider and a network
// path, which some Jellyfin versions lack.
func testUnitLibraryGatedConfig(t *testing.T) (*wire.Binding, tfsdk.Config) {
	t.Helper()
	ctx := context.Background()
	root, err := libraryWire()
	if err != nil {
		t.Fatal(err)
	}
	data := testUnitLibraryRead(t, testUnitLibraryOptionsWire(t),
		`{"TypeOptions": [{"Type": "Movie", "SimilarItemProviders": ["Local Genre/Tag"]}], "PathInfos": [{"Path": "/media", "NetworkPath": "//nas/media"}]}`)
	obj, d := types.ObjectValueFrom(ctx, root.AttrTypes, &data)
	if d.HasError() {
		t.Fatal(d)
	}
	raw, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return root, tfsdk.Config{Schema: schemaOf(NewLibraryResource()), Raw: raw}
}

func testUnitTypeOptions(typ string) TypeOptionsModel {
	null := types.ListNull(types.StringType)
	return TypeOptionsModel{
		Type:                     types.StringValue(typ),
		MetadataFetchers:         null,
		MetadataFetcherOrder:     null,
		ImageFetchers:            null,
		ImageOptions:             types.ListNull(imageOptionsObjectType()),
		ImageFetcherOrder:        null,
		SimilarItemProviders:     null,
		SimilarItemProviderOrder: null,
	}
}

func testUnitStringList(t *testing.T, values ...string) types.List {
	t.Helper()
	if values == nil {
		values = []string{}
	}
	return testUnitList(t, types.StringType, values)
}

func testUnitList(t *testing.T, elemType attr.Type, elements any) types.List {
	t.Helper()
	list, d := types.ListValueFrom(context.Background(), elemType, elements)
	if d.HasError() {
		t.Fatalf("building list: %v", d)
	}
	return list
}

func testUnitAssertJSONEqual(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	gotJSON, _ := json.Marshal(g)
	wantJSON, _ := json.Marshal(w)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("JSON mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func testUnitLibraryOptionsWire(t *testing.T) *wire.Binding {
	t.Helper()
	b, err := libraryOptionsWire()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// testUnitLibraryRead reads options into a library whose other attributes
// are set, as the resource's model always has them.
func testUnitLibraryRead(t *testing.T, b *wire.Binding, options string) LibraryResourceModel {
	t.Helper()
	data := LibraryResourceModel{
		ID:             types.StringValue("Movies"),
		Name:           types.StringValue("Movies"),
		CollectionType: types.StringValue("movies"),
		Paths:          testUnitStringList(t, "/media"),
		ItemID:         types.StringValue("item"),
	}
	if d := b.FlattenInto(context.Background(), options, &data); d.HasError() {
		t.Fatalf("read: %v", d)
	}
	return data
}

func typeOptionsObjectType() types.ObjectType { return testUnitObjectType(typeOptionsAttributes()) }

func imageOptionsObjectType() types.ObjectType { return testUnitObjectType(imageOptionsAttributes()) }

func testUnitObjectType(attrs map[string]schema.Attribute) types.ObjectType {
	t := types.ObjectType{AttrTypes: map[string]attr.Type{}}
	for name, a := range attrs {
		t.AttrTypes[name] = a.GetType()
	}
	return t
}
