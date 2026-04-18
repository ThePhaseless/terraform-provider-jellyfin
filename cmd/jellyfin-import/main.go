// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func main() {
	endpoint := flag.String("endpoint", os.Getenv("JELLYFIN_ENDPOINT"), "Jellyfin server URL (or JELLYFIN_ENDPOINT env)")
	apiKey := flag.String("api-key", os.Getenv("JELLYFIN_API_KEY"), "Jellyfin API key (or JELLYFIN_API_KEY env)")
	outputDir := flag.String("output", ".", "Output directory for generated Terraform files")
	flag.Parse()

	if *endpoint == "" {
		fmt.Fprintln(os.Stderr, "Error: --endpoint or JELLYFIN_ENDPOINT is required")
		os.Exit(1)
	}
	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: --api-key or JELLYFIN_API_KEY is required")
		os.Exit(1)
	}

	c := client.NewClient(*endpoint, *apiKey)

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	g := &generator{
		client:    c,
		outputDir: *outputDir,
		usedNames: make(map[string]int),
	}

	if err := g.Generate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Import files generated successfully in", *outputDir)
}

type generator struct {
	client    *client.Client
	outputDir string
	usedNames map[string]int // tracks used resource addresses to avoid collisions
}

// uniqueName returns a unique Terraform resource name, appending a numeric suffix on collision.
func (g *generator) uniqueName(resourceType, baseName string) string {
	key := resourceType + "." + baseName
	count := g.usedNames[key]
	g.usedNames[key] = count + 1
	if count == 0 {
		return baseName
	}
	return fmt.Sprintf("%s_%d", baseName, count)
}

// Generate generates all Terraform files.
func (g *generator) Generate() error {
	var imports []string
	var resources []string

	// Users
	userImports, userResources, err := g.generateUsers()
	if err != nil {
		return fmt.Errorf("generating users: %w", err)
	}
	imports = append(imports, userImports...)
	resources = append(resources, userResources...)

	// Libraries
	libImports, libResources, err := g.generateLibraries()
	if err != nil {
		return fmt.Errorf("generating libraries: %w", err)
	}
	imports = append(imports, libImports...)
	resources = append(resources, libResources...)

	// API Keys
	keyImports, keyResources, err := g.generateAPIKeys()
	if err != nil {
		return fmt.Errorf("generating API keys: %w", err)
	}
	imports = append(imports, keyImports...)
	resources = append(resources, keyResources...)

	// Plugin Repositories
	repoImports, repoResources, err := g.generatePluginRepositories()
	if err != nil {
		return fmt.Errorf("generating plugin repositories: %w", err)
	}
	imports = append(imports, repoImports...)
	resources = append(resources, repoResources...)

	// Plugins
	pluginImports, pluginResources, err := g.generatePlugins()
	if err != nil {
		return fmt.Errorf("generating plugins: %w", err)
	}
	imports = append(imports, pluginImports...)
	resources = append(resources, pluginResources...)

	// Scheduled Tasks
	taskImports, taskResources, err := g.generateScheduledTasks()
	if err != nil {
		return fmt.Errorf("generating scheduled tasks: %w", err)
	}
	imports = append(imports, taskImports...)
	resources = append(resources, taskResources...)

	// Singleton configurations
	singletonImports, singletonResources, err := g.generateSingletonConfigs()
	if err != nil {
		return fmt.Errorf("generating configurations: %w", err)
	}
	imports = append(imports, singletonImports...)
	resources = append(resources, singletonResources...)

	// Write imports.tf
	if len(imports) > 0 {
		if err := g.writeFile("imports.tf", strings.Join(imports, "\n")); err != nil {
			return fmt.Errorf("writing imports.tf: %w", err)
		}
	}

	// Write resources.tf
	if len(resources) > 0 {
		if err := g.writeFile("resources.tf", strings.Join(resources, "\n")); err != nil {
			return fmt.Errorf("writing resources.tf: %w", err)
		}
	}

	return nil
}

func (g *generator) generateUsers() ([]string, []string, error) {
	users, err := g.client.GetUsers()
	if err != nil {
		return nil, nil, err
	}

	var imports, resources []string
	for _, user := range users {
		name := g.uniqueName("jellyfin_user", sanitizeName(user.Name))
		imports = append(imports, importBlock("jellyfin_user", name, user.Id))

		attrs := map[string]string{
			"name":               quote(user.Name),
			"is_administrator":   fmt.Sprintf("%t", user.Policy.IsAdministrator),
			"is_disabled":        fmt.Sprintf("%t", user.Policy.IsDisabled),
			"enable_all_folders": fmt.Sprintf("%t", user.Policy.EnableAllFolders),
		}
		resources = append(resources, resourceBlock("jellyfin_user", name, attrs))
	}

	return imports, resources, nil
}

