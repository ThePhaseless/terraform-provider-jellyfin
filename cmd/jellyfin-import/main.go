// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/provider"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func main() {
	// The environment fills in a flag only after parsing: as a flag default,
	// the usage text that -h or a mistyped flag prints would show the secrets.
	endpoint := flag.String("endpoint", "", "Jellyfin server URL (or JELLYFIN_ENDPOINT env)")
	apiKey := flag.String("api-key", "", "Jellyfin API key (or JELLYFIN_API_KEY env)")
	username := flag.String("username", "", "Jellyfin username (or JELLYFIN_USERNAME env)")
	password := flag.String("password", "", "Jellyfin password (or JELLYFIN_PASSWORD env)")
	outputDir := flag.String("output", ".", "Output directory for generated Terraform files")
	flag.Parse()
	fromEnv(endpoint, "JELLYFIN_ENDPOINT")
	fromEnv(apiKey, "JELLYFIN_API_KEY")
	fromEnv(username, "JELLYFIN_USERNAME")
	fromEnv(password, "JELLYFIN_PASSWORD")

	if *endpoint == "" {
		fmt.Fprintln(os.Stderr, "Error: --endpoint or JELLYFIN_ENDPOINT is required")
		os.Exit(1)
	}
	if *apiKey == "" && (*username == "" || *password == "") {
		fmt.Fprintln(os.Stderr, "Error: Either --api-key (or JELLYFIN_API_KEY) or both --username/--password (or JELLYFIN_USERNAME/JELLYFIN_PASSWORD) must be set")
		os.Exit(1)
	}

	c, err := importClient(context.Background(), *endpoint, *apiKey, *username, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error configuring Jellyfin client: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	g := &generator{
		client:    c,
		ctx:       context.Background(),
		outputDir: *outputDir,
		usedNames: make(map[string]bool),
	}

	if err := g.Generate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Import files generated successfully in", *outputDir)
}

// fromEnv sets an unset flag's value from the environment variable env.
func fromEnv(value *string, env string) {
	if *value == "" {
		*value = os.Getenv(env)
	}
}

func importClient(ctx context.Context, endpoint, apiKey, username, password string) (*client.Client, error) {
	c := client.NewClient(endpoint, apiKey)
	if apiKey != "" {
		return c, nil
	}
	if username == "" {
		return nil, fmt.Errorf("missing Jellyfin username; set --username or JELLYFIN_USERNAME")
	}
	if password == "" {
		return nil, fmt.Errorf("missing Jellyfin password; set --password or JELLYFIN_PASSWORD")
	}

	authResult, err := c.AuthenticateByName(ctx, username, password)
	if err != nil {
		return nil, fmt.Errorf("authenticating with Jellyfin: %w", err)
	}
	c.APIKey = authResult.AccessToken
	return c, nil
}

type generator struct {
	client    *client.Client
	ctx       context.Context
	outputDir string
	usedNames map[string]bool // resource addresses already handed out
	warnings  io.Writer
}

func (g *generator) context() context.Context {
	if g.ctx != nil {
		return g.ctx
	}
	return context.Background()
}

func (g *generator) warnf(format string, args ...any) {
	w := g.warnings
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, "Warning: "+format+"\n", args...)
}

// uniqueName returns a Terraform resource name that no earlier call returned
// for resourceType, appending the lowest numeric suffix that is still free.
// A suffixed name has to be checked as well, because another name can
// sanitize to it.
func (g *generator) uniqueName(resourceType, baseName string) string {
	name := baseName
	for i := 1; g.usedNames[resourceType+"."+name]; i++ {
		name = fmt.Sprintf("%s_%d", baseName, i)
	}
	g.usedNames[resourceType+"."+name] = true
	return name
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
		content := terraformBlock + "\n" + strings.Join(resources, "\n")
		if err := g.writeFile("resources.tf", content); err != nil {
			return fmt.Errorf("writing resources.tf: %w", err)
		}
		g.warnIfOtherConfiguration()
	}

	return nil
}

// terraformBlock names the provider's registry address, without which
// terraform init looks for hashicorp/jellyfin.
const terraformBlock = `terraform {
  required_providers {
    jellyfin = {
      source = "` + providerSource + `"
    }
  }
}
`

const providerSource = "ThePhaseless/jellyfin"

// warnIfOtherConfiguration points out that Terraform rejects a second
// required_providers entry for jellyfin, which configuration already in the
// output directory may have.
func (g *generator) warnIfOtherConfiguration() {
	files, err := filepath.Glob(filepath.Join(g.outputDir, "*.tf"))
	if err != nil {
		return
	}
	for _, f := range files {
		if name := filepath.Base(f); name != "imports.tf" && name != "resources.tf" {
			g.warnf("%s holds other Terraform files; if one of them already lists jellyfin in required_providers, remove the terraform block at the top of resources.tf", g.outputDir)
			return
		}
	}
}

