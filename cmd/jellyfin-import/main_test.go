// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/provider"
)

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"Movies", "movies"},
		{"TV Shows", "tv_shows"},
		{"My Library!", "my_library"},
		{"  spaces  ", "spaces"},
		{"123abc", "r_123abc"},
		{"Hello World 123", "hello_world_123"},
		{"", "unnamed"},
		{"---", "unnamed"},
		{"a-b_c.d", "a_b_c_d"},
		{"UPPER CASE", "upper_case"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestImportBlock(t *testing.T) {
	result := importBlock("jellyfin_user", "admin", "abc-123")
	expected := `import {
  to = jellyfin_user.admin
  id = "abc-123"
}
`
	if result != expected {
		t.Errorf("importBlock() = %q, want %q", result, expected)
	}
}

func TestResourceBlock(t *testing.T) {
	attrs := map[string]string{
		"name":  `"test"`,
		"count": "5",
	}
	result := resourceBlock("jellyfin_user", "test", attrs)
	if !strings.Contains(result, `resource "jellyfin_user" "test"`) {
		t.Errorf("resourceBlock() missing resource header: %s", result)
	}
	if !strings.Contains(result, `name = "test"`) {
		t.Errorf("resourceBlock() missing name attr: %s", result)
	}
	if !strings.Contains(result, "count = 5") {
		t.Errorf("resourceBlock() missing count attr: %s", result)
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string]string{"c": "3", "a": "1", "b": "2"}
	keys := sortedKeys(m)
	expected := []string{"a", "b", "c"}
	for i, k := range keys {
		if k != expected[i] {
			t.Errorf("sortedKeys()[%d] = %q, want %q", i, k, expected[i])
		}
	}
}

// writeJSON encodes v as JSON into w, logging any error via t.
func writeJSON(t *testing.T, w http.ResponseWriter, v interface{}) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("failed to encode JSON response: %v", err)
	}
}

// setupTestServer creates a mock Jellyfin server for testing.
func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/Users", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{
				"Id":   "user-id-1",
				"Name": "admin",
				"Policy": map[string]interface{}{
					"IsAdministrator":  true,
					"IsDisabled":       false,
					"EnableAllFolders": true,
				},
			},
			{
				"Id":   "user-id-2",
				"Name": "viewer",
				"Policy": map[string]interface{}{
					"IsAdministrator":  false,
					"IsDisabled":       false,
					"EnableAllFolders": false,
				},
			},
		})
	})

	mux.HandleFunc("/Library/VirtualFolders", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{
				"Name":           "Movies",
				"CollectionType": "movies",
				"Locations":      []string{"/media/movies"},
				"ItemId":         "item-1",
			},
		})
	})

	mux.HandleFunc("/Auth/Keys", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"Items": []map[string]interface{}{
				{
					"AccessToken": "test-token-123",
					"AppName":     "MyApp",
				},
			},
		})
	})

	mux.HandleFunc("/Repositories", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{
				"Name":    "Jellyfin Stable",
				"Url":     "https://repo.jellyfin.org/files/plugin/manifest.json",
				"Enabled": true,
			},
		})
	})

	mux.HandleFunc("/Plugins", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{
				"Name":    "MusicBrainz",
				"Version": "14.0.0.0",
				"Id":      "plugin-id-1",
			},
		})
	})

	mux.HandleFunc("/Packages", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{
				"name": "MusicBrainz",
				"versions": []map[string]interface{}{
					{
						"version":        "14.0.0.0",
						"repositoryUrl":  "https://repo.jellyfin.org/files/plugin/manifest.json",
						"repositoryName": "Jellyfin Stable",
					},
				},
			},
		})
	})

	mux.HandleFunc("/ScheduledTasks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{
				"Name":     "Scan Media Library",
				"Id":       "task-id-1",
				"IsHidden": false,
				"Triggers": []map[string]interface{}{
					{
						"Type":          "IntervalTrigger",
						"IntervalTicks": 432000000000,
					},
				},
			},
			{
				"Name":     "Hidden Task",
				"Id":       "task-id-2",
				"IsHidden": true,
				"Triggers": []map[string]interface{}{},
			},
		})
	})

	mux.HandleFunc("/System/Configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"ServerName":                    "Test Server",
			"IsStartupWizardCompleted":      true,
			"EnableNormalizedItemByNameIds": true,
			"CachePath":                     nil,
			"SortRemoveWords":               []string{"the", "a"},
			"MetadataOptions": []map[string]interface{}{
				{"ItemType": "Movie", "DisabledMetadataFetchers": []string{"OMDb"}, "ImageFetcherOrder": nil},
			},
			"TrickplayOptions": map[string]interface{}{
				"Interval":         10000,
				"ProcessPriority":  "BelowNormal",
				"WidthResolutions": []int{320},
			},
		})
	})

	mux.HandleFunc("/System/Configuration/encoding", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"EncodingThreadCount": -1,
		})
	})

	mux.HandleFunc("/System/Configuration/network", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"BaseUrl":     "",
			"EnableHttps": false,
		})
	})

	mux.HandleFunc("/System/Configuration/branding", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"SplashscreenEnabled": false,
			"LoginDisclaimer":     "Hi ${user}\n100%{x}",
		})
	})

	mux.HandleFunc("/System/Configuration/livetv", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"EnableRecordingSubfolders": false,
		})
	})

	mux.HandleFunc("/System/Configuration/metadata", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"UseFileCreationTimeForDateAdded": true,
		})
	})

	return httptest.NewServer(mux)
}

