// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnitLibraryOptionsOverlay(t *testing.T) {
	ctx := context.Background()
	fixture := `{
		"EnablePhotos": true,
		"EnableRealtimeMonitor": true,
		"EnableEmbiPhotos": false,
		"EnablePhotoSubtitle": false,
		"ExtractChaptersDuringLibraryScan": false,
		"EnableChapterImageExtraction": false,
		"ChapterImageIntervalSeconds": 100,
		"ExtractMediaInformationDuringLibraryScan": true,
		"DownloadImagesInAdvance": false,
		"CacheImagesInLibrary": true,
		"EnableMediaConversion": false,
		"PathInfos": [
			{"Path": "/media", "NetworkPath": "\\\\server\\media", "Username": "user", "Password": "pass"}
		],
		"PreferredMetadataLanguage": "en",
		"MetadataCountryCode": "US",
		"DisabledMetadataSavers": ["Nfo"],
		"LocalMetadataReaderOrder": ["Nfo"],
		"DisabledMetadataFetchers": ["TheMovieDb"],
		"MetadataFetcherOrder": ["TheMovieDb"],
		"DisabledImageFetchers": ["TheMovieDb"],
		"ImageFetcherOrder": ["TheMovieDb"],
		"DisabledSubtitleFetchers": ["OpenSubtitles"],
		"SubtitleFetcherOrder": ["OpenSubtitles"],
		"SaveLocalMetadata": true,
		"SaveLocalThumbnailSets": true,
		"ImportMissingEpisodes": true,
		"EnableAutomaticSeriesGrouping": false,
		"SeasonZeroDisplayName": "Specials",
		"MetadataRefreshMode": "Default",
		"Disabled": false,
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

	var diags diag.Diagnostics
	data := flattenLibraryOptions(ctx, fixture, &diags)
	if diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}

	base := map[string]json.RawMessage{}
	if d := overlayLibraryOptions(ctx, base, data); d.HasError() {
		t.Fatalf("overlay: %v", d)
	}

	result, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	var want map[string]interface{}
	if err := json.Unmarshal([]byte(fixture), &want); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("round-trip mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestUnitTypeOptionsOverlayKeepsUnsetServerValues(t *testing.T) {
	ctx := context.Background()
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

	movie := testUnitTypeOptions("movie")
	movie.MetadataFetchers = testUnitStringList(t, "TheMovieDb")
	backdrop := ImageOptionsModel{Type: types.StringValue("backdrop"), Limit: types.Int64Value(2), MinWidth: types.Int64Null()}
	movie.ImageOptions = testUnitList(t, imageOptionsObjectType(), []ImageOptionsModel{backdrop})

	if d := overlayTypeOptions(ctx, base, testUnitList(t, typeOptionsObjectType(), []TypeOptionsModel{movie})); d.HasError() {
		t.Fatalf("overlay: %v", d)
	}

	want := `[{
		"Type": "movie",
		"MetadataFetchers": ["TheMovieDb"],
		"MetadataFetcherOrder": ["TheMovieDb", "The Open Movie Database"],
		"ImageFetchers": ["TheMovieDb"],
		"ImageFetcherOrder": ["TheMovieDb"],
		"ImageOptions": [{"Type": "backdrop", "Limit": 2, "MinWidth": 1280}],
		"SimilarItemProviders": ["Local Genre/Tag"],
		"SimilarItemProviderOrder": ["Local Genre/Tag", "TheMovieDb"]
	}]`
	testUnitAssertJSONEqual(t, base["TypeOptions"], want)
}

func TestUnitFlattenTypeOptionsWithoutSimilarItemKeysIsNull(t *testing.T) {
	var diags diag.Diagnostics
	opts := flattenLibraryOptions(context.Background(), `{"TypeOptions": [{"Type": "Movie", "MetadataFetchers": ["TheMovieDb"]}]}`, &diags)
	if diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}

	var entries []TypeOptionsModel
	if d := opts.TypeOptions.ElementsAs(context.Background(), &entries, false); d.HasError() {
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

func TestUnitKeepPlannedNullsCoversTypeOptionsLists(t *testing.T) {
	ctx := context.Background()
	planned := testUnitTypeOptions("Movie")
	got := testUnitTypeOptions("Movie")
	got.MetadataFetcherOrder = testUnitStringList(t)
	got.SimilarItemProviders = testUnitStringList(t)
	got.SimilarItemProviderOrder = testUnitStringList(t, "TheMovieDb")

	out := keepPlannedNulls(ctx,
		&LibraryOptionsModel{TypeOptions: testUnitList(t, typeOptionsObjectType(), []TypeOptionsModel{planned}), PathInfos: types.ListNull(pathInfoObjectType())},
		&LibraryOptionsModel{TypeOptions: testUnitList(t, typeOptionsObjectType(), []TypeOptionsModel{got}), PathInfos: types.ListNull(pathInfoObjectType())},
	)

	var entries []TypeOptionsModel
	if d := out.TypeOptions.ElementsAs(ctx, &entries, false); d.HasError() {
		t.Fatalf("elements: %v", d)
	}
	if !entries[0].MetadataFetcherOrder.IsNull() || !entries[0].SimilarItemProviders.IsNull() {
		t.Errorf("empty lists planned null: got %+v, want null", entries[0])
	}
	if entries[0].SimilarItemProviderOrder.IsNull() {
		t.Errorf("similar_item_provider_order = null, want the server's non-empty value kept")
	}
}

func TestUnitCheckSimilarItemSettingsKeptReportsDroppedSettings(t *testing.T) {
	ctx := context.Background()
	planned := testUnitTypeOptions("Movie")
	planned.SimilarItemProviders = testUnitStringList(t, "Local Genre/Tag")
	planned.SimilarItemProviderOrder = testUnitStringList(t, "Local Genre/Tag")
	kept := planned
	dropped := testUnitTypeOptions("Movie")
	options := func(entry TypeOptionsModel) *LibraryOptionsModel {
		return &LibraryOptionsModel{TypeOptions: testUnitList(t, typeOptionsObjectType(), []TypeOptionsModel{entry})}
	}

	var diags diag.Diagnostics
	checkSimilarItemSettingsKept(ctx, options(planned), options(kept), &diags)
	if diags.HasError() {
		t.Fatalf("settings the server kept: %v", diags)
	}

	checkSimilarItemSettingsKept(ctx, options(planned), options(dropped), &diags)
	if !diags.HasError() {
		t.Fatal("settings the server dropped: no error")
	}
	for _, name := range []string{"type_options[0].similar_item_providers", "type_options[0].similar_item_provider_order"} {
		if !strings.Contains(diags[0].Detail(), name) {
			t.Errorf("error detail %q does not name %s", diags[0].Detail(), name)
		}
	}
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
