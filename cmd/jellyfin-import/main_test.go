// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func TestSanitizeName(t *testing.T) {
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
			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", `"hello"`},
		{`say "hi"`, `"say \"hi\""`},
		{`back\slash`, `"back\\slash"`},
		{"", `""`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := quote(tt.input)
			if result != tt.expected {
				t.Errorf("quote(%q) = %q, want %q", tt.input, result, tt.expected)
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

func TestPrettyJSON(t *testing.T) {
	result, err := prettyJSON(`{"b":2,"a":1}`)
	if err != nil {
		t.Fatalf("prettyJSON() error: %v", err)
	}
	if !strings.Contains(result, "\n") {
		t.Error("prettyJSON() should produce multi-line output")
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

// setupTestServer creates a mock Jellyfin server for testing.
func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/Users", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"Id":   "user-id-1",
				"Name": "admin",
				"Policy": map[string]interface{}{
					"IsAdministrator": true,
					"IsDisabled":      false,
					"EnableAllFolders": true,
				},
			},
			{
				"Id":   "user-id-2",
				"Name": "viewer",
				"Policy": map[string]interface{}{
					"IsAdministrator": false,
					"IsDisabled":      false,
					"EnableAllFolders": false,
				},
			},
		})
	})

	mux.HandleFunc("/Library/VirtualFolders", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"Name":           "Movies",
				"CollectionType": "movies",
				"Locations":      []string{"/media/movies"},
				"ItemId":         "item-1",
			},
		})
	})

	mux.HandleFunc("/Auth/Keys", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"Items": []map[string]interface{}{
				{
					"AccessToken": "test-token-123",
					"AppName":     "MyApp",
				},
			},
		})
	})

	mux.HandleFunc("/Repositories", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"Name":    "Jellyfin Stable",
				"Url":     "https://repo.jellyfin.org/files/plugin/manifest.json",
				"Enabled": true,
			},
		})
	})

	mux.HandleFunc("/Plugins", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"Name":    "MusicBrainz",
				"Version": "14.0.0.0",
				"Id":      "plugin-id-1",
			},
		})
	})

	mux.HandleFunc("/ScheduledTasks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{
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

	mux.HandleFunc("/System/Configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ServerName": "Test Server",
			"IsStartupWizardCompleted": true,
		})
	})

	mux.HandleFunc("/System/Configuration/encoding", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"EncodingThreadCount": -1,
		})
	})

	mux.HandleFunc("/System/Configuration/network", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"BaseUrl":    "",
			"EnableHttps": false,
		})
	})

	mux.HandleFunc("/System/Configuration/branding", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"SplashscreenEnabled": false,
		})
	})

	mux.HandleFunc("/System/Configuration/livetv", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"EnableRecordingSubfolders": false,
		})
	})

	mux.HandleFunc("/System/Configuration/metadata", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
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
}

func TestGenerateSingletonConfigs(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
	}

	imports, resources, err := g.generateSingletonConfigs()
	if err != nil {
		t.Fatalf("generateSingletonConfigs() error: %v", err)
	}

	// 6 singleton configs
	if len(imports) != 6 {
		t.Errorf("expected 6 import blocks, got %d", len(imports))
	}
	if len(resources) != 6 {
		t.Errorf("expected 6 resource blocks, got %d", len(resources))
	}

	// Check system config
	if !strings.Contains(resources[0], `server_name = "Test Server"`) {
		t.Errorf("expected server_name in system config: %s", resources[0])
	}
}

func TestGeneratePlugins(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
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
}

func TestGeneratePluginRepositories(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal Server Error")
	}))
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
	}

	err := g.Generate()
	if err == nil {
		t.Error("expected error from Generate(), got nil")
	}
}

func TestSanitizeNameEdgeCases(t *testing.T) {
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
			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