func TestGenerateUsers(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]bool),
	}

	imports, resources, err := g.generateUsers()
	if err != nil {
		t.Fatalf("generateUsers() error: %v", err)
	}

	if len(imports) != 2 {
		t.Errorf("expected 2 import blocks, got %d", len(imports))
	}
	if len(resources) != 2 {
		t.Errorf("expected 2 resource blocks, got %d", len(resources))
	}

	// Check admin user import
	if !strings.Contains(imports[0], "jellyfin_user.admin") {
		t.Errorf("expected import to contain jellyfin_user.admin, got: %s", imports[0])
	}
	if !strings.Contains(imports[0], `"user-id-1"`) {
		t.Errorf("expected import ID user-id-1, got: %s", imports[0])
	}

	// Check admin user resource
	if !strings.Contains(resources[0], "is_administrator = true") {
		t.Errorf("expected admin to be administrator: %s", resources[0])
	}
}

func TestGenerateLibraries(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]bool),
	}

	imports, resources, err := g.generateLibraries()
	if err != nil {
		t.Fatalf("generateLibraries() error: %v", err)
	}

	if len(imports) != 1 {
		t.Errorf("expected 1 import block, got %d", len(imports))
	}
	if len(resources) != 1 {
		t.Errorf("expected 1 resource block, got %d", len(resources))
	}

	if !strings.Contains(resources[0], `collection_type = "movies"`) {
		t.Errorf("expected collection_type movies: %s", resources[0])
	}
	if !strings.Contains(resources[0], `"/media/movies"`) {
		t.Errorf("expected path /media/movies: %s", resources[0])
	}
}

func TestGenerateAPIKeys(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]bool),
	}

	imports, resources, err := g.generateAPIKeys()
	if err != nil {
		t.Fatalf("generateAPIKeys() error: %v", err)
	}

	if len(imports) != 1 {
		t.Errorf("expected 1 import block, got %d", len(imports))
	}
	if !strings.Contains(imports[0], `"test-token-123"`) {
		t.Errorf("expected import ID test-token-123: %s", imports[0])
	}
	if !strings.Contains(resources[0], `app_name = "MyApp"`) {
		t.Errorf("expected app_name MyApp: %s", resources[0])
	}
}

func TestGenerateScheduledTasks(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]bool),
	}

	imports, resources, err := g.generateScheduledTasks()
	if err != nil {
		t.Fatalf("generateScheduledTasks() error: %v", err)
	}

	// Hidden tasks should be skipped
	if len(imports) != 1 {
		t.Errorf("expected 1 import block (hidden tasks skipped), got %d", len(imports))
	}
	if len(resources) != 1 {
		t.Errorf("expected 1 resource block, got %d", len(resources))
	}

	if !strings.Contains(imports[0], "task-id-1") {
		t.Errorf("expected task-id-1 in import: %s", imports[0])
	}

	want := `resource "jellyfin_scheduled_task" "scan_media_library" {
  task_id = "task-id-1"
  triggers = [
    {
      type = "IntervalTrigger"
      interval_ticks = 432000000000
    },
  ]
}
`
	if resources[0] != want {
		t.Errorf("resource block = %q, want %q", resources[0], want)
	}
}

