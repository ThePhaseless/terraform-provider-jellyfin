// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
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
)

func TestUnitSystemConfigurationOverlay(t *testing.T) {
	ctx := context.Background()
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

	var data SystemConfigurationResourceModel
	flattenSystemConfiguration(ctx, fixture, &data, nil)

	base := map[string]json.RawMessage{}
	if d := overlaySystemConfiguration(ctx, base, &data); d.HasError() {
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
	state := tfsdk.State{Raw: tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{})}
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
				req := planmodifier.ListRequest{State: state, StateValue: prior, ConfigValue: types.ListNull(a.ElementType), PlanValue: types.ListUnknown(a.ElementType)}
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
