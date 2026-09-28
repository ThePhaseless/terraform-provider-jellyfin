// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

func TestUnitSystemConfigurationRoundTrip(t *testing.T) {
	ctx := context.Background()
	b, err := systemWire()
	if err != nil {
		t.Fatal(err)
	}
	fixture := `{
		"EnableMetrics": true,
		"EnableNormalizedItemByNameIds": false,
		"IsPortAuthorized": true,
		"QuickConnectAvailable": false,
		"EnableCaseSensitiveItemIds": false,
		"DisableLiveTvChannelUserDataName": false,
		"MetadataPath": "/metadata",
		"PreferredMetadataLanguage": "en",
		"MetadataCountryCode": "US",
		"SortReplaceCharacters": ["."],
		"SortRemoveCharacters": ["!", "?"],
		"SortRemoveWords": ["the"],
		"MinResumePct": 5,
		"MaxResumePct": 95,
		"MinResumeDurationSeconds": 300,
		"MinAudiobookResume": 5,
		"MaxAudiobookResume": 95,
		"InactiveSessionThreshold": 900,
		"LibraryMonitorDelay": 60,
		"LibraryUpdateDuration": 30,
		"CacheSize": 0,
		"ImageSavingConvention": "Compatible",
		"MetadataOptions": [
			{
				"ItemType": "Movie",
				"DisabledMetadataSavers": [],
				"LocalMetadataReaderOrder": [],
				"DisabledMetadataFetchers": [],
				"MetadataFetcherOrder": [],
				"DisabledImageFetchers": [],
				"ImageFetcherOrder": []
			}
		],
		"SkipDeserializationForBasicTypes": false,
		"UICulture": "en-US",
		"SaveMetadataHidden": false,
		"ContentTypes": [
			{"Name": "movies", "Value": "Movies"}
		],
		"RemoteClientBitrateLimit": 0,
		"EnableFolderView": false,
		"EnableGroupingMoviesIntoCollections": false,
		"EnableGroupingShowsIntoCollections": false,
		"DisplaySpecialsWithinSeasons": false,
		"CodecsUsed": ["h264", "hevc"],
		"EnableExternalContentInSuggestions": false,
		"ImageExtractionTimeoutMs": 10000,
		"PathSubstitutions": [
			{"From": "/mnt/media", "To": "/media"}
		],
		"EnableSlowResponseWarning": false,
		"SlowResponseThresholdMs": 500,
		"CorsHosts": ["*"],
		"ActivityLogRetentionDays": 7,
		"LibraryScanFanoutConcurrency": 1,
		"LibraryMetadataRefreshConcurrency": 1,
		"AllowClientLogUpload": false,
		"DummyChapterDuration": 0,
		"ChapterImageResolution": "MatchSource",
		"ParallelImageEncodingLimit": 0,
		"CastReceiverApplications": [
			{"Id": "ABCDEF", "Name": "Example"}
		],
		"TrickplayOptions": {
			"EnableHwAcceleration": false,
			"EnableHwEncoding": false,
			"EnableKeyFrameOnlyExtraction": false,
			"ScanBehavior": "NonBlocking",
			"ProcessPriority": "BelowNormal",
			"Interval": 10000,
			"WidthResolutions": [320],
			"TileWidth": 10,
			"TileHeight": 10,
			"Qscale": 4,
			"JpegQuality": 90,
			"ProcessThreads": 1
		},
		"EnableLegacyAuthorization": false,
		"LogFileRetentionDays": 3,
		"CachePath": "/cache",
		"ServerName": "My Jellyfin Server"
	}`

	offered := testUnitOfferingByItemType(map[string][]string{"MetadataFetchers/Movie": {"TheMovieDb"}, "ImageFetchers/Movie": {"TheMovieDb"}})
	data := readWireIn[SystemConfigurationResourceModel](offered, t, b, fixture)
	base := map[string]json.RawMessage{}
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("write: %v", d)
	}
	checkSameJSON(t, base, fixture)
}