func TestTriggersHCL(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw  []string
		want string
	}{
		"no triggers": {want: "[]"},
		"interval trigger without day_of_week": {
			raw: []string{`{"Type":"IntervalTrigger","IntervalTicks":864000000000}`},
			want: `[
    {
      type = "IntervalTrigger"
      interval_ticks = 864000000000
    },
  ]`,
		},
		"daily trigger keeps max_runtime_ticks": {
			raw: []string{`{"Type":"DailyTrigger","TimeOfDayTicks":72000000000,"MaxRuntimeTicks":144000000000}`},
			want: `[
    {
      type = "DailyTrigger"
      time_of_day_ticks = 72000000000
      max_runtime_ticks = 144000000000
    },
  ]`,
		},
		"weekly trigger and startup trigger": {
			raw: []string{
				`{"Type":"WeeklyTrigger","TimeOfDayTicks":36000000000,"DayOfWeek":"Tuesday"}`,
				`{"Type":"StartupTrigger"}`,
			},
			want: `[
    {
      type = "WeeklyTrigger"
      time_of_day_ticks = 36000000000
      day_of_week = "Tuesday"
    },
    {
      type = "StartupTrigger"
    },
  ]`,
		},
		"explicit nulls and zero ticks": {
			raw: []string{`{"Type":"IntervalTrigger","IntervalTicks":0,"TimeOfDayTicks":null,"DayOfWeek":null,"MaxRuntimeTicks":0}`},
			want: `[
    {
      type = "IntervalTrigger"
      interval_ticks = 0
      max_runtime_ticks = 0
    },
  ]`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			raw := make([]json.RawMessage, len(tc.raw))
			for i, r := range tc.raw {
				raw[i] = json.RawMessage(r)
			}

			got, err := triggersHCL(raw)
			if err != nil {
				t.Fatalf("triggersHCL() error: %v", err)
			}
			if got != tc.want {
				t.Errorf("triggersHCL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGenerateSingletonConfigs(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]bool),
	}

	imports, resources, err := g.generateSingletonConfigs()
	if err != nil {
		t.Fatalf("generateSingletonConfigs() error: %v", err)
	}

	wantImports := []string{"system", "encoding", "networking", "branding", "livetv", "metadata"}
	if len(imports) != len(wantImports) {
		t.Fatalf("expected %d import blocks, got %d", len(wantImports), len(imports))
	}
	for i, id := range wantImports {
		want := importBlock("jellyfin_"+id+"_configuration", "this", id)
		if imports[i] != want {
			t.Errorf("imports[%d] = %q, want %q", i, imports[i], want)
		}
	}

	want := []string{
		`resource "jellyfin_system_configuration" "this" {
  metadata_options = [
    {
      disabled_metadata_fetchers = ["OMDb"]
      item_type = "Movie"
    },
  ]
  server_name = "Test Server"
  sort_remove_words = ["the", "a"]
  trickplay_options = {
    interval = 10000
    width_resolutions = [320]
  }
}
`,
		`resource "jellyfin_encoding_configuration" "this" {
  encoding_thread_count = -1
}
`,
		`resource "jellyfin_networking_configuration" "this" {
  base_url = ""
  enable_https = false
}
`,
		`resource "jellyfin_branding_configuration" "this" {
  login_disclaimer = "Hi $${user}\n100%%{x}"
  splashscreen_enabled = false
}
`,
		`resource "jellyfin_livetv_configuration" "this" {
  enable_recording_subfolders = false
}
`,
		`resource "jellyfin_metadata_configuration" "this" {
  use_file_creation_time_for_date_added = true
}
`,
	}
	if len(resources) != len(want) {
		t.Fatalf("expected %d resource blocks, got %d", len(want), len(resources))
	}
	for i := range want {
		if resources[i] != want[i] {
			t.Errorf("resources[%d] =\n%s\nwant\n%s", i, resources[i], want[i])
		}
	}
}

func TestGeneratePlugins(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]bool),
	}

	imports, resources, err := g.generatePlugins()
	if err != nil {
		t.Fatalf("generatePlugins() error: %v", err)
	}

	if len(imports) != 1 {
		t.Errorf("expected 1 import block, got %d", len(imports))
	}
	if !strings.Contains(imports[0], "plugin-id-1") {
		t.Errorf("expected plugin-id-1 in import: %s", imports[0])
	}
	if !strings.Contains(resources[0], `name = "MusicBrainz"`) {
		t.Errorf("expected plugin name MusicBrainz: %s", resources[0])
	}
	if !strings.Contains(resources[0], `repository_url = "https://repo.jellyfin.org/files/plugin/manifest.json"`) {
		t.Errorf("expected repository_url resolved from packages: %s", resources[0])
	}
}

func TestGeneratePluginRepositories(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]bool),
	}

	imports, resources, err := g.generatePluginRepositories()
	if err != nil {
		t.Fatalf("generatePluginRepositories() error: %v", err)
	}

	if len(imports) != 1 {
		t.Errorf("expected 1 import block, got %d", len(imports))
	}
	if !strings.Contains(resources[0], `url = "https://repo.jellyfin.org/files/plugin/manifest.json"`) {
		t.Errorf("expected repo URL in resource: %s", resources[0])
	}
}

