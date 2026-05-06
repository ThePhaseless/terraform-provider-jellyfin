// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
)

// PluginRepository represents a plugin repository.
type PluginRepository struct {
	Name    string `json:"Name"`
	Url     string `json:"Url"`
	Enabled bool   `json:"Enabled"`
}

// InstalledPlugin represents a plugin installed on the server.
type InstalledPlugin struct {
	Name         string `json:"Name"`
	Version      string `json:"Version"`
	Id           string `json:"Id"`
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
	SourceUrl      string `json:"sourceUrl"`
	Checksum       string `json:"checksum"`
	Timestamp      string `json:"timestamp"`
	RepositoryName string `json:"repositoryName"`
	RepositoryUrl  string `json:"repositoryUrl"`
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
func (c *Client) InstallPlugin(ctx context.Context, name, version, repositoryUrl string) error {
	params := url.Values{}
	params.Set("version", version)
	params.Set("repositoryUrl", repositoryUrl)

	path := fmt.Sprintf("/Packages/Installed/%s?%s", url.PathEscape(name), params.Encode())

	if err := c.post(ctx, path, nil); err != nil {
		return fmt.Errorf("installing plugin %s version %s: %w", name, version, err)
	}
	return nil
}

// UninstallPlugin removes an installed plugin by its ID.
func (c *Client) UninstallPlugin(ctx context.Context, pluginId string) error {
	if err := c.delete(ctx, fmt.Sprintf("/Plugins/%s", url.PathEscape(pluginId))); err != nil {
		return fmt.Errorf("uninstalling plugin %s: %w", pluginId, err)
	}
	return nil
}

// GetPluginConfiguration retrieves the configuration for a plugin as raw JSON.
func (c *Client) GetPluginConfiguration(ctx context.Context, pluginId string) (string, error) {
	raw, err := c.getRaw(ctx, fmt.Sprintf("/Plugins/%s/Configuration", url.PathEscape(pluginId)))
	if err != nil {
		return "", fmt.Errorf("getting configuration for plugin %s: %w", pluginId, err)
	}
	return raw, nil
}

// UpdatePluginConfiguration updates the configuration for a plugin with raw JSON.
func (c *Client) UpdatePluginConfiguration(ctx context.Context, pluginId string, configJSON string) error {
	path := fmt.Sprintf("/Plugins/%s/Configuration", url.PathEscape(pluginId))
	if err := c.postRaw(ctx, path, configJSON); err != nil {
		return fmt.Errorf("updating configuration for plugin %s: %w", pluginId, err)
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
