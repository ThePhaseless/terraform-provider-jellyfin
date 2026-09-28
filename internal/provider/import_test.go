// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"maps"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func TestUnitReadForImportRefusesResourcesWithoutABinding(t *testing.T) {
	_, _, err := ReadForImport(context.Background(), nil, "jellyfin_api_key", "id", "{}")
	if err == nil || !strings.Contains(err.Error(), "does not import a Jellyfin JSON document") {
		t.Errorf("ReadForImport(jellyfin_api_key) error = %v, want one saying it has no document to import", err)
	}
}

// A library's providers depend on its collection type, which ReadForImport
// has no way to ask the server about.
func TestUnitReadForImportRefusesResourcesThatAskForOfferedNames(t *testing.T) {
	_, _, err := ReadForImport(context.Background(), nil, "jellyfin_library", "Movies", `{"LibraryOptions": {"SubtitleFetcherOrder": [], "DisabledSubtitleFetchers": []}}`)
	if err == nil || !strings.Contains(err.Error(), "cannot ask for") {
		t.Errorf("ReadForImport(jellyfin_library) error = %v, want one saying it cannot ask for the offered providers", err)
	}
}

func TestUnitReadForImportRefusesUnknownResourceTypes(t *testing.T) {
	_, _, err := ReadForImport(context.Background(), nil, "jellyfin_no_such_resource", "id", "{}")
	if err == nil || !strings.Contains(err.Error(), "has no resource jellyfin_no_such_resource") {
		t.Errorf("ReadForImport(jellyfin_no_such_resource) error = %v, want one naming the unknown type", err)
	}
}

func TestUnitReadForImportStartsFromWhatImportStateSets(t *testing.T) {
	_, got, err := ReadForImport(context.Background(), nil, "jellyfin_scheduled_task", "task-id", `{"Id":"served-id"}`)
	if err != nil {
		t.Fatal(err)
	}
	if v := got.Attributes()["id"].String(); v != `"task-id"` {
		t.Errorf("id = %s, want the import ID", v)
	}
}

func TestUnitReadForImportReadsFetchersOnlyForItemTypesTheServerLists(t *testing.T) {
	var mu sync.Mutex
	srv := httptest.NewServer(testUnitServeAvailableOptions(map[string]string{
		"movies": `{"TypeOptions": [{"Type": "Movie", "MetadataFetchers": [{"Name": "TheMovieDb"}, {"Name": "The Open Movie Database"}], "ImageFetchers": []}]}`,
	}, map[string]int{}, &mu))
	t.Cleanup(srv.Close)

	_, got, err := ReadForImport(context.Background(), client.NewClient(srv.URL, "k"), "jellyfin_system_configuration", "system", `{"MetadataOptions": [
		{"ItemType": "Movie", "DisabledMetadataFetchers": ["The Open Movie Database"], "MetadataFetcherOrder": []},
		{"ItemType": "Person", "DisabledMetadataFetchers": ["TheMovieDb"], "MetadataFetcherOrder": []}]}`)
	if err != nil {
		t.Fatal(err)
	}
	entries, ok := got.Attributes()["metadata_options"].(types.List)
	if !ok || len(entries.Elements()) != 2 {
		t.Fatalf("metadata_options = %v, want both entries", got.Attributes()["metadata_options"])
	}
	for i, want := range []string{`["TheMovieDb"]`, `<null>`} {
		entry, ok := entries.Elements()[i].(types.Object)
		if !ok {
			t.Fatalf("metadata_options[%d] is a %T", i, entries.Elements()[i])
		}
		if v := entry.Attributes()["metadata_fetchers"].String(); v != want {
			t.Errorf("metadata_options[%d].metadata_fetchers = %s, want %s", i, v, want)
		}
	}
}

func TestUnitSharedKeysNameTheAttributeThatAlsoWritesEachKey(t *testing.T) {
	shared, err := SharedKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for resourceType, want := range map[string]map[string]string{
		"jellyfin_system_configuration": {
			"metadata_options.disabled_metadata_fetchers": "metadata_fetchers",
			"metadata_options.metadata_fetcher_order":     "metadata_fetchers",
			"metadata_options.disabled_image_fetchers":    "image_fetchers",
			"metadata_options.image_fetcher_order":        "image_fetchers",
		},
		"jellyfin_library": {
			"library_options.disabled_subtitle_fetchers":               "subtitle_fetchers",
			"library_options.subtitle_fetcher_order":                   "subtitle_fetchers",
			"library_options.type_options.metadata_fetcher_order":      "metadata_fetchers",
			"library_options.type_options.image_fetcher_order":         "image_fetchers",
			"library_options.type_options.similar_item_provider_order": "similar_item_providers",
		},
	} {
		if !maps.Equal(shared[resourceType], want) {
			t.Errorf("SharedKeys()[%s] = %v, want %v", resourceType, shared[resourceType], want)
		}
	}
	if len(shared) != 2 {
		t.Errorf("SharedKeys() covers %d resource types, want 2: %v", len(shared), shared)
	}
}