func TestFullGenerate(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	outputDir := t.TempDir()
	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: outputDir,
		usedNames: make(map[string]bool),
	}

	if err := g.Generate(); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Check that files were created
	importsPath := filepath.Join(outputDir, "imports.tf")
	if _, err := os.Stat(importsPath); os.IsNotExist(err) {
		t.Error("imports.tf was not created")
	}

	resourcesPath := filepath.Join(outputDir, "resources.tf")
	if _, err := os.Stat(resourcesPath); os.IsNotExist(err) {
		t.Error("resources.tf was not created")
	}

	// Verify imports.tf content
	importsContent, err := os.ReadFile(importsPath)
	if err != nil {
		t.Fatalf("Failed to read imports.tf: %v", err)
	}

	expectedImports := []string{
		"jellyfin_user.admin",
		"jellyfin_user.viewer",
		"jellyfin_library.movies",
		"jellyfin_api_key.myapp",
		"jellyfin_plugin_repository.jellyfin_stable",
		"jellyfin_plugin.musicbrainz",
		"jellyfin_scheduled_task.scan_media_library",
		"jellyfin_system_configuration.this",
		"jellyfin_encoding_configuration.this",
		"jellyfin_networking_configuration.this",
		"jellyfin_branding_configuration.this",
		"jellyfin_livetv_configuration.this",
		"jellyfin_metadata_configuration.this",
	}

	for _, expected := range expectedImports {
		if !strings.Contains(string(importsContent), expected) {
			t.Errorf("imports.tf missing %s", expected)
		}
	}

	// Verify resources.tf content
	resourcesContent, err := os.ReadFile(resourcesPath)
	if err != nil {
		t.Fatalf("Failed to read resources.tf: %v", err)
	}

	expectedResources := []string{
		`resource "jellyfin_user" "admin"`,
		`resource "jellyfin_user" "viewer"`,
		`resource "jellyfin_library" "movies"`,
		`resource "jellyfin_api_key" "myapp"`,
		`resource "jellyfin_plugin_repository" "jellyfin_stable"`,
		`resource "jellyfin_plugin" "musicbrainz"`,
		`resource "jellyfin_scheduled_task" "scan_media_library"`,
		`resource "jellyfin_system_configuration" "this"`,
		`resource "jellyfin_encoding_configuration" "this"`,
		`resource "jellyfin_networking_configuration" "this"`,
		`resource "jellyfin_branding_configuration" "this"`,
		`resource "jellyfin_livetv_configuration" "this"`,
		`resource "jellyfin_metadata_configuration" "this"`,
	}

	for _, expected := range expectedResources {
		if !strings.Contains(string(resourcesContent), expected) {
			t.Errorf("resources.tf missing %s", expected)
		}
	}
}

func TestGenerateWithServerError(t *testing.T) {
	// Server that returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal Server Error")
	}))
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]bool),
	}

	err := g.Generate()
	if err == nil {
		t.Error("expected error from Generate(), got nil")
	}
}

func TestSanitizeNameEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"a", "a"},
		{"1", "r_1"},
		{"_leading", "leading"},
		{"trailing_", "trailing"},
		{"multi___underscores", "multi_underscores"},
		{"café", "caf"},
		{"hello.world", "hello_world"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestUniqueName(t *testing.T) {
	g := &generator{usedNames: make(map[string]bool)}

	// First use: no suffix
	name1 := g.uniqueName("jellyfin_user", "admin")
	if name1 != "admin" {
		t.Errorf("first uniqueName() = %q, want %q", name1, "admin")
	}

	// Second use of same type+name: gets suffix _1
	name2 := g.uniqueName("jellyfin_user", "admin")
	if name2 != "admin_1" {
		t.Errorf("second uniqueName() = %q, want %q", name2, "admin_1")
	}

	// Third use: suffix _2
	name3 := g.uniqueName("jellyfin_user", "admin")
	if name3 != "admin_2" {
		t.Errorf("third uniqueName() = %q, want %q", name3, "admin_2")
	}

	// Different resource type: no suffix
	name4 := g.uniqueName("jellyfin_library", "admin")
	if name4 != "admin" {
		t.Errorf("different type uniqueName() = %q, want %q", name4, "admin")
	}
}

func TestUniqueNameSkipsSuffixedNamesAlreadyTaken(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		bases []string
		want  []string
	}{
		"suffix taken by an earlier base name": {
			bases: []string{"films", "films_1", "films"},
			want:  []string{"films", "films_1", "films_2"},
		},
		"base name taken by an earlier suffix": {
			bases: []string{"films", "films", "films_1"},
			want:  []string{"films", "films_1", "films_1_1"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g := &generator{usedNames: make(map[string]bool)}
			for i, base := range tc.bases {
				if got := g.uniqueName("jellyfin_library", base); got != tc.want[i] {
					t.Errorf("uniqueName(%q) call %d = %q, want %q", base, i+1, got, tc.want[i])
				}
			}
		})
	}
}