func (g *generator) generateUsers() ([]string, []string, error) {
	users, err := g.client.GetUsers(g.context())
	if err != nil {
		return nil, nil, err
	}

	var imports, resources []string
	for _, user := range users {
		name := g.uniqueName("jellyfin_user", sanitizeName(user.Name))
		imports = append(imports, importBlock("jellyfin_user", name, user.ID))

		attrs := map[string]string{
			"name":               hclString(user.Name),
			"is_administrator":   fmt.Sprintf("%t", user.Policy.IsAdministrator),
			"is_disabled":        fmt.Sprintf("%t", user.Policy.IsDisabled),
			"enable_all_folders": fmt.Sprintf("%t", user.Policy.EnableAllFolders),
		}
		resources = append(resources, resourceBlock("jellyfin_user", name, attrs))
	}

	return imports, resources, nil
}

// libraryCollectionTypes are the collection types jellyfin_library accepts.
var libraryCollectionTypes = map[string]bool{
	"movies":      true,
	"tvshows":     true,
	"music":       true,
	"musicvideos": true,
	"books":       true,
	"homevideos":  true,
	"boxsets":     true,
	"mixed":       true,
}

func (g *generator) generateLibraries() ([]string, []string, error) {
	folders, err := g.client.GetVirtualFolders(g.context())
	if err != nil {
		return nil, nil, err
	}

	var imports, resources []string
	for _, folder := range folders {
		// Jellyfin's web UI creates a mixed library without a collection type;
		// the provider reads that as mixed and rejects "".
		collectionType := folder.CollectionType
		if collectionType == "" {
			collectionType = "mixed"
		}
		// collection_type is checked against a fixed list and replaces the
		// library when it changes, so for any other type no configuration both
		// passes validation and matches the imported state.
		if !libraryCollectionTypes[collectionType] {
			g.warnf("skipping library %q: jellyfin_library does not accept its collection type %q", folder.Name, folder.CollectionType)
			continue
		}

		name := g.uniqueName("jellyfin_library", sanitizeName(folder.Name))
		imports = append(imports, importBlock("jellyfin_library", name, folder.Name))

		paths := make([]string, len(folder.Locations))
		for i, loc := range folder.Locations {
			paths[i] = hclString(loc)
		}

		attrs := map[string]string{
			"name":            hclString(folder.Name),
			"collection_type": hclString(collectionType),
			"paths":           "[" + strings.Join(paths, ", ") + "]",
		}
		resources = append(resources, resourceBlock("jellyfin_library", name, attrs))
	}

	return imports, resources, nil
}

func (g *generator) generateAPIKeys() ([]string, []string, error) {
	keys, err := g.client.GetAPIKeys(g.context())
	if err != nil {
		return nil, nil, err
	}

	var imports, resources []string
	for _, key := range keys {
		name := g.uniqueName("jellyfin_api_key", sanitizeName(key.AppName))
		imports = append(imports, importBlock("jellyfin_api_key", name, key.AccessToken))

		attrs := map[string]string{
			"app_name": hclString(key.AppName),
		}
		resources = append(resources, resourceBlock("jellyfin_api_key", name, attrs))
	}

	return imports, resources, nil
}

func (g *generator) generatePluginRepositories() ([]string, []string, error) {
	repos, err := g.client.GetPluginRepositories(g.context())
	if err != nil {
		return nil, nil, err
	}

	named := map[string]int{}
	for _, repo := range repos {
		named[repo.Name]++
	}

	var imports, resources []string
	for _, repo := range repos {
		// jellyfin_plugin_repository imports by name, so it cannot tell such
		// repositories apart.
		if named[repo.Name] > 1 {
			g.warnf("skipping plugin repository %q at %s: %d repositories have that name, and jellyfin_plugin_repository imports by name; rename them to import them", repo.Name, repo.URL, named[repo.Name])
			continue
		}
		name := g.uniqueName("jellyfin_plugin_repository", sanitizeName(repo.Name))
		imports = append(imports, importBlock("jellyfin_plugin_repository", name, repo.Name))

		attrs := map[string]string{
			"name":    hclString(repo.Name),
			"url":     hclString(repo.URL),
			"enabled": fmt.Sprintf("%t", repo.Enabled),
		}
		resources = append(resources, resourceBlock("jellyfin_plugin_repository", name, attrs))
	}

	return imports, resources, nil
}

func (g *generator) generatePlugins() ([]string, []string, error) {
	listed, err := g.client.GetInstalledPlugins(g.context())
	if err != nil {
		return nil, nil, err
	}
	plugins := provider.ImportablePlugins(listed)

	// Try to resolve repository URLs from available packages.
	repoURLs := g.resolvePluginRepoURLs(plugins)

	var imports, resources []string
	for _, plugin := range plugins {
		name := g.uniqueName("jellyfin_plugin", sanitizeName(plugin.Name))
		imports = append(imports, importBlock("jellyfin_plugin", name, plugin.ID))

		attrs := map[string]string{
			"name":    hclString(plugin.Name),
			"version": hclString(plugin.Version),
		}
		// Left out when unresolved: the attribute rejects an empty string,
		// and the imported state holds null.
		if repoURL := repoURLs[plugin.ID]; repoURL != "" {
			attrs["repository_url"] = hclString(repoURL)
		}
		resources = append(resources, resourceBlock("jellyfin_plugin", name, attrs))
	}

	return imports, resources, nil
}

