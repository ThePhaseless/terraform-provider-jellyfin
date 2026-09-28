// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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
	want := `resource "jellyfin_user" "test" {
  count = 5
  name  = "test"
}
`
	if result != want {
		t.Errorf("resourceBlock() = %q, want %q", result, want)
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
				"Key":      "RefreshLibrary",
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
				"Key":      "RefreshGuide",
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
				{"ItemType": "Person", "DisabledMetadataFetchers": []string{"TheMovieDb"}, "MetadataFetcherOrder": []string{}},
			},
			"TrickplayOptions": map[string]interface{}{
				"Interval":         10000,
				"ProcessPriority":  "BelowNormal",
				"WidthResolutions": []int{320},
			},
		})
	})

	mux.HandleFunc("/Libraries/AvailableOptions", func(w http.ResponseWriter, r *http.Request) {
		typeOptions := []map[string]interface{}{}
		if r.URL.Query().Get("libraryContentType") == "movies" {
			typeOptions = append(typeOptions, map[string]interface{}{
				"Type":             "Movie",
				"MetadataFetchers": []map[string]interface{}{{"Name": "TheMovieDb"}, {"Name": "OMDb"}},
				"ImageFetchers":    []map[string]interface{}{{"Name": "TheMovieDb"}},
			})
		}
		writeJSON(t, w, map[string]interface{}{"TypeOptions": typeOptions})
	})

	mux.HandleFunc("/System/Configuration/encoding", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"EncodingThreadCount": -1,
			"DownMixAudioBoost":   2.5,
			"TonemappingPeak":     100,
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
	if !strings.Contains(resources[0], "is_administrator   = true") {
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

func TestGenerateLibrariesWritesMixedForMissingCollectionType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{
				"Name":      "Mixed",
				"Locations": []string{"/media/mixed"},
				"ItemId":    "item-1",
			},
		})
	}))
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]bool),
	}

	_, resources, err := g.generateLibraries()
	if err != nil {
		t.Fatalf("generateLibraries() error: %v", err)
	}
	if len(resources) != 1 || !strings.Contains(resources[0], `collection_type = "mixed"`) {
		t.Errorf("expected one library with collection_type mixed: %q", resources)
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

	if !strings.Contains(imports[0], `id = "RefreshLibrary"`) {
		t.Errorf("expected the key RefreshLibrary in import: %s", imports[0])
	}

	want := `resource "jellyfin_scheduled_task" "scan_media_library" {
  key = "RefreshLibrary"
  triggers = [
    {
      interval_ticks = 432000000000
      type           = "IntervalTrigger"
    },
  ]
}
`
	if resources[0] != want {
		t.Errorf("resource block = %q, want %q", resources[0], want)
	}
}