func TestGenerateLibrariesGivesEachLibraryItsOwnAddress(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/Library/VirtualFolders", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{"Name": "Films!", "CollectionType": "movies", "Locations": []string{"/media/movies"}},
			{"Name": "Films 1", "CollectionType": "movies", "Locations": []string{"/media/movies"}},
			{"Name": "Films", "CollectionType": "movies", "Locations": []string{"/media/movies"}},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		usedNames: make(map[string]bool),
	}
	imports, _, err := g.generateLibraries()
	if err != nil {
		t.Fatalf("generateLibraries() error: %v", err)
	}

	want := []string{"jellyfin_library.films\n", "jellyfin_library.films_1\n", "jellyfin_library.films_2\n"}
	if len(imports) != len(want) {
		t.Fatalf("expected %d import blocks, got %d", len(want), len(imports))
	}
	for i, to := range want {
		if !strings.Contains(imports[i], "to = "+to) {
			t.Errorf("imports[%d] = %q, want it to import to %s", i, imports[i], strings.TrimSpace(to))
		}
	}
}

func TestGeneratePluginsWithoutPackagesEndpoint(t *testing.T) {
	// Server that has /Plugins but returns 500 for /Packages
	mux := http.NewServeMux()
	mux.HandleFunc("/Plugins", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{
				"Name":    "TestPlugin",
				"Version": "1.0.0",
				"Id":      "test-plugin-id",
			},
		})
	})
	mux.HandleFunc("/Packages", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "error")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]bool),
	}

	imports, resources, err := g.generatePlugins()
	if err != nil {
		t.Fatalf("generatePlugins() should not fail when /Packages is unavailable: %v", err)
	}

	if len(imports) != 1 {
		t.Errorf("expected 1 import block, got %d", len(imports))
	}

	if strings.Contains(resources[0], "repository_url") {
		t.Errorf("expected no repository_url when packages are unavailable: %s", resources[0])
	}
}

func TestGeneratePluginsResolvesOnlyTheInstalledVersion(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/Plugins", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{"Name": "Bundled", "Version": "12.1.0.0", "Id": "bundled-id"},
			{"Name": "Listed", "Version": "2.0.0.0", "Id": "listed-id"},
		})
	})
	mux.HandleFunc("/Packages", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{"name": "Bundled", "versions": []map[string]interface{}{
				{"version": "11.0.0.0", "repositoryUrl": "https://repo.example/bundled.json"},
			}},
			{"name": "Listed", "versions": []map[string]interface{}{
				{"version": "3.0.0.0", "repositoryUrl": "https://repo.example/newer.json"},
				{"version": "2.0.0.0", "repositoryUrl": "https://repo.example/listed.json"},
			}},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]bool),
	}

	_, resources, err := g.generatePlugins()
	if err != nil {
		t.Fatalf("generatePlugins() error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resource blocks, got %d", len(resources))
	}

	if strings.Contains(resources[0], "repository_url") {
		t.Errorf("expected no repository_url for a version the repository does not list: %s", resources[0])
	}
	if !strings.Contains(resources[1], `repository_url = "https://repo.example/listed.json"`) {
		t.Errorf("expected the installed version's repository_url: %s", resources[1])
	}
}

func TestGenerateLibrariesSkipsCollectionTypesTheProviderRejects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/Library/VirtualFolders", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{"Name": "Untyped", "Locations": []string{"/media/untyped"}},
			{"Name": "Clips", "CollectionType": "musicvideos", "Locations": []string{"/media/clips"}},
			{"Name": "Films ${x}", "CollectionType": "movies", "Locations": []string{"/media/films"}},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	var warnings strings.Builder
	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]bool),
		warnings:  &warnings,
	}

	imports, resources, err := g.generateLibraries()
	if err != nil {
		t.Fatalf("generateLibraries() error: %v", err)
	}
	if len(imports) != 1 || len(resources) != 1 {
		t.Fatalf("expected 1 import and 1 resource block, got %d and %d", len(imports), len(resources))
	}

	wantImport := `import {
  to = jellyfin_library.films_x
  id = "Films $${x}"
}
`
	if imports[0] != wantImport {
		t.Errorf("import block = %q, want %q", imports[0], wantImport)
	}
	for _, skipped := range []string{`"Untyped"`, `"Clips"`} {
		if !strings.Contains(warnings.String(), skipped) {
			t.Errorf("expected a warning naming the skipped library %s, got %q", skipped, warnings.String())
		}
	}
}

