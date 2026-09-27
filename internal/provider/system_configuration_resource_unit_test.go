// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