func (g *generator) generateLibraries() ([]string, []string, error) {
	folders, err := g.client.GetVirtualFolders()
	if err != nil {
		return nil, nil, err
	}

	var imports, resources []string
	for _, folder := range folders {
		name := g.uniqueName("jellyfin_library", sanitizeName(folder.Name))
		imports = append(imports, importBlock("jellyfin_library", name, folder.Name))

		paths := make([]string, len(folder.Locations))
		for i, loc := range folder.Locations {
			paths[i] = quote(loc)
		}

		attrs := map[string]string{
			"name":            quote(folder.Name),
			"collection_type": quote(folder.CollectionType),
			"paths":           "[" + strings.Join(paths, ", ") + "]",
		}
		resources = append(resources, resourceBlock("jellyfin_library", name, attrs))
	}

	return imports, resources, nil
}

func (g *generator) generateAPIKeys() ([]string, []string, error) {
	keys, err := g.client.GetAPIKeys()
	if err != nil {
		return nil, nil, err
	}

	var imports, resources []string
	for _, key := range keys {
		name := g.uniqueName("jellyfin_api_key", sanitizeName(key.AppName))
		imports = append(imports, importBlock("jellyfin_api_key", name, key.AccessToken))

		attrs := map[string]string{
			"app_name": quote(key.AppName),
		}
		resources = append(resources, resourceBlock("jellyfin_api_key", name, attrs))
	}

	return imports, resources, nil
}

func (g *generator) generatePluginRepositories() ([]string, []string, error) {
	repos, err := g.client.GetPluginRepositories()
	if err != nil {
		return nil, nil, err
	}

	var imports, resources []string
	for _, repo := range repos {
		name := g.uniqueName("jellyfin_plugin_repository", sanitizeName(repo.Name))
		imports = append(imports, importBlock("jellyfin_plugin_repository", name, repo.Name))

		attrs := map[string]string{
			"name":    quote(repo.Name),
			"url":     quote(repo.Url),
			"enabled": fmt.Sprintf("%t", repo.Enabled),
		}
		resources = append(resources, resourceBlock("jellyfin_plugin_repository", name, attrs))
	}

	return imports, resources, nil
}

func (g *generator) generatePlugins() ([]string, []string, error) {
	plugins, err := g.client.GetInstalledPlugins()
	if err != nil {
		return nil, nil, err
	}

	// Try to resolve repository URLs from available packages.
	repoURLs := g.resolvePluginRepoURLs(plugins)

	var imports, resources []string
	for _, plugin := range plugins {
		name := g.uniqueName("jellyfin_plugin", sanitizeName(plugin.Name))
		imports = append(imports, importBlock("jellyfin_plugin", name, plugin.Id))

		repoURL := repoURLs[plugin.Id]

		attrs := map[string]string{
			"name":           quote(plugin.Name),
			"version":        quote(plugin.Version),
			"repository_url": quote(repoURL),
		}
		resources = append(resources, resourceBlock("jellyfin_plugin", name, attrs))
	}

	return imports, resources, nil
}

// resolvePluginRepoURLs tries to find the repository URL for each installed plugin
// by cross-referencing with available packages from configured repositories.
func (g *generator) resolvePluginRepoURLs(plugins []client.InstalledPlugin) map[string]string {
	result := make(map[string]string)
	for _, p := range plugins {
		result[p.Id] = ""
	}

	packages, err := g.client.GetAvailablePackages()
	if err != nil {
		// Non-fatal: we'll use empty repository URLs.
		return result
	}

	for _, p := range plugins {
		for _, pkg := range packages {
			if pkg.Name != p.Name {
				continue
			}
			for _, v := range pkg.Versions {
				if v.Version == p.Version {
					result[p.Id] = v.RepositoryUrl
					break
				}
			}
			if result[p.Id] != "" {
				break
			}
			// Fallback: use any version's repository URL for this package.
			if len(pkg.Versions) > 0 {
				result[p.Id] = pkg.Versions[0].RepositoryUrl
			}
			break
		}
	}

	return result
}

func (g *generator) generateScheduledTasks() ([]string, []string, error) {
	tasks, err := g.client.GetScheduledTasks()
	if err != nil {
		return nil, nil, err
	}

	var imports, resources []string
	for _, task := range tasks {
		if task.IsHidden {
			continue
		}

		name := g.uniqueName("jellyfin_scheduled_task", sanitizeName(task.Name))
		imports = append(imports, importBlock("jellyfin_scheduled_task", name, task.Id))

		triggersJSON, err := json.Marshal(task.Triggers)
		if err != nil {
			return nil, nil, fmt.Errorf("marshaling triggers for task %s: %w", task.Id, err)
		}

		prettyTriggers, err := prettyJSON(string(triggersJSON))
		if err != nil {
			return nil, nil, fmt.Errorf("formatting triggers for task %s: %w", task.Id, err)
		}

		attrs := map[string]string{
			"id":            quote(task.Id),
			"triggers_json": "jsonencode(" + prettyTriggers + ")",
		}
		resources = append(resources, resourceBlock("jellyfin_scheduled_task", name, attrs))
	}

	return imports, resources, nil
}