func TestImportClientUsesAPIKeyWhenProvided(t *testing.T) {
	t.Parallel()

	c, err := importClient(context.Background(), "http://example.test", "api-key", "", "")
	if err != nil {
		t.Fatalf("importClient() error = %v", err)
	}
	if c.APIKey != "api-key" {
		t.Fatalf("APIKey = %q, want api-key", c.APIKey)
	}
}

func TestImportClientAuthenticatesWithCredentials(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Users/AuthenticateByName" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding auth body: %v", err)
		}
		if body["Username"] != "admin" || body["Pw"] != "Admin123!" {
			t.Fatalf("unexpected auth body: %#v", body)
		}
		writeJSON(t, w, map[string]string{"AccessToken": "login-token"})
	}))
	defer server.Close()

	c, err := importClient(context.Background(), server.URL, "", "admin", "Admin123!")
	if err != nil {
		t.Fatalf("importClient() error = %v", err)
	}
	if c.APIKey != "login-token" {
		t.Fatalf("APIKey = %q, want login-token", c.APIKey)
	}
}

func TestImportClientRequiresUsernameWhenAPIKeyMissing(t *testing.T) {
	t.Parallel()

	_, err := importClient(context.Background(), "http://example.test", "", "", "Admin123!")
	if err == nil {
		t.Fatal("importClient() error = nil, want missing username error")
	}
	if !strings.Contains(err.Error(), "missing Jellyfin username") {
		t.Fatalf("importClient() error = %v, want missing username error", err)
	}
}

func TestImportClientRequiresPasswordWhenAPIKeyMissing(t *testing.T) {
	t.Parallel()

	_, err := importClient(context.Background(), "http://example.test", "", "admin", "")
	if err == nil {
		t.Fatal("importClient() error = nil, want missing password error")
	}
	if !strings.Contains(err.Error(), "missing Jellyfin password") {
		t.Fatalf("importClient() error = %v, want missing password error", err)
	}
}

func testAccImportClient(t *testing.T) *client.Client {
	t.Helper()

	endpoint := os.Getenv("JELLYFIN_ENDPOINT")
	apiKey := os.Getenv("JELLYFIN_API_KEY")
	username := os.Getenv("JELLYFIN_USERNAME")
	password := os.Getenv("JELLYFIN_PASSWORD")
	if endpoint == "" || (apiKey == "" && (username == "" || password == "")) {
		skip := t.Skip
		if os.Getenv("TF_ACC") != "" {
			skip = t.Fatal
		}
		skip("JELLYFIN_ENDPOINT and either JELLYFIN_API_KEY or JELLYFIN_USERNAME/JELLYFIN_PASSWORD must be set for acceptance tests")
	}

	c, err := importClient(context.Background(), endpoint, apiKey, username, password)
	if err != nil {
		t.Fatalf("failed to configure Jellyfin import acceptance test client: %v", err)
	}
	return c
}

