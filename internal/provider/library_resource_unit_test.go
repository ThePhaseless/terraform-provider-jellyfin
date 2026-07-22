// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
				"ImageFetchers": ["TheMovieDb"],
				"ImageOptions": [{"Type": "Backdrop", "Limit": 3}],
				"ImageFetcherOrder": ["TheMovieDb"]
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
