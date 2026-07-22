// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"
)

func TestUnitSystemConfigurationOverlay(t *testing.T) {
	ctx := context.Background()
	fixture := `{
		"EnableMetrics": true,
		"EnableNormalizedItemByNameIDs": false,
		"IsPortAuthorized": true,
		"QuickConnectAvailable": false,
		"EnableCaseSensitiveItemIDs": false,
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
		"ChapterImageResolution": "Standard",
		"ParallelImageEncodingLimit": 0,
		"CastReceiverApplications": [
			{"Id": "ABCDEF", "Name": "Example"}
		],
		"TrickplayOptions": {
			"EnableHwAcceleration": false,
			"EnableHwEncoding": false,
			"EnableKeyFrameOnlyExtraction": false,
			"ScanBehavior": "Job",
			"ProcessPriorityClass": "BelowNormal",
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
