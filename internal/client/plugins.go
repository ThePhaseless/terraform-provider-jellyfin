// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sync"
)

// pluginChangeMu serialises plugin installs and uninstalls. Jellyfin's plugin
// manager adds and removes entries in an unsynchronised list, so overlapping
// requests fail with ArgumentOutOfRangeException or a missing meta.json and can
// leave that list inconsistent until the server restarts.
var pluginChangeMu sync.Mutex

// PluginRepository represents a plugin repository.
type PluginRepository struct {
	Name    string `json:"Name"`
	URL     string `json:"Url"`
	Enabled bool   `json:"Enabled"`
}

// InstalledPlugin represents a plugin installed on the server.
type InstalledPlugin struct {
	Name         string `json:"Name"`
	Version      string `json:"Version"`
	ID           string `json:"Id"`
	Description  string `json:"Description"`
	Status       string `json:"Status"`
	CanUninstall bool   `json:"CanUninstall"`
	HasImage     bool   `json:"HasImage"`
}

// PackageInfo represents information about an available package.
type PackageInfo struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Versions    []VersionInfo `json:"versions"`
}

// VersionInfo represents information about a specific version of a package.
type VersionInfo struct {
	Version        string `json:"version"`
	VersionNumber  string `json:"VersionNumber"`
	TargetAbi      string `json:"targetAbi"`
	SourceURL      string `json:"sourceUrl"`
	Checksum       string `json:"checksum"`
	Timestamp      string `json:"timestamp"`
	RepositoryName string `json:"repositoryName"`
	RepositoryURL  string `json:"repositoryUrl"`
}

// GetPluginRepositories retrieves all configured plugin repositories.
func (c *Client) GetPluginRepositories(ctx context.Context) ([]PluginRepository, error) {
	var repos []PluginRepository
	if err := c.get(ctx, "/Repositories", func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(&repos)
	}); err != nil {
		return nil, fmt.Errorf("getting plugin repositories: %w", err)
	}
	return repos, nil
}

// SetPluginRepositories replaces all plugin repositories with the given list.
func (c *Client) SetPluginRepositories(ctx context.Context, repos []PluginRepository) error {
	jsonBody, err := json.Marshal(repos)
	if err != nil {
		return fmt.Errorf("marshaling plugin repositories: %w", err)
	}
	if err := c.post(ctx, "/Repositories", jsonBody); err != nil {
		return fmt.Errorf("setting plugin repositories: %w", err)
	}
	return nil
}

// GetInstalledPlugins retrieves all installed plugins.
func (c *Client) GetInstalledPlugins(ctx context.Context) ([]InstalledPlugin, error) {
	var plugins []InstalledPlugin
	if err := c.get(ctx, "/Plugins", func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(&plugins)
	}); err != nil {
		return nil, fmt.Errorf("getting installed plugins: %w", err)
	}
	return plugins, nil
}

// InstallPlugin installs a plugin by name and version from a specific repository.
func (c *Client) InstallPlugin(ctx context.Context, name, version, repositoryURL string) error {
	params := url.Values{}
	params.Set("version", version)
	params.Set("repositoryUrl", repositoryURL)

	path := fmt.Sprintf("/Packages/Installed/%s?%s", url.PathEscape(name), params.Encode())

	pluginChangeMu.Lock()
	defer pluginChangeMu.Unlock()
	if err := c.post(ctx, path, nil); err != nil {
		return fmt.Errorf("installing plugin %s version %s: %w", name, version, err)
	}
	return nil
}

// UninstallPluginVersion removes one installed version of a plugin. version
// must be spelled as GET /Plugins lists it: Jellyfin parses it as a .NET
// Version, which tells 13.0.0 and 13.0.0.0 apart, and answers 404 for a
// version it does not list.
func (c *Client) UninstallPluginVersion(ctx context.Context, pluginID, version string) error {
	pluginChangeMu.Lock()
	defer pluginChangeMu.Unlock()
	if err := c.delete(ctx, fmt.Sprintf("/Plugins/%s/%s", url.PathEscape(pluginID), url.PathEscape(version))); err != nil {
		return fmt.Errorf("uninstalling plugin %s version %s: %w", pluginID, version, err)
	}
	return nil
}

// GetPluginConfiguration retrieves the configuration for a plugin as raw JSON.
func (c *Client) GetPluginConfiguration(ctx context.Context, pluginID string) (string, error) {
	raw, err := c.getRaw(ctx, fmt.Sprintf("/Plugins/%s/Configuration", url.PathEscape(pluginID)))
	if err != nil {
		return "", fmt.Errorf("getting configuration for plugin %s: %w", pluginID, err)
	}
	return raw, nil
}

// UpdatePluginConfiguration updates the configuration for a plugin with raw JSON.
func (c *Client) UpdatePluginConfiguration(ctx context.Context, pluginID string, configJSON string) error {
	path := fmt.Sprintf("/Plugins/%s/Configuration", url.PathEscape(pluginID))
	if err := c.postRaw(ctx, path, configJSON); err != nil {
		return fmt.Errorf("updating configuration for plugin %s: %w", pluginID, err)
	}
	return nil
}

// GetAvailablePackages retrieves all available packages from configured repositories.
func (c *Client) GetAvailablePackages(ctx context.Context) ([]PackageInfo, error) {
	var packages []PackageInfo
	if err := c.get(ctx, "/Packages", func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(&packages)
	}); err != nil {
		return nil, fmt.Errorf("getting available packages: %w", err)
	}
	return packages, nil
}