func TestGenerateScheduledTasksRendersTriggers(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		triggers string
		want     string
	}{
		"no triggers": {
			triggers: `[]`,
			want: `resource "jellyfin_scheduled_task" "task" {
  key      = "Task"
  triggers = []
}
`,
		},
		"null triggers": {
			triggers: `null`,
			want: `resource "jellyfin_scheduled_task" "task" {
  key      = "Task"
  triggers = []
}
`,
		},
		"interval trigger without day_of_week": {
			triggers: `[{"Type":"IntervalTrigger","IntervalTicks":864000000000}]`,
			want: `resource "jellyfin_scheduled_task" "task" {
  key = "Task"
  triggers = [
    {
      interval_ticks = 864000000000
      type           = "IntervalTrigger"
    },
  ]
}
`,
		},
		"daily trigger keeps max_runtime_ticks": {
			triggers: `[{"Type":"DailyTrigger","TimeOfDayTicks":72000000000,"MaxRuntimeTicks":144000000000}]`,
			want: `resource "jellyfin_scheduled_task" "task" {
  key = "Task"
  triggers = [
    {
      max_runtime_ticks = 144000000000
      time_of_day_ticks = 72000000000
      type              = "DailyTrigger"
    },
  ]
}
`,
		},
		"weekly trigger and startup trigger": {
			triggers: `[
				{"Type":"WeeklyTrigger","TimeOfDayTicks":36000000000,"DayOfWeek":"Tuesday"},
				{"Type":"StartupTrigger"}
			]`,
			want: `resource "jellyfin_scheduled_task" "task" {
  key = "Task"
  triggers = [
    {
      day_of_week       = "Tuesday"
      time_of_day_ticks = 36000000000
      type              = "WeeklyTrigger"
    },
    {
      type = "StartupTrigger"
    },
  ]
}
`,
		},
		"explicit nulls and zero ticks": {
			triggers: `[{"Type":"IntervalTrigger","IntervalTicks":0,"TimeOfDayTicks":null,"DayOfWeek":null,"MaxRuntimeTicks":0}]`,
			want: `resource "jellyfin_scheduled_task" "task" {
  key = "Task"
  triggers = [
    {
      interval_ticks    = 0
      max_runtime_ticks = 0
      type              = "IntervalTrigger"
    },
  ]
}
`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintf(w, `[{"Name": "Task", "Id": "task-id", "Key": "Task", "Triggers": %s}]`, tc.triggers)
			}))
			defer server.Close()

			g := &generator{
				client:    client.NewClient(server.URL, "test-key"),
				usedNames: make(map[string]bool),
			}
			_, resources, err := g.generateScheduledTasks()
			if err != nil {
				t.Fatalf("generateScheduledTasks() error: %v", err)
			}
			if len(resources) != 1 {
				t.Fatalf("expected 1 resource block, got %d", len(resources))
			}
			if resources[0] != tc.want {
				t.Errorf("resource block =\n%s\nwant\n%s", resources[0], tc.want)
			}
		})
	}
}