// seedEscapingFixtures gives the server strings that HCL would interpolate
// or reject unless the importer escapes them, and puts the server back when
// the test ends.
func seedEscapingFixtures(t *testing.T, c *client.Client) {
	t.Helper()

	ctx := context.Background()
	const tricky = `import-e2e ${a} %{b} "q" \ $${c} %%{d} ${`

	var restore []func(*client.Client) error
	t.Cleanup(func() {
		// The provider logs in under the same device ID during the plan, and
		// Jellyfin then revokes the session token c holds.
		fresh := testAccImportClient(t)
		for i := len(restore) - 1; i >= 0; i-- {
			if err := restore[i](fresh); err != nil {
				t.Error(err)
			}
		}
	})

	branding, err := c.GetBrandingConfiguration(ctx)
	if err != nil {
		t.Fatalf("reading branding configuration: %v", err)
	}
	restore = append(restore, func(c *client.Client) error {
		return c.UpdateBrandingConfiguration(ctx, &client.BrandingConfiguration{RawJSON: branding.RawJSON})
	})
	seeded, err := json.Marshal(map[string]interface{}{
		"LoginDisclaimer":     tricky + "\nsecond line\twith a tab and \x01",
		"CustomCss":           "body {\n  color: red;\n}\n/* ${x} %{y} */\n",
		"SplashscreenEnabled": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateBrandingConfiguration(ctx, &client.BrandingConfiguration{RawJSON: string(seeded)}); err != nil {
		t.Fatalf("seeding branding configuration: %v", err)
	}

	if err := c.CreateAPIKey(ctx, tricky); err != nil {
		t.Fatalf("seeding API key: %v", err)
	}
	key, err := c.GetAPIKeyByAppName(ctx, tricky)
	if err != nil {
		t.Fatalf("finding seeded API key: %v", err)
	}
	restore = append(restore, func(c *client.Client) error {
		return c.DeleteAPIKey(ctx, key.AccessToken)
	})

	repos, err := c.GetPluginRepositories(ctx)
	if err != nil {
		t.Fatalf("reading plugin repositories: %v", err)
	}
	restore = append(restore, func(c *client.Client) error {
		return c.SetPluginRepositories(ctx, repos)
	})
	// Disabled, so Jellyfin never fetches the unreachable URL.
	seededRepos := append(append([]client.PluginRepository{}, repos...), client.PluginRepository{
		Name:    tricky,
		URL:     "http://127.0.0.1:1/${x}/%{y}/manifest.json",
		Enabled: false,
	})
	if err := c.SetPluginRepositories(ctx, seededRepos); err != nil {
		t.Fatalf("seeding plugin repository: %v", err)
	}

	// Library names cannot hold quotes or backslashes.
	const library = "import-e2e ${lib} %{x} $${y}"
	if err := c.AddVirtualFolder(ctx, library, "movies", []string{"/media/movies"}, &client.LibraryOptions{RawJSON: "{}"}); err != nil {
		t.Fatalf("seeding library: %v", err)
	}
	restore = append(restore, func(c *client.Client) error {
		return c.RemoveVirtualFolder(ctx, library)
	})
}

// TestAccImportToolE2E runs the import tool against a real Jellyfin instance
// and plans the generated files with Terraform and this provider: the plan
// must import every resource without changes, and each imported resource's
// configuration must set every value its imported state holds.
// Set JELLYFIN_ENDPOINT and either JELLYFIN_API_KEY or JELLYFIN_USERNAME/JELLYFIN_PASSWORD to enable this test.
func TestAccImportToolE2E(t *testing.T) {
	outputDir := t.TempDir()
	c := testAccImportClient(t)
	seedEscapingFixtures(t, c)

	var warnings strings.Builder
	g := &generator{
		client:    c,
		outputDir: outputDir,
		usedNames: make(map[string]bool),
		warnings:  &warnings,
	}

	// Run the full generation.
	if err := g.Generate(); err != nil {
		t.Fatalf("Generate() against live Jellyfin failed: %v", err)
	}

	// Verify imports.tf was created and has content.
	importsPath := filepath.Join(outputDir, "imports.tf")
	importsContent, err := os.ReadFile(importsPath)
	if err != nil {
		t.Fatalf("Failed to read imports.tf: %v", err)
	}
	if len(importsContent) == 0 {
		t.Fatal("imports.tf is empty")
	}

	// Verify resources.tf was created and has content.
	resourcesPath := filepath.Join(outputDir, "resources.tf")
	resourcesContent, err := os.ReadFile(resourcesPath)
	if err != nil {
		t.Fatalf("Failed to read resources.tf: %v", err)
	}
	if len(resourcesContent) == 0 {
		t.Fatal("resources.tf is empty")
	}

	importsStr := string(importsContent)
	resourcesStr := string(resourcesContent)

	// A real Jellyfin instance always has at least one user (admin).
	if !strings.Contains(importsStr, "jellyfin_user.") {
		t.Error("imports.tf should contain at least one jellyfin_user import block")
	}
	if !strings.Contains(resourcesStr, `resource "jellyfin_user"`) {
		t.Error("resources.tf should contain at least one jellyfin_user resource block")
	}

	// Singleton configs should always be present.
	singletonTypes := []string{
		"jellyfin_system_configuration",
		"jellyfin_encoding_configuration",
		"jellyfin_networking_configuration",
		"jellyfin_branding_configuration",
		"jellyfin_livetv_configuration",
		"jellyfin_metadata_configuration",
	}
	for _, rt := range singletonTypes {
		if !strings.Contains(importsStr, rt+".this") {
			t.Errorf("imports.tf should contain %s.this import block", rt)
		}
		if !strings.Contains(resourcesStr, fmt.Sprintf(`resource "%s" "this"`, rt)) {
			t.Errorf("resources.tf should contain %s resource block", rt)
		}
	}

	// All import blocks should have 'to' and 'id' fields.
	importBlocks := strings.Count(importsStr, "import {")
	toFields := strings.Count(importsStr, "to = ")
	idFields := strings.Count(importsStr, "id = ")
	if importBlocks != toFields || importBlocks != idFields {
		t.Errorf("import block count mismatch: blocks=%d, to=%d, id=%d", importBlocks, toFields, idFields)
	}

	// All resource blocks should have opening and closing braces.
	resourceBlocks := strings.Count(resourcesStr, "resource \"")
	if resourceBlocks == 0 {
		t.Error("resources.tf should contain at least one resource block")
	}

	t.Logf("Generated %d import blocks and %d resource blocks", importBlocks, resourceBlocks)
	if warnings.Len() > 0 {
		t.Logf("Generator warnings:\n%s", warnings.String())
	}

	// PlanOnly never applies, so no state is saved and nothing on the server
	// is changed or destroyed.
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"jellyfin": providerserver.NewProtocol6WithError(provider.New("test")()),
		},
		Steps: []resource.TestStep{
			{
				Config:   importsStr + "\n" + resourcesStr,
				PlanOnly: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh: []plancheck.PlanCheck{
						configMatchesImportedState{schemas: providerSchemas(t)},
					},
				},
			},
		},
	})
}