func (g *generator) generateSingletonConfigs() ([]string, []string, error) {
	var imports, resources []string

	// System Configuration
	sysConfig, err := g.client.GetSystemConfiguration()
	if err != nil {
		return nil, nil, fmt.Errorf("getting system configuration: %w", err)
	}
	pretty, err := prettyJSON(sysConfig.RawJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("formatting system configuration: %w", err)
	}
	imports = append(imports, importBlock("jellyfin_system_configuration", "this", "singleton"))
	resources = append(resources, resourceBlock("jellyfin_system_configuration", "this", map[string]string{
		"server_name":        quote(sysConfig.ServerName),
		"configuration_json": "jsonencode(" + pretty + ")",
	}))

	// Encoding Configuration
	encConfig, err := g.client.GetEncodingOptions()
	if err != nil {
		return nil, nil, fmt.Errorf("getting encoding configuration: %w", err)
	}
	pretty, err = prettyJSON(encConfig.RawJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("formatting encoding configuration: %w", err)
	}
	imports = append(imports, importBlock("jellyfin_encoding_configuration", "this", "singleton"))
	resources = append(resources, resourceBlock("jellyfin_encoding_configuration", "this", map[string]string{
		"configuration_json": "jsonencode(" + pretty + ")",
	}))

	// Networking Configuration
	netConfig, err := g.client.GetNetworkConfiguration()
	if err != nil {
		return nil, nil, fmt.Errorf("getting networking configuration: %w", err)
	}
	pretty, err = prettyJSON(netConfig.RawJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("formatting networking configuration: %w", err)
	}
	imports = append(imports, importBlock("jellyfin_networking_configuration", "this", "singleton"))
	resources = append(resources, resourceBlock("jellyfin_networking_configuration", "this", map[string]string{
		"configuration_json": "jsonencode(" + pretty + ")",
	}))

	// Branding Configuration
	brandConfig, err := g.client.GetBrandingConfiguration()
	if err != nil {
		return nil, nil, fmt.Errorf("getting branding configuration: %w", err)
	}
	pretty, err = prettyJSON(brandConfig.RawJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("formatting branding configuration: %w", err)
	}
	imports = append(imports, importBlock("jellyfin_branding_configuration", "this", "singleton"))
	resources = append(resources, resourceBlock("jellyfin_branding_configuration", "this", map[string]string{
		"configuration_json": "jsonencode(" + pretty + ")",
	}))

	// Live TV Configuration
	livetvConfig, err := g.client.GetLiveTVConfiguration()
	if err != nil {
		return nil, nil, fmt.Errorf("getting livetv configuration: %w", err)
	}
	pretty, err = prettyJSON(livetvConfig.RawJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("formatting livetv configuration: %w", err)
	}
	imports = append(imports, importBlock("jellyfin_livetv_configuration", "this", "singleton"))
	resources = append(resources, resourceBlock("jellyfin_livetv_configuration", "this", map[string]string{
		"configuration_json": "jsonencode(" + pretty + ")",
	}))

	// Metadata Configuration
	metaConfig, err := g.client.GetMetadataConfiguration()
	if err != nil {
		return nil, nil, fmt.Errorf("getting metadata configuration: %w", err)
	}
	pretty, err = prettyJSON(metaConfig.RawJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("formatting metadata configuration: %w", err)
	}
	imports = append(imports, importBlock("jellyfin_metadata_configuration", "this", "singleton"))
	resources = append(resources, resourceBlock("jellyfin_metadata_configuration", "this", map[string]string{
		"configuration_json": "jsonencode(" + pretty + ")",
	}))

	return imports, resources, nil
}

func (g *generator) writeFile(name, content string) error {
	p := filepath.Join(g.outputDir, name)
	return os.WriteFile(p, []byte(content+"\n"), 0o644)
}

// sanitizeName converts a human-readable name to a valid Terraform identifier.
func sanitizeName(name string) string {
	result := sanitizeRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "_")
	result = strings.Trim(result, "_")
	if result == "" {
		result = "unnamed"
	}
	// Ensure it starts with a letter.
	if result[0] >= '0' && result[0] <= '9' {
		result = "r_" + result
	}
	return result
}

// importBlock generates a Terraform import block.
func importBlock(resourceType, name, id string) string {
	return fmt.Sprintf(`import {
  to = %s.%s
  id = %s
}
`, resourceType, name, quote(id))
}

// resourceBlock generates a Terraform resource block from a map of attributes.
func resourceBlock(resourceType, name string, attrs map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "resource %s %s {\n", quote(resourceType), quote(name))

	keys := sortedKeys(attrs)
	for _, k := range keys {
		fmt.Fprintf(&b, "  %s = %s\n", k, attrs[k])
	}

	b.WriteString("}\n")
	return b.String()
}

// sortedKeys returns map keys in sorted order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// quote wraps a string in double quotes, escaping inner quotes.
func quote(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// prettyJSON formats a JSON string with indentation, preserving number precision.
func prettyJSON(raw string) (string, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "  ", "  "); err != nil {
		return "", err
	}
	return buf.String(), nil
}
