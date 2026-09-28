// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
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

func TestAuthenticateByNameRejectionCarriesStatusCode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("Invalid username or password entered."))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "").AuthenticateByName(context.Background(), "viewer", "wrong")
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("AuthenticateByName() error = %v, want an HTTPError with status 401", err)
	}
}

func TestGetUserPolicyRawKeepsFieldsMissingFromUserPolicy(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"Id":   "user-1",
			"Name": "viewer",
			"Policy": map[string]interface{}{
				"MaxParentalRating":    10,
				"MaxParentalSubRating": 2,
			},
		})
	}))
	defer server.Close()

	raw, err := NewClient(server.URL, "test-key").GetUserPolicyRaw(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetUserPolicyRaw() error = %v", err)
	}

	var policy map[string]int
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		t.Fatalf("parsing policy %s: %v", raw, err)
	}
	if got := policy["MaxParentalSubRating"]; got != 2 {
		t.Fatalf("MaxParentalSubRating = %d, want 2 (policy %s)", got, raw)
	}
	if len(policy) != 2 {
		t.Fatalf("policy = %s, want only the fields the server sent", raw)
	}
}

func TestUpdateUserRawPostsBodyToUsersWithUserIDQuery(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath, gotUserID, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotUserID = r.Method, r.URL.Path, r.URL.Query().Get("userId")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		gotBody = string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	userJSON := `{"Id":"user-1","Name":"renamed","Configuration":{"SubtitleLanguagePreference":"fre"}}`
	if err := NewClient(server.URL, "test-key").UpdateUserRaw(context.Background(), "user-1", userJSON); err != nil {
		t.Fatalf("UpdateUserRaw() error = %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/Users" || gotUserID != "user-1" {
		t.Fatalf("expected POST /Users?userId=user-1, got %s %s with userId %q", gotMethod, gotPath, gotUserID)
	}
	if gotBody != userJSON {
		t.Fatalf("body = %s, want the user JSON passed in unchanged", gotBody)
	}
}

func TestUpdateUserPasswordPostsPasswordsToUsersPasswordWithUserIDQuery(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath, gotUserID string
	var gotBody map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotUserID = r.Method, r.URL.Path, r.URL.Query().Get("userId")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := NewClient(server.URL, "test-key").UpdateUserPassword(context.Background(), "user-1", "old", "new"); err != nil {
		t.Fatalf("UpdateUserPassword() error = %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/Users/Password" || gotUserID != "user-1" {
		t.Fatalf("expected POST /Users/Password?userId=user-1, got %s %s with userId %q", gotMethod, gotPath, gotUserID)
	}
	want := map[string]string{"CurrentPw": `"old"`, "NewPw": `"new"`}
	if len(gotBody) != len(want) {
		t.Fatalf("body has fields %v, want only CurrentPw and NewPw", slices.Sorted(maps.Keys(gotBody)))
	}
	for field, value := range want {
		if got := string(gotBody[field]); got != value {
			t.Errorf("%s = %s, want %s", field, got, value)
		}
	}
}

func TestUserUpdatesRejectBlankIDWithoutSendingRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-key")
	for _, id := range []string{"", " ", "\t"} {
		if err := c.UpdateUserRaw(context.Background(), id, `{"Name":"renamed"}`); !errors.Is(err, errBlankUserID) {
			t.Errorf("UpdateUserRaw(%q) error = %v, want errBlankUserID", id, err)
		}
		if err := c.UpdateUserPassword(context.Background(), id, "", "new"); !errors.Is(err, errBlankUserID) {
			t.Errorf("UpdateUserPassword(%q) error = %v, want errBlankUserID", id, err)
		}
	}
}

func TestGetScheduledTaskKeepsTheServedDocumentAndReportsAMissingTask(t *testing.T) {
	t.Parallel()

	const served = `{"Id":"task-1","Name":"Clean Logs","Triggers":[{"Type":"StartupTrigger"}],"LastExecutionResult":{"Status":"Completed"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ScheduledTasks/task-1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, served)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-key")
	task, err := c.GetScheduledTask(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("GetScheduledTask() error = %v", err)
	}
	if task.RawJSON != served {
		t.Errorf("RawJSON = %s, want %s", task.RawJSON, served)
	}
	if task.ID != "task-1" || len(task.Triggers) != 1 {
		t.Errorf("task = %+v, want Id task-1 with one trigger", task)
	}

	if _, err := c.GetScheduledTask(context.Background(), "missing"); !IsNotFound(err) {
		t.Errorf("GetScheduledTask(missing) error = %v, want a not-found error", err)
	}
}

func TestRestartServerPostsSystemRestart(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := NewClient(server.URL, "test-key").RestartServer(context.Background()); err != nil {
		t.Fatalf("RestartServer() error = %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/System/Restart" {
		t.Fatalf("expected POST /System/Restart, got %s %s", gotMethod, gotPath)
	}
}

func TestDirectoryExistsAsksValidatePathForADirectory(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/Environment/ValidatePath" {
			t.Errorf("expected POST /Environment/ValidatePath, got %s %s", r.Method, r.URL.Path)
		}
		var req struct {
			Path   string
			IsFile *bool
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		if req.IsFile == nil || *req.IsFile {
			t.Errorf("IsFile = %v, want false", req.IsFile)
		}
		switch req.Path {
		case "/media/movies":
			w.WriteHeader(http.StatusNoContent)
		case "/media/missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-key")
	tests := map[string]struct {
		path    string
		want    bool
		wantErr bool
	}{
		"existing directory": {path: "/media/movies", want: true},
		"missing directory":  {path: "/media/missing", want: false},
		"other error":        {path: "/forbidden", wantErr: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := c.DirectoryExists(context.Background(), test.path)
			if (err != nil) != test.wantErr {
				t.Fatalf("DirectoryExists(%q) error = %v, wantErr %t", test.path, err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("DirectoryExists(%q) = %t, want %t", test.path, got, test.want)
			}
		})
	}
}

func TestGetAvailableLibraryOptionsAsksForTheContentType(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Libraries/AvailableOptions" || r.URL.Query().Get("libraryContentType") != "movies" {
			t.Errorf("expected GET /Libraries/AvailableOptions?libraryContentType=movies, got %s %s", r.Method, r.URL)
		}
		writeJSON(t, w, map[string]any{
			"SubtitleFetchers": []map[string]any{{"Name": "Open Subtitles", "DefaultEnabled": true}},
			"TypeOptions": []map[string]any{{
				"Type":                 "Movie",
				"MetadataFetchers":     []map[string]any{{"Name": "TheMovieDb", "DefaultEnabled": true}},
				"ImageFetchers":        []map[string]any{{"Name": "Screen Grabber", "DefaultEnabled": false}},
				"SimilarItemProviders": []map[string]any{{"Name": "Local Genre/Tag", "DefaultEnabled": true}},
			}},
		})
	}))
	defer server.Close()

	got, err := NewClient(server.URL, "test-key").GetAvailableLibraryOptions(context.Background(), "movies")
	if err != nil {
		t.Fatalf("GetAvailableLibraryOptions() error = %v", err)
	}
	want := AvailableLibraryOptions{
		SubtitleFetchers: []AvailableOption{{Name: "Open Subtitles"}},
		TypeOptions: []AvailableLibraryTypeInfo{{
			Type:                 "Movie",
			MetadataFetchers:     []AvailableOption{{Name: "TheMovieDb"}},
			ImageFetchers:        []AvailableOption{{Name: "Screen Grabber"}},
			SimilarItemProviders: []AvailableOption{{Name: "Local Genre/Tag"}},
		}},
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("GetAvailableLibraryOptions() = %+v, want %+v", *got, want)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v interface{}) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encoding response: %v", err)
	}
}