// TestAccImportToolIndividualGenerators tests each generator function against a real
// Jellyfin instance to verify they produce valid output.
func TestAccImportToolIndividualGenerators(t *testing.T) {
	c := testAccImportClient(t)

	t.Run("Users", func(t *testing.T) {
		g := &generator{
			client:    c,
			outputDir: t.TempDir(),
			usedNames: make(map[string]bool),
		}

		imports, resources, err := g.generateUsers()
		if err != nil {
			t.Fatalf("generateUsers() error: %v", err)
		}

		// A real Jellyfin always has at least the admin user.
		if len(imports) == 0 {
			t.Error("expected at least 1 user import block")
		}
		if len(resources) == 0 {
			t.Error("expected at least 1 user resource block")
		}

		// Verify structure of first user.
		if len(imports) > 0 && !strings.Contains(imports[0], "jellyfin_user.") {
			t.Errorf("import block should reference jellyfin_user: %s", imports[0])
		}
		if len(resources) > 0 && !strings.Contains(resources[0], "is_administrator") {
			t.Errorf("resource block should contain is_administrator: %s", resources[0])
		}
	})

	t.Run("ScheduledTasks", func(t *testing.T) {
		g := &generator{
			client:    c,
			outputDir: t.TempDir(),
			usedNames: make(map[string]bool),
		}

		imports, resources, err := g.generateScheduledTasks()
		if err != nil {
			t.Fatalf("generateScheduledTasks() error: %v", err)
		}

		// Jellyfin always has scheduled tasks.
		if len(imports) == 0 {
			t.Error("expected at least 1 scheduled task import block")
		}
		if len(resources) == 0 {
			t.Error("expected at least 1 scheduled task resource block")
		}
	})

	t.Run("SingletonConfigs", func(t *testing.T) {
		g := &generator{
			client:    c,
			outputDir: t.TempDir(),
			usedNames: make(map[string]bool),
		}

		imports, resources, err := g.generateSingletonConfigs()
		if err != nil {
			t.Fatalf("generateSingletonConfigs() error: %v", err)
		}

		if len(imports) != 6 {
			t.Errorf("expected 6 singleton config imports, got %d", len(imports))
		}
		if len(resources) != 6 {
			t.Errorf("expected 6 singleton config resources, got %d", len(resources))
		}

		// System config should have server_name.
		if len(resources) > 0 && !strings.Contains(resources[0], "server_name") {
			t.Errorf("system config should contain server_name: %s", resources[0])
		}
	})

	t.Run("APIKeys", func(t *testing.T) {
		g := &generator{
			client:    c,
			outputDir: t.TempDir(),
			usedNames: make(map[string]bool),
		}

		imports, resources, err := g.generateAPIKeys()
		if err != nil {
			t.Fatalf("generateAPIKeys() error: %v", err)
		}

		if len(imports) != len(resources) {
			t.Errorf("API key import/resource count mismatch: imports=%d resources=%d", len(imports), len(resources))
		}
		if len(resources) > 0 && !strings.Contains(resources[0], "app_name") {
			t.Errorf("API key resource should contain app_name: %s", resources[0])
		}
	})

	t.Run("Libraries", func(t *testing.T) {
		g := &generator{
			client:    c,
			outputDir: t.TempDir(),
			usedNames: make(map[string]bool),
		}

		// Libraries may or may not exist on a fresh instance - just verify no error.
		_, _, err := g.generateLibraries()
		if err != nil {
			t.Fatalf("generateLibraries() error: %v", err)
		}
	})

	t.Run("PluginRepositories", func(t *testing.T) {
		g := &generator{
			client:    c,
			outputDir: t.TempDir(),
			usedNames: make(map[string]bool),
		}

		// Plugin repos may or may not exist - just verify no error.
		_, _, err := g.generatePluginRepositories()
		if err != nil {
			t.Fatalf("generatePluginRepositories() error: %v", err)
		}
	})

	t.Run("Plugins", func(t *testing.T) {
		g := &generator{
			client:    c,
			outputDir: t.TempDir(),
			usedNames: make(map[string]bool),
		}

		// Plugins may or may not exist - just verify no error.
		_, _, err := g.generatePlugins()
		if err != nil {
			t.Fatalf("generatePlugins() error: %v", err)
		}
	})
}
