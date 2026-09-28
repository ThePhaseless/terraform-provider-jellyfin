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
	"strconv"
	"strings"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/provider"
)

func main() {
	// The environment fills in a flag only after parsing: as a flag default,
	// the usage text that -h or a mistyped flag prints would show the secrets.
	endpoint := flag.String("endpoint", "", "Jellyfin server URL (or JELLYFIN_ENDPOINT env)")
	apiKey := flag.String("api-key", "", "Jellyfin API key (or JELLYFIN_API_KEY env)")
	username := flag.String("username", "", "Jellyfin username (or JELLYFIN_USERNAME env)")
	password := flag.String("password", "", "Jellyfin password (or JELLYFIN_PASSWORD env)")
	outputDir := flag.String("output", ".", "Output directory for generated Terraform files")
	flag.Parse()
	fromEnv(flag.CommandLine, "endpoint", "JELLYFIN_ENDPOINT")
	fromEnv(flag.CommandLine, "api-key", "JELLYFIN_API_KEY")
	fromEnv(flag.CommandLine, "username", "JELLYFIN_USERNAME")
	fromEnv(flag.CommandLine, "password", "JELLYFIN_PASSWORD")

	if *endpoint == "" {
		fmt.Fprintln(os.Stderr, "Error: --endpoint or JELLYFIN_ENDPOINT is required")
		os.Exit(1)
	}
	if *apiKey == "" && (*username == "" || *password == "") {
		fmt.Fprintln(os.Stderr, "Error: Either --api-key (or JELLYFIN_API_KEY) or both --username/--password (or JELLYFIN_USERNAME/JELLYFIN_PASSWORD) must be set")
		os.Exit(1)
	}

	ctx := context.Background()
	c, err := importClient(ctx, *endpoint, *apiKey, *username, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error configuring Jellyfin client: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	g := &generator{client: c, outputDir: *outputDir}
	if err := g.Generate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Import files generated successfully in", *outputDir)
}

// fromEnv sets the flag name from the environment variable env when the
// command line leaves it out, as a default would, so a flag set to empty, such
// as -api-key= to sign in with a password instead, stays empty.
func fromEnv(fs *flag.FlagSet, name, env string) {
	value, ok := os.LookupEnv(env)
	if !ok {
		return
	}
	explicit := false
	fs.Visit(func(f *flag.Flag) { explicit = explicit || f.Name == name })
	if !explicit {
		// A string flag takes any value.
		_ = fs.Set(name, value)
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
	outputDir string
	usedNames map[string]bool // resource addresses already handed out
	warnings  io.Writer
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
func (g *generator) uniqueName(resourceType, baseName string) string {
	if g.usedNames == nil {
		g.usedNames = make(map[string]bool)
	}
	name := baseName
	for i := 1; g.usedNames[resourceType+"."+name]; i++ {
		name = fmt.Sprintf("%s_%d", baseName, i)
	}
	g.usedNames[resourceType+"."+name] = true
	return name
}

// Generate generates all Terraform files.
func (g *generator) Generate(ctx context.Context) error {
	sections := []struct {
		name          string
		generate      func(context.Context) ([]string, []string, error)
		importsTokens bool
	}{
		{"users", g.generateUsers, false},
		{"libraries", g.generateLibraries, false},
		{"API keys", g.generateAPIKeys, true},
		{"plugin repositories", g.generatePluginRepositories, false},
		{"plugins", g.generatePlugins, false},
		{"scheduled tasks", g.generateScheduledTasks, false},
		{"configurations", g.generateSingletonConfigs, false},
	}

	var imports, resources []string
	tokensImported := false
	for _, s := range sections {
		sectionImports, sectionResources, err := s.generate(ctx)
		if err != nil {
			return fmt.Errorf("generating %s: %w", s.name, err)
		}
		tokensImported = tokensImported || s.importsTokens && len(sectionImports) > 0
		imports = append(imports, sectionImports...)
		resources = append(resources, sectionResources...)
	}

	if len(imports) > 0 {
		if err := g.writeFile("imports.tf", strings.Join(imports, "\n")); err != nil {
			return fmt.Errorf("writing imports.tf: %w", err)
		}
		if tokensImported {
			g.warnf("imports.tf holds the access token of each API key as its import ID; keep it out of version control, and remove those import blocks once terraform apply has imported the keys")
		}
	}

	if len(resources) > 0 {
		content := terraformBlock + "\n" + strings.Join(resources, "\n")
		if err := g.writeFile("resources.tf", content); err != nil {
			return fmt.Errorf("writing resources.tf: %w", err)
		}
		g.warnIfOtherConfiguration()
	}

	return nil
}

const providerSource = "ThePhaseless/jellyfin"

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

func (g *generator) generateUsers(ctx context.Context) ([]string, []string, error) {
	users, err := g.client.GetUsers(ctx)
	if err != nil {
		return nil, nil, err
	}

	var imports, resources []string
	for _, user := range users {
		name := g.uniqueName("jellyfin_user", sanitizeName(user.Name))
		imports = append(imports, importBlock("jellyfin_user", name, user.ID))

		attrs := map[string]string{
			"name":               hclString(user.Name),
			"is_administrator":   strconv.FormatBool(user.Policy.IsAdministrator),
			"is_disabled":        strconv.FormatBool(user.Policy.IsDisabled),
			"enable_all_folders": strconv.FormatBool(user.Policy.EnableAllFolders),
		}
		resources = append(resources, resourceBlock("jellyfin_user", name, attrs))
	}

	return imports, resources, nil
}

func (g *generator) generateLibraries(ctx context.Context) ([]string, []string, error) {
	folders, err := g.client.GetVirtualFolders(ctx)
	if err != nil {
		return nil, nil, err
	}

	var imports, resources []string
	for _, folder := range folders {
		collectionType, accepted := provider.LibraryCollectionType(folder.CollectionType)
		if !accepted {
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

func (g *generator) generateAPIKeys(ctx context.Context) ([]string, []string, error) {
	keys, err := g.client.GetAPIKeys(ctx)
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

func (g *generator) generatePluginRepositories(ctx context.Context) ([]string, []string, error) {
	repos, err := g.client.GetPluginRepositories(ctx)
	if err != nil {
		return nil, nil, err
	}

	named := map[string]int{}
	for _, repo := range repos {
		named[repo.Name]++
	}

	var imports, resources []string
	for _, repo := range repos {
		if named[repo.Name] > 1 {
			g.warnf("skipping plugin repository %q at %s: %d repositories have that name, and jellyfin_plugin_repository imports by name; rename them to import them", repo.Name, repo.URL, named[repo.Name])
			continue
		}
		name := g.uniqueName("jellyfin_plugin_repository", sanitizeName(repo.Name))
		imports = append(imports, importBlock("jellyfin_plugin_repository", name, repo.Name))

		attrs := map[string]string{
			"name":    hclString(repo.Name),
			"url":     hclString(repo.URL),
			"enabled": strconv.FormatBool(repo.Enabled),
		}
		resources = append(resources, resourceBlock("jellyfin_plugin_repository", name, attrs))
	}

	return imports, resources, nil
}

func (g *generator) generatePlugins(ctx context.Context) ([]string, []string, error) {
	listed, err := g.client.GetInstalledPlugins(ctx)
	if err != nil {
		return nil, nil, err
	}
	plugins := provider.ImportablePlugins(listed)

	repoURLs := g.resolvePluginRepoURLs(ctx, plugins)

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
func (g *generator) resolvePluginRepoURLs(ctx context.Context, plugins []client.InstalledPlugin) map[string]string {
	result := make(map[string]string)
	packages, err := g.client.GetAvailablePackages(ctx)
	if err != nil {
		return result
	}
	for _, p := range plugins {
		if repositoryURL := provider.PluginRepositoryURL(packages, p.Name, p.Version); repositoryURL != "" {
			result[p.ID] = repositoryURL
		}
	}
	return result
}

func (g *generator) generateScheduledTasks(ctx context.Context) ([]string, []string, error) {
	tasks, err := g.client.GetScheduledTasks(ctx)
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

		ref, keySelectsTask := task.ID, task.Key != "" && tasksWithKey[task.Key] == 1
		if keySelectsTask {
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
		attrs, err := importedAttributes(ctx, g.client, "jellyfin_scheduled_task", ref, string(raw))
		if err != nil {
			return nil, nil, fmt.Errorf("formatting task %s: %w", task.ID, err)
		}
		if !keySelectsTask {
			delete(attrs, "key")
			attrs["task_id"] = hclString(task.ID)
		}
		resources = append(resources, resourceBlock("jellyfin_scheduled_task", name, attrs))
	}

	return imports, resources, nil
}

func (g *generator) generateSingletonConfigs(ctx context.Context) ([]string, []string, error) {
	singletons := []struct {
		name string
		read func(context.Context) (string, error)
	}{
		{"system", g.client.GetSystemConfiguration},
		{"encoding", g.client.GetEncodingOptions},
		{"networking", g.client.GetNetworkConfiguration},
		{"branding", g.client.GetBrandingConfiguration},
		{"livetv", g.client.GetLiveTVConfiguration},
		{"metadata", g.client.GetMetadataConfiguration},
	}

	var imports, resources []string
	for _, s := range singletons {
		raw, err := s.read(ctx)
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

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// sanitizeName converts a human-readable name to a valid Terraform identifier.
func sanitizeName(name string) string {
	result := sanitizeRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "_")
	result = strings.Trim(result, "_")
	if result == "" {
		result = "unnamed"
	}
	startsWithDigit := result[0] >= '0' && result[0] <= '9'
	if startsWithDigit {
		result = "r_" + result
	}
	return result
}