// resolvePluginRepoURLs finds each installed plugin's repository URL the way
// jellyfin_plugin's Read does, so that repository_url matches the imported
// state: only the first package with the plugin's name counts, and only its
// entry for the installed version. Any other URL would plan a replacement.
func (g *generator) resolvePluginRepoURLs(plugins []client.InstalledPlugin) map[string]string {
	result := make(map[string]string)
	packages, err := g.client.GetAvailablePackages(g.context())
	if err != nil {
		return result
	}

	for _, p := range plugins {
		for _, pkg := range packages {
			if pkg.Name != p.Name {
				continue
			}
			for _, v := range pkg.Versions {
				if v.Version == p.Version && v.RepositoryURL != "" {
					result[p.ID] = v.RepositoryURL
					break
				}
			}
			break
		}
	}

	return result
}

func (g *generator) generateScheduledTasks() ([]string, []string, error) {
	tasks, err := g.client.GetScheduledTasks(g.context())
	if err != nil {
		return nil, nil, err
	}

	tasksWithKey := map[string]int{}
	for _, task := range tasks {
		tasksWithKey[task.Key]++
	}

	var imports, resources []string
	for _, task := range tasks {
		if task.IsHidden {
			continue
		}

		// A key selects the task only when it is set and no other task has it.
		ref, byKey := task.ID, task.Key != "" && tasksWithKey[task.Key] == 1
		if byKey {
			ref = task.Key
		}
		name := g.uniqueName("jellyfin_scheduled_task", sanitizeName(task.Name))
		imports = append(imports, importBlock("jellyfin_scheduled_task", name, ref))

		raw, err := json.Marshal(task)
		if err != nil {
			return nil, nil, fmt.Errorf("encoding task %s: %w", task.ID, err)
		}
		// Every trigger attribute the server returns is written out, because
		// the resource removes unset trigger attributes from the server on
		// apply.
		attrs, err := importedAttributes(g.context(), g.client, "jellyfin_scheduled_task", ref, string(raw))
		if err != nil {
			return nil, nil, fmt.Errorf("formatting task %s: %w", task.ID, err)
		}
		if !byKey {
			delete(attrs, "key")
			attrs["task_id"] = hclString(task.ID)
		}
		resources = append(resources, resourceBlock("jellyfin_scheduled_task", name, attrs))
	}

	return imports, resources, nil
}

func (g *generator) generateSingletonConfigs() ([]string, []string, error) {
	ctx := g.context()
	singletons := []struct {
		name string
		read func() (string, error)
	}{
		{"system", func() (string, error) {
			c, err := g.client.GetSystemConfiguration(ctx)
			if err != nil {
				return "", err
			}
			return c.RawJSON, nil
		}},
		{"encoding", func() (string, error) {
			c, err := g.client.GetEncodingOptions(ctx)
			if err != nil {
				return "", err
			}
			return c.RawJSON, nil
		}},
		{"networking", func() (string, error) {
			c, err := g.client.GetNetworkConfiguration(ctx)
			if err != nil {
				return "", err
			}
			return c.RawJSON, nil
		}},
		{"branding", func() (string, error) {
			c, err := g.client.GetBrandingConfiguration(ctx)
			if err != nil {
				return "", err
			}
			return c.RawJSON, nil
		}},
		{"livetv", func() (string, error) {
			c, err := g.client.GetLiveTVConfiguration(ctx)
			if err != nil {
				return "", err
			}
			return c.RawJSON, nil
		}},
		{"metadata", func() (string, error) {
			c, err := g.client.GetMetadataConfiguration(ctx)
			if err != nil {
				return "", err
			}
			return c.RawJSON, nil
		}},
	}

	var imports, resources []string
	for _, s := range singletons {
		raw, err := s.read()
		if err != nil {
			return nil, nil, fmt.Errorf("getting %s configuration: %w", s.name, err)
		}
		resourceType := "jellyfin_" + s.name + "_configuration"
		attrs, err := importedAttributes(ctx, g.client, resourceType, s.name, raw)
		if err != nil {
			return nil, nil, fmt.Errorf("formatting %s configuration: %w", s.name, err)
		}
		imports = append(imports, importBlock(resourceType, "this", s.name))
		resources = append(resources, resourceBlock(resourceType, "this", attrs))
	}

	return imports, resources, nil
}

func (g *generator) writeFile(name, content string) error {
	p := filepath.Join(g.outputDir, name)
	return os.WriteFile(p, []byte(content+"\n"), 0o600)
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