func TestGenerateScheduledTasksWritesTaskIDWhenNoKeySelectsTheTask(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[
			{"Name": "No Key", "Id": "id-no-key", "Key": "", "Triggers": []},
			{"Name": "Shared A", "Id": "id-shared-a", "Key": "Shared", "Triggers": []},
			{"Name": "Shared B", "Id": "id-shared-b", "Key": "Shared", "Triggers": []},
			{"Name": "Own Key", "Id": "id-own-key", "Key": "Own", "Triggers": []}
		]`)
	}))
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		usedNames: make(map[string]bool),
	}
	imports, resources, err := g.generateScheduledTasks()
	if err != nil {
		t.Fatalf("generateScheduledTasks() error: %v", err)
	}

	want := []struct{ importID, resource string }{
		{"id-no-key", `resource "jellyfin_scheduled_task" "no_key" {
  task_id  = "id-no-key"
  triggers = []
}
`},
		{"id-shared-a", `resource "jellyfin_scheduled_task" "shared_a" {
  task_id  = "id-shared-a"
  triggers = []
}
`},
		{"id-shared-b", `resource "jellyfin_scheduled_task" "shared_b" {
  task_id  = "id-shared-b"
  triggers = []
}
`},
		{"Own", `resource "jellyfin_scheduled_task" "own_key" {
  key      = "Own"
  triggers = []
}
`},
	}
	if len(imports) != len(want) || len(resources) != len(want) {
		t.Fatalf("got %d import and %d resource blocks, want %d of each", len(imports), len(resources), len(want))
	}
	for i, w := range want {
		if !strings.Contains(imports[i], fmt.Sprintf("id = %q", w.importID)) {
			t.Errorf("import block %d = %s, want the id %q", i, imports[i], w.importID)
		}
		if resources[i] != w.resource {
			t.Errorf("resource block %d =\n%s\nwant\n%s", i, resources[i], w.resource)
		}
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
  enable_normalized_item_by_name_ids = true
  metadata_options = [
    {
      item_type         = "Movie"
      metadata_fetchers = ["TheMovieDb"]
    },
    {
      disabled_metadata_fetchers = ["TheMovieDb"]
      item_type                  = "Person"
      metadata_fetcher_order     = []
    },
  ]
  server_name       = "Test Server"
  sort_remove_words = ["the", "a"]
  trickplay_options = {
    interval          = 10000
    process_priority  = "BelowNormal"
    width_resolutions = [320]
  }
}
`,
		`resource "jellyfin_encoding_configuration" "this" {
  down_mix_audio_boost  = 2.5
  encoding_thread_count = -1
  tonemapping_peak      = 100
}
`,
		`resource "jellyfin_networking_configuration" "this" {
  base_url     = ""
  enable_https = false
}
`,
		`resource "jellyfin_branding_configuration" "this" {
  login_disclaimer     = "Hi $${user}\n100%%{x}"
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
	if !strings.Contains(resources[0], `name           = "MusicBrainz"`) {
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
	if !strings.Contains(resources[0], `url     = "https://repo.jellyfin.org/files/plugin/manifest.json"`) {
		t.Errorf("expected repo URL in resource: %s", resources[0])
	}
}

func TestFullGenerate(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	outputDir := t.TempDir()
	var warnings strings.Builder
	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: outputDir,
		usedNames: make(map[string]bool),
		warnings:  &warnings,
	}

	if err := g.Generate(); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	// The server has an API key, whose token is its import ID.
	if got := warnings.String(); strings.Count(got, "Warning:") != 1 || !strings.Contains(got, "access token of each API key") {
		t.Errorf("expected only the API key token warning for an empty output directory, got %q", got)
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

	wantRequiredProviders := `terraform {
  required_providers {
    jellyfin = {
      source = "ThePhaseless/jellyfin"
    }
  }
}
`
	if !strings.HasPrefix(string(resourcesContent), wantRequiredProviders) {
		t.Errorf("resources.tf should start with\n%s\ngot\n%s", wantRequiredProviders, resourcesContent)
	}
}

func TestGenerateWarnsAboutOtherConfigurationInOutputDir(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outputDir, "versions.tf"), []byte("terraform {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var warnings strings.Builder
	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: outputDir,
		usedNames: make(map[string]bool),
		warnings:  &warnings,
	}

	if err := g.Generate(); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(warnings.String(), "required_providers") {
		t.Errorf("expected a warning about a second required_providers entry, got %q", warnings.String())
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
			{"Name": "Lists", "CollectionType": "playlists", "Locations": []string{"/media/lists"}},
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
	if !strings.Contains(warnings.String(), `"Lists"`) {
		t.Errorf("expected a warning naming the skipped library \"Lists\", got %q", warnings.String())
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

func TestFromEnvFillsOnlyFlagsTheCommandLineLeavesOut(t *testing.T) {
	t.Setenv("JELLYFIN_API_KEY", "key-from-env")
	t.Setenv("JELLYFIN_ENDPOINT", "http://env.test")

	fs := flag.NewFlagSet("jellyfin-import", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "")
	apiKey := fs.String("api-key", "", "")
	username := fs.String("username", "", "")
	if err := fs.Parse([]string{"-api-key=", "-username=admin"}); err != nil {
		t.Fatal(err)
	}
	fromEnv(fs, "endpoint", "JELLYFIN_ENDPOINT")
	fromEnv(fs, "api-key", "JELLYFIN_API_KEY")
	fromEnv(fs, "username", "JELLYFIN_USERNAME")

	if *endpoint != "http://env.test" {
		t.Errorf("endpoint = %q, want the environment's", *endpoint)
	}
	if *apiKey != "" {
		t.Errorf("api-key set to empty = %q, want it kept empty", *apiKey)
	}
	if *username != "admin" {
		t.Errorf("username = %q, want the command line's", *username)
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
	testAccBootstrap(t, endpoint)

	c, err := importClient(context.Background(), endpoint, apiKey, username, password)
	if err != nil {
		t.Fatalf("failed to configure Jellyfin import acceptance test client: %v", err)
	}
	return c
}

// testAccBootstrap completes a fresh server's startup wizard the way a first
// terraform run does, by configuring the provider. Until then Jellyfin has no
// user for the importer to log in as.
func testAccBootstrap(t *testing.T, endpoint string) {
	t.Helper()

	info, err := client.NewClient(endpoint, "").GetPublicSystemInfo(context.Background())
	if err != nil {
		t.Fatalf("reading Jellyfin startup status: %v", err)
	}
	if info.StartupWizardCompleted {
		return
	}
	testAccTerraform(t, resource.TestStep{
		Config: terraformBlock + `data "jellyfin_system_info" "bootstrap" {}`,
	})
}

// testAccTerraform runs steps with this provider in-process, registered under
// the source address the generated terraform block requires.
func testAccTerraform(t *testing.T, steps ...resource.TestStep) {
	t.Helper()

	namespace, _, _ := strings.Cut(providerSource, "/")
	t.Setenv(resource.EnvTfAccProviderNamespace, namespace)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"jellyfin": providerserver.NewProtocol6WithError(provider.New("test")()),
		},
		Steps: steps,
	})
}

// terraformFmtCheck fails the test if terraform fmt, run from PATH, would
// rewrite a file in dir.
func terraformFmtCheck(t *testing.T, dir string) {
	t.Helper()

	if out, err := exec.Command("terraform", "fmt", "-check", "-diff", dir).CombinedOutput(); err != nil {
		t.Errorf("terraform fmt -check: %v\n%s", err, out)
	}
}

// seedFixtures gives the server what the importer has to handle for the
// generated files to validate, and puts the server back when the test ends:
// strings that HCL would interpolate or reject unless escaped, metadata
// options for an item type Jellyfin lists no fetchers for, API keys whose
// names sanitize to the same resource name or to its suffixed form, and a
// music videos library and one without a collection type, which the importer
// writes as mixed.
func seedFixtures(t *testing.T, c *client.Client) {
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
		return c.UpdateBrandingConfiguration(ctx, branding)
	})
	seeded, err := json.Marshal(map[string]interface{}{
		"LoginDisclaimer":     tricky + "\nsecond line\twith a tab and \x01",
		"CustomCss":           "body {\n  color: red;\n}\n/* ${x} %{y} */\n",
		"SplashscreenEnabled": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateBrandingConfiguration(ctx, string(seeded)); err != nil {
		t.Fatalf("seeding branding configuration: %v", err)
	}

	system, err := c.GetSystemConfiguration(ctx)
	if err != nil {
		t.Fatalf("reading system configuration: %v", err)
	}
	restore = append(restore, func(c *client.Client) error {
		return c.UpdateSystemConfiguration(ctx, system)
	})
	var systemDoc map[string]interface{}
	if err := json.Unmarshal([]byte(system), &systemDoc); err != nil {
		t.Fatalf("parsing system configuration: %v", err)
	}
	options, _ := systemDoc["MetadataOptions"].([]interface{})
	systemDoc["MetadataOptions"] = append(options, map[string]interface{}{
		"ItemType":                 "Person",
		"DisabledMetadataSavers":   []string{},
		"LocalMetadataReaderOrder": []string{},
		"DisabledMetadataFetchers": []string{"TheMovieDb"},
		"MetadataFetcherOrder":     []string{},
		"DisabledImageFetchers":    []string{"TheMovieDb"},
		"ImageFetcherOrder":        []string{},
	})
	seededSystem, err := json.Marshal(systemDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateSystemConfiguration(ctx, string(seededSystem)); err != nil {
		t.Fatalf("seeding system configuration: %v", err)
	}

	keysBefore, err := c.GetAPIKeys(ctx)
	if err != nil {
		t.Fatalf("reading API keys: %v", err)
	}
	restore = append(restore, func(c *client.Client) error {
		keys, err := c.GetAPIKeys(ctx)
		if err != nil {
			return err
		}
		var errs []error
		for _, key := range keys {
			if !slices.ContainsFunc(keysBefore, func(k client.APIKey) bool { return k.AccessToken == key.AccessToken }) {
				errs = append(errs, c.DeleteAPIKey(ctx, key.AccessToken))
			}
		}
		return errors.Join(errs...)
	})
	for _, appName := range []string{tricky, "import-e2e dup", "import-e2e dup", "import-e2e dup 1"} {
		if err := c.CreateAPIKey(ctx, appName); err != nil {
			t.Fatalf("seeding API key %q: %v", appName, err)
		}
	}

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

	// Library names cannot hold quotes or backslashes. An empty collection
	// type leaves the library without one, as the web UI's "Mixed Movies and
	// Shows" does.
	libraries := []struct{ name, collectionType string }{
		{"import-e2e ${lib} %{x} $${y}", "movies"},
		{"import-e2e clips", "musicvideos"},
		{"import-e2e untyped", ""},
	}
	for _, lib := range libraries {
		if err := c.AddVirtualFolder(ctx, lib.name, lib.collectionType, []string{"/media/movies"}, &client.LibraryOptions{RawJSON: "{}"}); err != nil {
			t.Fatalf("seeding library %q: %v", lib.name, err)
		}
		restore = append(restore, func(c *client.Client) error {
			return c.RemoveVirtualFolder(ctx, lib.name)
		})
	}
}

// TestAccImportToolE2E runs the import tool against a real Jellyfin instance,
// checks the generated files with terraform fmt and plans them, as written,
// with Terraform and this provider: the plan must import every resource
// without changes, and each imported resource's configuration must set every
// value its imported state holds, apart from the attributes rendered leaves
// out.
// Set JELLYFIN_ENDPOINT and either JELLYFIN_API_KEY or JELLYFIN_USERNAME/JELLYFIN_PASSWORD to enable this test.
func TestAccImportToolE2E(t *testing.T) {
	outputDir := t.TempDir()
	c := testAccImportClient(t)
	seedFixtures(t, c)

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

	if !regexp.MustCompile(`disabled_metadata_fetchers\s+=\s+\["TheMovieDb"\]`).MatchString(resourcesStr) {
		t.Error("resources.tf should keep the disabled metadata fetchers of the seeded Person metadata options")
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

	terraformFmtCheck(t, outputDir)

	// PlanOnly never applies, so no state is saved and nothing on the server
	// is changed or destroyed.
	testAccTerraform(t, resource.TestStep{
		Config:   importsStr + "\n" + resourcesStr,
		PlanOnly: true,
		ConfigPlanChecks: resource.ConfigPlanChecks{
			PostApplyPreRefresh: []plancheck.PlanCheck{
				configMatchesImportedState{schemas: providerSchemas(t)},
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

// Jellyfin lists each version of a plugin it holds, and a plugin it deletes at
// the next restart, under the plugin's ID; the import reads one entry per ID.
func TestGeneratePluginsImportsEachPluginOnce(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/Plugins", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]any{
			{"Id": "foo-id", "Name": "Foo", "Version": "1.0.0.0", "Status": "Superseded"},
			{"Id": "foo-id", "Name": "Foo", "Version": "2.0.0.0", "Status": "Restart"},
			{"Id": "bar-id", "Name": "Bar", "Version": "3.0.0.0", "Status": "Deleted"},
		})
	})
	mux.HandleFunc("/Packages", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []any{})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	g := &generator{client: client.NewClient(server.URL, "k"), outputDir: t.TempDir(), usedNames: map[string]bool{}}
	imports, resources, err := g.generatePlugins()
	if err != nil {
		t.Fatalf("generatePlugins() error: %v", err)
	}
	if len(imports) != 1 || !strings.Contains(imports[0], `id = "foo-id"`) {
		t.Fatalf("imports = %q, want Foo alone", imports)
	}
	if !strings.Contains(resources[0], `version = "2.0.0.0"`) {
		t.Errorf("resource = %s, want the version the import reads", resources[0])
	}
}

// jellyfin_plugin_repository imports by name, which cannot tell apart two
// repositories with the same name.
func TestGeneratePluginRepositoriesSkipsSharedNames(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/Repositories", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]any{
			{"Name": "Repo", "Url": "https://a", "Enabled": true},
			{"Name": "Repo", "Url": "https://b", "Enabled": true},
			{"Name": "Other", "Url": "https://c", "Enabled": true},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	var warnings strings.Builder
	g := &generator{client: client.NewClient(server.URL, "k"), outputDir: t.TempDir(), usedNames: map[string]bool{}, warnings: &warnings}
	imports, _, err := g.generatePluginRepositories()
	if err != nil {
		t.Fatalf("generatePluginRepositories() error: %v", err)
	}
	if len(imports) != 1 || !strings.Contains(imports[0], `id = "Other"`) {
		t.Errorf("imports = %q, want Other alone", imports)
	}
	if strings.Count(warnings.String(), "skipping plugin repository") != 2 {
		t.Errorf("warnings = %q, want one for each repository named Repo", warnings.String())
	}
}
