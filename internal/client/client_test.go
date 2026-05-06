// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetVirtualFoldersUsesJellyfinItemIDCasing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]string{
			{
				"Name":           "Movies",
				"CollectionType": "movies",
				"ItemId":         "item-1",
			},
		})
	}))
	defer server.Close()

	folders, err := NewClient(server.URL, "test-key").GetVirtualFolders(context.Background())
	if err != nil {
		t.Fatalf("GetVirtualFolders() error = %v", err)
	}
	if got := folders[0].ItemID; got != "item-1" {
		t.Fatalf("ItemID = %q, want %q", got, "item-1")
	}
}

func TestGetAvailablePackagesUsesJellyfinRepositoryURLCasing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{
				"name": "MusicBrainz",
				"versions": []map[string]string{
					{
						"version":       "14.0.0.0",
						"repositoryUrl": "https://repo.example/manifest.json",
					},
				},
			},
		})
	}))
	defer server.Close()

	packages, err := NewClient(server.URL, "test-key").GetAvailablePackages(context.Background())
	if err != nil {
		t.Fatalf("GetAvailablePackages() error = %v", err)
	}
	if got := packages[0].Versions[0].RepositoryURL; got != "https://repo.example/manifest.json" {
		t.Fatalf("RepositoryURL = %q, want %q", got, "https://repo.example/manifest.json")
	}
}

func TestInstallPluginUsesJellyfinRepositoryURLQueryCasing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("repositoryUrl"); got != "https://repo.example/manifest.json" {
			t.Fatalf("repositoryUrl query = %q, want %q", got, "https://repo.example/manifest.json")
		}
		if got := r.URL.Query().Get("repositoryURL"); got != "" {
			t.Fatalf("repositoryURL query = %q, want empty", got)
		}
	}))
	defer server.Close()

	if err := NewClient(server.URL, "test-key").InstallPlugin(context.Background(), "MusicBrainz", "14.0.0.0", "https://repo.example/manifest.json"); err != nil {
		t.Fatalf("InstallPlugin() error = %v", err)
	}
}

func TestUserAndAuthResponsesUseJellyfinIDCasing(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/Users", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []map[string]interface{}{
			{
				"Id":   "user-1",
				"Name": "admin",
				"Policy": map[string]string{
					"AuthenticationProviderId": "auth-provider",
					"PasswordResetProviderId":  "password-reset-provider",
				},
			},
		})
	})
	mux.HandleFunc("/Users/AuthenticateByName", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]string{
			"AccessToken": "token-1",
			"ServerId":    "server-1",
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	users, err := client.GetUsers(context.Background())
	if err != nil {
		t.Fatalf("GetUsers() error = %v", err)
	}
	if got := users[0].Policy.AuthenticationProviderID; got != "auth-provider" {
		t.Fatalf("AuthenticationProviderID = %q, want %q", got, "auth-provider")
	}
	if got := users[0].Policy.PasswordResetProviderID; got != "password-reset-provider" {
		t.Fatalf("PasswordResetProviderID = %q, want %q", got, "password-reset-provider")
	}

	auth, err := client.AuthenticateByName(context.Background(), "admin", "password")
	if err != nil {
		t.Fatalf("AuthenticateByName() error = %v", err)
	}
	if got := auth.ServerID; got != "server-1" {
		t.Fatalf("ServerID = %q, want %q", got, "server-1")
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v interface{}) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encoding response: %v", err)
	}
}