func TestUnitSystemConfigurationEnumValidators(t *testing.T) {
	ctx := context.Background()

	var resp resource.SchemaResponse
	(&SystemConfigurationResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	trickplay, ok := resp.Schema.Attributes["trickplay_options"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("trickplay_options attribute type = %T, want schema.SingleNestedAttribute", resp.Schema.Attributes["trickplay_options"])
	}

	for _, tc := range []struct {
		path      path.Path
		attribute rschema.Attribute
		valid     string
		wrongCase string
	}{
		{path.Root("image_saving_convention"), resp.Schema.Attributes["image_saving_convention"], "Compatible", "compatible"},
		{path.Root("chapter_image_resolution"), resp.Schema.Attributes["chapter_image_resolution"], "P720", "p720"},
		{path.Root("trickplay_options").AtName("scan_behavior"), trickplay.Attributes["scan_behavior"], "Blocking", "blocking"},
		{path.Root("trickplay_options").AtName("process_priority"), trickplay.Attributes["process_priority"], "Idle", "idle"},
	} {
		attr, ok := tc.attribute.(rschema.StringAttribute)
		if !ok {
			t.Fatalf("%s attribute type = %T, want schema.StringAttribute", tc.path, tc.attribute)
		}

		for value, wantError := range map[string]bool{tc.valid: false, "": true, tc.wrongCase: true} {
			var diags diag.Diagnostics
			for _, v := range attr.Validators {
				vresp := validator.StringResponse{}
				v.ValidateString(ctx, validator.StringRequest{
					Path:        tc.path,
					ConfigValue: types.StringValue(value),
				}, &vresp)
				diags.Append(vresp.Diagnostics...)
			}

			if diags.HasError() != wantError {
				t.Errorf("%s = %q: got error %t, want %t: %v", tc.path, value, diags.HasError(), wantError, diags)
			}
		}
	}
}

func TestUnitSystemConfigurationEntriesAndRenamedKeysCopyOnlyNonNullPriorValues(t *testing.T) {
	ctx := context.Background()

	var resp resource.SchemaResponse
	(&SystemConfigurationResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	trickplay, ok := resp.Schema.Attributes["trickplay_options"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("trickplay_options attribute type = %T, want schema.SingleNestedAttribute", resp.Schema.Attributes["trickplay_options"])
	}

	attributes := map[string]rschema.Attribute{
		"enable_normalized_item_by_name_ids": resp.Schema.Attributes["enable_normalized_item_by_name_ids"],
		"enable_case_sensitive_item_ids":     resp.Schema.Attributes["enable_case_sensitive_item_ids"],
		"trickplay_options.process_priority": trickplay.Attributes["process_priority"],
	}
	for _, list := range []string{"metadata_options", "content_types", "path_substitutions", "cast_receiver_applications"} {
		nested, ok := resp.Schema.Attributes[list].(rschema.ListNestedAttribute)
		if !ok {
			t.Fatalf("%s attribute type = %T, want schema.ListNestedAttribute", list, resp.Schema.Attributes[list])
		}
		for name, a := range nested.NestedObject.Attributes {
			attributes[list+"[*]."+name] = a
		}
	}

	// UseStateForUnknown leaves the plan alone unless the resource has state.
	nulls := map[string]tftypes.Value{}
	for name, a := range resp.Schema.Attributes {
		nulls[name] = tftypes.NewValue(a.GetType().TerraformType(ctx), nil)
	}
	state := tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nulls)}
	config := tfsdk.Config(state)
	pathOf := func(name string) path.Path {
		if list, attr, ok := strings.Cut(name, "[*]."); ok {
			return path.Root(list).AtListIndex(0).AtName(attr)
		}
		if parent, attr, ok := strings.Cut(name, "."); ok {
			return path.Root(parent).AtName(attr)
		}
		return path.Root(name)
	}
	for name, a := range attributes {
		var plannedFromNull, plannedFromSet, set attr.Value
		switch a := a.(type) {
		case rschema.StringAttribute:
			plan := func(prior types.String) types.String {
				req := planmodifier.StringRequest{State: state, StateValue: prior, ConfigValue: types.StringNull(), PlanValue: types.StringUnknown()}
				resp := planmodifier.StringResponse{PlanValue: req.PlanValue}
				for _, m := range a.PlanModifiers {
					m.PlanModifyString(ctx, req, &resp)
				}
				return resp.PlanValue
			}
			set = types.StringValue("x")
			plannedFromNull, plannedFromSet = plan(types.StringNull()), plan(types.StringValue("x"))
		case rschema.BoolAttribute:
			plan := func(prior types.Bool) types.Bool {
				req := planmodifier.BoolRequest{State: state, StateValue: prior, ConfigValue: types.BoolNull(), PlanValue: types.BoolUnknown()}
				resp := planmodifier.BoolResponse{PlanValue: req.PlanValue}
				for _, m := range a.PlanModifiers {
					m.PlanModifyBool(ctx, req, &resp)
				}
				return resp.PlanValue
			}
			set = types.BoolValue(true)
			plannedFromNull, plannedFromSet = plan(types.BoolNull()), plan(types.BoolValue(true))
		case rschema.ListAttribute:
			plan := func(prior types.List) types.List {
				req := planmodifier.ListRequest{Path: pathOf(name), Config: config, State: state, StateValue: prior, ConfigValue: types.ListNull(a.ElementType), PlanValue: types.ListUnknown(a.ElementType)}
				resp := planmodifier.ListResponse{PlanValue: req.PlanValue}
				for _, m := range a.PlanModifiers {
					m.PlanModifyList(ctx, req, &resp)
				}
				return resp.PlanValue
			}
			setList := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("x")})
			set = setList
			plannedFromNull, plannedFromSet = plan(types.ListNull(a.ElementType)), plan(setList)
		default:
			t.Fatalf("%s attribute type = %T, want a string, bool or list attribute", name, a)
		}

		if !plannedFromNull.IsUnknown() {
			t.Errorf("%s: null prior value planned as %s, want unknown", name, plannedFromNull)
		}
		if !plannedFromSet.Equal(set) {
			t.Errorf("%s: prior value %s planned as %s, want it copied", name, set, plannedFromSet)
		}
	}
}

