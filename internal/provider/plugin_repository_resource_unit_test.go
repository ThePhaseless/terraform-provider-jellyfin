// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func TestFindPluginRepositoryIndex(t *testing.T) {
	t.Parallel()

	repos := []client.PluginRepository{
		{Name: "stable", Url: "https://stable.example/manifest.json", Enabled: true},
		{Name: "testing", Url: "https://testing.example/manifest.json", Enabled: true},
		{Name: "stable", Url: "https://mirror.example/manifest.json", Enabled: true},
	}

	tests := []struct {
		name      string
		repos     []client.PluginRepository
		repoName  string
		repoURL   string
		wantIndex int
		wantErr   bool
	}{
		{
			name:      "unique name matches directly",
			repos:     repos[:2],
			repoName:  "testing",
			wantIndex: 1,
		},
		{
			name:      "duplicate names can be disambiguated by URL",
			repos:     repos,
			repoName:  "stable",
			repoURL:   "https://mirror.example/manifest.json",
			wantIndex: 2,
		},
		{
			name:     "duplicate names without URL are ambiguous",
			repos:    repos,
			repoName: "stable",
			wantErr:  true,
		},
		{
			name:      "missing repository returns sentinel index",
			repos:     repos[:2],
			repoName:  "missing",
			wantIndex: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, err := findPluginRepositoryIndex(tt.repos, tt.repoName, tt.repoURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if index != tt.wantIndex {
				t.Fatalf("expected index %d, got %d", tt.wantIndex, index)
			}
		})
	}
}
