// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

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
		name := sanitizeName(user.Name)
		imports = append(imports, importBlock("jellyfin_user", name, user.Id))

		attrs := map[string]string{
			"name":             quote(user.Name),
			"is_administrator": fmt.Sprintf("%t", user.Policy.IsAdministrator),
			"is_disabled":      fmt.Sprintf("%t", user.Policy.IsDisabled),
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
		name := sanitizeName(folder.Name)
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
		name := sanitizeName(key.AppName)
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
		name := sanitizeName(repo.Name)
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

	var imports, resources []string
	for _, plugin := range plugins {
		name := sanitizeName(plugin.Name)
		imports = append(imports, importBlock("jellyfin_plugin", name, plugin.Id))

		attrs := map[string]string{
			"name":    quote(plugin.Name),
			"version": quote(plugin.Version),
		}
		resources = append(resources, resourceBlock("jellyfin_plugin", name, attrs))
	}

	return imports, resources, nil
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

		name := sanitizeName(task.Name)
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
	path := g.outputDir + "/" + name
	return os.WriteFile(path, []byte(content+"\n"), 0o644)
}

// sanitizeName converts a human-readable name to a valid Terraform identifier.
func sanitizeName(name string) string {
	// Replace non-alphanumeric characters with underscores.
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	result := re.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "_")
	result = strings.Trim(result, "_")
	if result == "" {
		result = "unnamed"
	}
	// Ensure it starts with a letter.
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
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
	b.WriteString(fmt.Sprintf("resource %s %s {\n", quote(resourceType), quote(name)))

	// Write attributes in a consistent order
	keys := sortedKeys(attrs)
	for _, k := range keys {
		v := attrs[k]
		b.WriteString(fmt.Sprintf("  %s = %s\n", k, v))
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
	// Simple insertion sort for small maps.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// quote wraps a string in double quotes, escaping inner quotes.
func quote(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// prettyJSON formats a JSON string with indentation.
func prettyJSON(raw string) (string, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(v, "  ", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