// testUnitOfferingByItemType returns a context whose server offers, for each
// "list/item type" key of offered, the names it maps to, and lists nothing
// for any other item type.
func testUnitOfferingByItemType(offered map[string][]string) context.Context {
	return wire.WithAvailable(context.Background(), func(_ context.Context, list, scope string) ([]string, error) {
		names, ok := offered[list+"/"+scope]
		if !ok {
			return nil, fmt.Errorf("item type %q: %w", scope, wire.ErrNotOffered)
		}
		return names, nil
	})
}

// testUnitServeAvailableOptions serves GET /Libraries/AvailableOptions from
// the documents byContentType holds, and counts the requests for each content
// type.
func testUnitServeAvailableOptions(byContentType map[string]string, asked map[string]int, mu *sync.Mutex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		contentType := r.URL.Query().Get("libraryContentType")
		asked[contentType]++
		doc, ok := byContentType[contentType]
		if !ok {
			doc = `{"TypeOptions": []}`
		}
		_, _ = io.WriteString(w, doc)
	}
}

func TestUnitOfferedProvidersByItemType(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	asked := map[string]int{}
	srv := httptest.NewServer(testUnitServeAvailableOptions(map[string]string{
		"tvshows": `{"TypeOptions": [{"Type": "Series", "MetadataFetchers": [{"Name": "TheMovieDb"}, {"Name": "The Open Movie Database"}], "ImageFetchers": [{"Name": "TheMovieDb"}]}]}`,
		"music":   `{"TypeOptions": [{"Type": "MusicAlbum", "MetadataFetchers": [{"Name": "MusicBrainz"}], "ImageFetchers": []}]}`,
	}, asked, &mu))
	t.Cleanup(srv.Close)
	o := newOfferedProviders(client.NewClient(srv.URL, "k"))

	for _, c := range []struct {
		list, scope string
		want        string
	}{
		{"MetadataFetchers", "series", "[TheMovieDb The Open Movie Database]"},
		{"ImageFetchers", "Series", "[TheMovieDb]"},
		{"MetadataFetchers", "MusicAlbum", "[MusicBrainz]"},
	} {
		got, err := o.byItemType(ctx, c.list, c.scope)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s of %s = %v (%v), want %s", c.list, c.scope, got, err, c.want)
		}
	}
	if _, err := o.byItemType(ctx, "MetadataFetchers", "Person"); !errors.Is(err, wire.ErrNotOffered) {
		t.Errorf("an item type no content type holds: %v, want wire.ErrNotOffered", err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, contentType := range contentTypesByItemType {
		if asked[contentType] != 1 {
			t.Errorf("asked for %s %d times, want once", contentType, asked[contentType])
		}
	}
}

func TestUnitSystemApplyWritesTheKeysItsFetchersShare(t *testing.T) {
	ctx := context.Background()
	const served = `{"ServerName": "s", "MetadataOptions": [{"ItemType": "Movie", "DisabledMetadataFetchers": ["The Open Movie Database"], "MetadataFetcherOrder": [], "DisabledImageFetchers": [], "ImageFetcherOrder": []}]}`
	var mu sync.Mutex
	var posted []byte
	asked := map[string]int{}
	available := testUnitServeAvailableOptions(map[string]string{
		"movies": `{"TypeOptions": [{"Type": "Movie", "MetadataFetchers": [{"Name": "TheMovieDb"}, {"Name": "The Open Movie Database"}], "ImageFetchers": [{"Name": "TheMovieDb"}, {"Name": "Screen Grabber"}]}]}`,
	}, asked, &mu)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Libraries/AvailableOptions" {
			available(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/System/Configuration":
			doc := served
			if posted != nil {
				doc = string(posted)
			}
			_, _ = io.WriteString(w, doc)
		case r.Method == http.MethodPost && r.URL.Path == "/System/Configuration":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			posted = body
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	r := NewSystemConfigurationResource()
	c := client.NewClient(srv.URL, "k")
	movie := path.Root("metadata_options").AtListIndex(0)
	state := readAgainst(t, r, c, tfsdk.State(planRead(t, r, `{"ServerName": "s", "MetadataOptions": [{"ItemType": "Movie"}]}`)))
	if state.Diagnostics.HasError() {
		t.Fatalf("read: %v", state.Diagnostics)
	}
	var enabled types.List
	if d := state.State.GetAttribute(ctx, movie.AtName("metadata_fetchers"), &enabled); d.HasError() || !enabled.Equal(testUnitStringList(t, "TheMovieDb")) {
		t.Errorf("metadata_fetchers = %v after a read (%v), want the offered fetcher not disabled", enabled, d)
	}

	plan := tfsdk.Plan(state.State)
	for _, set := range []planValue{
		{movie.AtName("metadata_fetchers"), testUnitStringList(t, "The Open Movie Database", "TheMovieDb")},
		{movie.AtName("disabled_metadata_fetchers"), types.ListUnknown(types.StringType)},
		{movie.AtName("metadata_fetcher_order"), types.ListUnknown(types.StringType)},
		{movie.AtName("image_fetchers"), testUnitStringList(t, "Screen Grabber")},
		{movie.AtName("disabled_image_fetchers"), types.ListUnknown(types.StringType)},
		{movie.AtName("image_fetcher_order"), types.ListUnknown(types.StringType)},
	} {
		if d := plan.SetAttribute(ctx, set.at, set.v); d.HasError() {
			t.Fatalf("planning %s: %v", set.at, d)
		}
	}
	resp := updateAgainst(t, r, c, plan)
	if resp.Diagnostics.HasError() {
		t.Fatalf("apply: %v", resp.Diagnostics)
	}

	mu.Lock()
	defer mu.Unlock()
	for key, want := range map[string]string{
		"MetadataFetcherOrder":     `["The Open Movie Database","TheMovieDb"]`,
		"DisabledMetadataFetchers": `[]`,
		"ImageFetcherOrder":        `["Screen Grabber","TheMovieDb"]`,
		"DisabledImageFetchers":    `["TheMovieDb"]`,
	} {
		checkSameJSON(t, jsonAt(t, posted, "MetadataOptions", 0, key), want)
	}
	if d := resp.State.GetAttribute(ctx, movie.AtName("image_fetchers"), &enabled); d.HasError() || !enabled.Equal(testUnitStringList(t, "Screen Grabber")) {
		t.Errorf("image_fetchers = %v after apply (%v), want the written list", enabled, d)
	}
	if asked["movies"] != 2 {
		t.Errorf("asked for the movies options %d times, want once for the read and once for the apply", asked["movies"])
	}
}
