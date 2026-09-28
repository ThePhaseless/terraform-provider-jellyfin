// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

const (
	jellyfinAPISchemaGolden      = "../wire/schema/jellyfin_api_schema.golden"
	jellyfinAPISchemaFloorGolden = "../wire/schema/jellyfin_api_schema_floor.golden"
	securityPluginPayloadGolden  = "../wire/schema/security_plugin_config_schema.golden"
	supportedJellyfinEnvFile     = "internal/provider/supported_jellyfin_version.env"
	floorJellyfinEnvFile         = "internal/wire/schema/floor.env"
)

const (
	clientSourceDir        = "../client"
	namedConfigurationPath = "/System/Configuration/{key}"
)

// schemaGuard is what a mismatch against one golden file has to tell the
// maintainer: what the lines record, which acceptance test regenerates them
// and with what environment, and what has to change along with them.
type schemaGuard struct {
	golden string
	test   string
	// envFile is the docker compose env file that picks the Jellyfin release
	// the golden records.
	envFile string
	// ciStep is empty for a golden CI does not check.
	ciStep   string
	env      string
	records  string
	followUp string
}

var jellyfinAPISchemaGuard = schemaGuard{
	golden:  jellyfinAPISchemaGolden,
	test:    "TestAccJellyfinAPISchemaGuard",
	envFile: supportedJellyfinEnvFile,
	ciStep:  "Run acceptance tests",
	records: `Each line is an endpoint internal/client calls ("op", or "undocumented" when
the server's OpenAPI document does not describe it) or a property of a schema
those endpoints send or return ("schema"), as that document describes it. The
golden changes when a Jellyfin release changes one of those, or when
internal/client starts or stops calling an endpoint.`,
	followUp: `The provider reads this golden at run time, through internal/wire, for the JSON
key of each attribute, so a changed line can change what it sends although the
test passes again. Run TestUnitWireBindings, review the diff of
internal/provider/testdata/wire_bindings.golden it reports, and fix the
affected resources in the same change.`,
}

var jellyfinAPISchemaFloorGuard = schemaGuard{
	golden:  jellyfinAPISchemaFloorGolden,
	test:    "TestAccJellyfinAPISchemaFloorGuard",
	envFile: floorJellyfinEnvFile,
	env:     "SCHEMA_GUARD_FLOOR=1",
	records: `The lines of jellyfin_api_schema.golden, reduced the same way from the OpenAPI
document of the older Jellyfin release that ` + floorJellyfinEnvFile + `
names. A field the pinned golden has and this one lacks needs the release
floor.env names as NEXT_JELLYFIN_VERSION, so internal/wire rejects it on older
servers. CI runs only the supported release and does not check this golden;
the test runs only with SCHEMA_GUARD_FLOOR=1, against a server of the floor
release.`,
	followUp: `A property the diff adds or removes moves the Jellyfin version its attribute
needs: run TestUnitWireBindings and review the since= changes in
internal/provider/testdata/wire_bindings.golden.`,
}

var securityPluginPayloadGuard = schemaGuard{
	golden:  securityPluginPayloadGolden,
	test:    "TestAccSecurityPluginConfigSchemaGuard",
	envFile: supportedJellyfinEnvFile,
	ciStep:  "Run restart acceptance tests (isolated)",
	env:     "JELLYFIN_RESTART_ACC=1",
	records: `Each line is a key of the configuration that the JellyfinSecurity build pinned
in internal/provider/supported_security_plugin_version.env serves, with its JSON
type. The test first writes ` + strconv.Quote(payloadListPlaceholder) + ` into every list the plugin serves empty, so a
list is typed by the element the plugin serves back, not by whether its default
is empty. That relies on the plugin keeping the entry; when it rejects or
drops it, the test fails saying so, and an entry the plugin accepts for that
list in the probe in testAccSecurityPluginPayloadShape fixes it. The golden
changes when a new pin adds, removes or retypes a key. The test installs the
plugin and restarts the server to load it, which it does only with
JELLYFIN_RESTART_ACC=1, so CI runs it in a step of its own.`,
	followUp: `jellyfin_security_plugin_configuration reads this golden at run time, through
internal/wire, for the JSON key of each attribute, so a changed line can change
what it sends although the test passes again. Run TestUnitWireBindings, review
the diff of internal/provider/testdata/wire_bindings.golden it reports, where a
key no attribute claims shows as kept, and fix the resource in the same change.
TestUnitJellyfinSecurityWriteKeepsTheServedShape fails while a rebuilt OIDC
provider, user email or role mapping drops a key the golden lists.`,
}

var namedConfigurationSchemas = map[string]string{
	"branding": "BrandingOptionsDto",
	"encoding": "EncodingOptions",
	"livetv":   "LiveTvOptions",
	"metadata": "MetadataConfiguration",
	"network":  "NetworkConfiguration",
}

var unexpandedSchemas = map[string]bool{
	"SessionInfoDto": true,
}

// formatVerb matches a whole fmt verb, with any flags, argument index, width
// and precision, so "%[1]s" and "%5d" each print one argument.
var formatVerb = regexp.MustCompile(`%[-+# 0]*(?:\[\d+\])?(?:\*|\d+)?(?:\.(?:\[\d+\])?(?:\*|\d+)?)?(?:\[\d+\])?[a-zA-Z%]`)

var formatArgIndex = regexp.MustCompile(`\[\d+\]`)

// A resolved request path writes a runtime value as "{}" and, when the value
// is computed from parameters of the enclosing function, lists their indexes
// inside it, as in "{#1}". That tells a parameter passed on as the whole path
// apart from one built into a path, and a path segment from a query value.
var (
	pathPlaceholder = regexp.MustCompile(`\{((?:#\d+)*)\}`)
	wholeParamPath  = regexp.MustCompile(`^\{#(\d+)\}$`)
)

type apiCall struct {
	method string
	path   string
}

type openAPIContent map[string]struct {
	Schema map[string]json.RawMessage `json:"schema"`
}

type openAPIOperation struct {
	Parameters  []map[string]json.RawMessage `json:"parameters"`
	RequestBody struct {
		Content openAPIContent `json:"content"`
	} `json:"requestBody"`
	Responses map[string]struct {
		Content openAPIContent `json:"content"`
	} `json:"responses"`
}

func TestAccJellyfinAPISchemaGuard(t *testing.T) {
	testAccPreCheck(t)
	checkSchemaGolden(t, jellyfinAPISchemaGuard, testAccReducedAPISchema(t, testAccClient(t)))
}

func TestAccJellyfinAPISchemaFloorGuard(t *testing.T) {
	if os.Getenv("SCHEMA_GUARD_FLOOR") != "1" {
		t.Skip("set SCHEMA_GUARD_FLOOR=1 to check the floor golden against a server of the release " + floorJellyfinEnvFile + " names")
	}
	testAccPreCheck(t)
	c := testAccClient(t)

	info, err := c.GetPublicSystemInfo(context.Background())
	if err != nil {
		t.Fatalf("reading the Jellyfin version: %v", err)
	}
	if info.Version != wire.FloorVersion() {
		t.Fatalf("the server runs Jellyfin %s, but %s names %s, the release the floor golden records\n\n%s", info.Version, floorJellyfinEnvFile, wire.FloorVersion(), jellyfinAPISchemaFloorGuard.regenerateHelp())
	}
	checkSchemaGolden(t, jellyfinAPISchemaFloorGuard, testAccReducedAPISchema(t, c))
}

func testAccReducedAPISchema(t *testing.T, c *client.Client) []string {
	t.Helper()
	spec, err := c.GetOpenAPISpec(context.Background())
	if err != nil {
		t.Fatalf("getting OpenAPI spec: %v", err)
	}

	calls, err := clientAPICalls(clientSourceDir)
	if err != nil {
		t.Fatalf("collecting client API calls: %v", err)
	}

	lines, err := reduceOpenAPISpec(spec, calls)
	if err != nil {
		t.Fatalf("reducing OpenAPI spec: %v", err)
	}
	return lines
}

// reduceOpenAPISpec keeps only the operations the client calls and the schemas
// they reach.
func reduceOpenAPISpec(spec string, calls []apiCall) ([]string, error) {
	var doc struct {
		Paths      map[string]map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(spec), &doc); err != nil {
		return nil, fmt.Errorf("parsing OpenAPI spec: %w", err)
	}

	var out []string
	roots := map[string]bool{}
	for _, call := range calls {
		tmpl := matchOperation(doc.Paths, call)
		if tmpl == "" {
			out = append(out, fmt.Sprintf("undocumented %s %s", call.method, call.path))
			continue
		}

		if tmpl == namedConfigurationPath {
			key := strings.ToLower(call.path[strings.LastIndex(call.path, "/")+1:])
			name, ok := namedConfigurationSchemas[key]
			if !ok {
				return nil, fmt.Errorf("%s %s: add the schema of named configuration %q to namedConfigurationSchemas", call.method, call.path, key)
			}
			roots[name] = true
		}

		line, err := operationLine(call.method, tmpl, doc.Paths[tmpl][strings.ToLower(call.method)], roots)
		if err != nil {
			return nil, fmt.Errorf("operation %s %s: %w", call.method, tmpl, err)
		}
		out = append(out, line)
	}

	schemaLines, err := schemaClosure(doc.Components.Schemas, roots)
	if err != nil {
		return nil, err
	}
	out = append(out, schemaLines...)

	sort.Strings(out)
	return dedupStrings(out), nil
}

// matchOperation follows ASP.NET Core routing, which serves Jellyfin: literal
// segments match case-insensitively and, read left to right, outrank
// parameters.
func matchOperation(paths map[string]map[string]json.RawMessage, call apiCall) string {
	want := strings.Split(call.path, "/")
	var best, bestRank string
	for tmpl, ops := range paths {
		if _, ok := ops[strings.ToLower(call.method)]; !ok {
			continue
		}
		segs := strings.Split(tmpl, "/")
		if len(segs) != len(want) {
			continue
		}

		var rank strings.Builder
		matched := true
		for i, seg := range segs {
			if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
				rank.WriteByte('P')
				continue
			}
			if !strings.EqualFold(seg, want[i]) {
				matched = false
				break
			}
			rank.WriteByte('L')
		}
		if !matched {
			continue
		}

		r := rank.String()
		if best == "" || r < bestRank || (r == bestRank && tmpl < best) {
			best, bestRank = tmpl, r
		}
	}
	return best
}

// goldenClientDrift lists where the golden and the client disagree on the
// endpoints called, so a client change that needs a new golden fails without a
// server. Matching against the golden's own templates, it misses a new call
// that an existing, less specific template also matches; the acceptance test
// still catches that one.
func goldenClientDrift(golden []string, calls []apiCall) []string {
	paths := map[string]map[string]json.RawMessage{}
	var listed []string
	for _, line := range golden {
		fields := strings.Fields(line)
		if len(fields) < 3 || (fields[0] != "op" && fields[0] != "undocumented") {
			continue
		}
		listed = append(listed, strings.Join(fields[:3], " "))
		if fields[0] == "op" {
			if paths[fields[2]] == nil {
				paths[fields[2]] = map[string]json.RawMessage{}
			}
			paths[fields[2]][strings.ToLower(fields[1])] = nil
		}
	}

	var drift []string
	used := map[string]bool{}
	for _, call := range calls {
		entry := "undocumented " + call.method + " " + call.path
		if !slices.Contains(listed, entry) {
			tmpl := matchOperation(paths, call)
			if tmpl == "" {
				drift = append(drift, fmt.Sprintf("  + internal/client calls %s %s, which the golden does not list", call.method, call.path))
				continue
			}
			entry = "op " + call.method + " " + tmpl
			key := strings.ToLower(call.path[strings.LastIndex(call.path, "/")+1:])
			if name, ok := namedConfigurationSchemas[key]; ok && tmpl == namedConfigurationPath && !goldenHasSchema(golden, name) {
				drift = append(drift, fmt.Sprintf("  + internal/client uses named configuration %q, whose schema %s the golden does not list", key, name))
			}
		}
		used[entry] = true
	}
	for _, entry := range listed {
		if !used[entry] {
			drift = append(drift, fmt.Sprintf("  - the golden lists %s, which internal/client no longer calls", entry))
		}
	}
	return dedupStrings(drift)
}

func goldenHasSchema(golden []string, name string) bool {
	return slices.ContainsFunc(golden, func(line string) bool {
		return strings.HasPrefix(line, "schema "+name+".") || strings.HasPrefix(line, "schema "+name+":")
	})
}

func operationLine(method, tmpl string, rawOp json.RawMessage, refs map[string]bool) (string, error) {
	var op openAPIOperation
	if err := json.Unmarshal(rawOp, &op); err != nil {
		return "", err
	}

	var paramParts []string
	for _, param := range op.Parameters {
		name := jsonString(param, "name")
		in := jsonString(param, "in")
		required := jsonBool(param, "required")
		paramParts = append(paramParts, fmt.Sprintf("%s:%s:required=%t", name, in, required))
	}
	sort.Strings(paramParts)

	line := fmt.Sprintf("op %s %s", method, tmpl)
	if len(paramParts) > 0 {
		line += " | params=" + strings.Join(paramParts, ",")
	}

	if body, ok := op.RequestBody.Content["application/json"]; ok {
		sig, err := typeSignature(body.Schema, refs)
		if err != nil {
			return "", err
		}
		line += " | body=" + sig
	}

	var codes []string
	for code := range op.Responses {
		if strings.HasPrefix(code, "2") {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	for _, code := range codes {
		resp, ok := op.Responses[code].Content["application/json"]
		if !ok {
			continue
		}
		sig, err := typeSignature(resp.Schema, refs)
		if err != nil {
			return "", err
		}
		line += " | resp=" + sig
		break
	}

	return line, nil
}

func schemaClosure(schemas map[string]json.RawMessage, roots map[string]bool) ([]string, error) {
	var out []string
	var queue []string
	for name := range roots {
		queue = append(queue, name)
	}

	seen := map[string]bool{}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if seen[name] || unexpandedSchemas[name] {
			continue
		}
		seen[name] = true

		raw, ok := schemas[name]
		if !ok {
			out = append(out, fmt.Sprintf("schema %s: <missing>", name))
			continue
		}

		var s map[string]json.RawMessage
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("parsing schema %s: %w", name, err)
		}
		refs := map[string]bool{}
		lines, err := schemaLines(name, s, refs)
		if err != nil {
			return nil, fmt.Errorf("signature for %s: %w", name, err)
		}
		out = append(out, lines...)

		for ref := range refs {
			queue = append(queue, ref)
		}
	}
	return out, nil
}

// schemaLines puts each property of an object schema on a line of its own.
func schemaLines(name string, s map[string]json.RawMessage, refs map[string]bool) ([]string, error) {
	rawProps, ok := s["properties"]
	if !ok || jsonString(s, "type") != "object" {
		sig, err := typeSignature(s, refs)
		if err != nil {
			return nil, err
		}
		return []string{fmt.Sprintf("schema %s: %s", name, sig)}, nil
	}

	var props map[string]json.RawMessage
	if err := json.Unmarshal(rawProps, &props); err != nil {
		return nil, err
	}
	var out []string
	for prop, raw := range props {
		var p map[string]json.RawMessage
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		sig, err := typeSignature(p, refs)
		if err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("schema %s.%s: %s", name, prop, sig))
	}

	rest := maps.Clone(s)
	delete(rest, "properties")
	sig, err := typeSignature(rest, refs)
	if err != nil {
		return nil, err
	}
	if len(props) == 0 || sig != "{object}" {
		out = append(out, fmt.Sprintf("schema %s: %s", name, sig))
	}
	return out, nil
}

// typeSignature renders s with each $ref as "#Name" and records the name in
// refs; the referenced schema gets its own line, so a change shows up once.
func typeSignature(s map[string]json.RawMessage, refs map[string]bool) (string, error) {
	if ref := jsonString(s, "$ref"); ref != "" {
		name := ref[strings.LastIndex(ref, "/")+1:]
		refs[name] = true
		return "#" + name, nil
	}

	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		raw, ok := s[key]
		if !ok {
			continue
		}
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return "", err
		}
		// Swashbuckle wraps a lone $ref in allOf only to attach a description
		// or nullable flag, so the wrapper carries no shape of its own.
		if key == "allOf" && len(items) == 1 {
			return typeSignature(items[0], refs)
		}
		parts := []string{key}
		for _, item := range items {
			sig, err := typeSignature(item, refs)
			if err != nil {
				return "", err
			}
			parts = append(parts, sig)
		}
		return strings.Join(parts, ","), nil
	}

	typ := jsonString(s, "type")
	format := jsonString(s, "format")

	switch typ {
	case "object":
		parts := []string{"object"}
		if rawProps, ok := s["properties"]; ok {
			var props map[string]json.RawMessage
			if err := json.Unmarshal(rawProps, &props); err != nil {
				return "", err
			}
			var keys []string
			for k := range props {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				var prop map[string]json.RawMessage
				if err := json.Unmarshal(props[k], &prop); err != nil {
					return "", err
				}
				sig, err := typeSignature(prop, refs)
				if err != nil {
					return "", err
				}
				parts = append(parts, fmt.Sprintf("%s=%s", k, sig))
			}
		}
		if rawAdd, ok := s["additionalProperties"]; ok {
			var add map[string]json.RawMessage
			if err := json.Unmarshal(rawAdd, &add); err == nil {
				sig, err := typeSignature(add, refs)
				if err != nil {
					return "", err
				}
				parts = append(parts, "map="+sig)
			}
		}
		return "{" + strings.Join(parts, ",") + "}", nil
	case "array":
		if rawItems, ok := s["items"]; ok {
			var items map[string]json.RawMessage
			if err := json.Unmarshal(rawItems, &items); err != nil {
				return "", err
			}
			sig, err := typeSignature(items, refs)
			if err != nil {
				return "", err
			}
			return "[]" + sig, nil
		}
		return "[]any", nil
	case "string", "integer", "number":
		sig := typ
		if format != "" {
			sig += ":" + format
		}
		if rawEnum, ok := s["enum"]; ok {
			var values []any
			if err := json.Unmarshal(rawEnum, &values); err != nil {
				return "", err
			}
			var names []string
			for _, v := range values {
				names = append(names, fmt.Sprint(v))
			}
			sort.Strings(names)
			sig += " enum(" + strings.Join(names, "|") + ")"
		}
		return sig, nil
	case "boolean":
		return "boolean", nil
	case "":
		if jsonBool(s, "nullable") {
			return "null", nil
		}
		return "any", nil
	default:
		return typ, nil
	}
}

// clientAPICalls lists the requests internal/client makes, read from its
// source.
func clientAPICalls(dir string) ([]apiCall, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}

	calls, err := packageAPICalls(fset, files)
	if err != nil {
		return nil, err
	}
	if len(calls) == 0 {
		return nil, fmt.Errorf("no API calls found in %s", dir)
	}
	return calls, nil
}

// packageAPICalls finds request helpers instead of listing them: a function
// that passes a parameter on as the whole request path becomes a request site
// for its callers, repeated until none turns up. Requests start where they
// enter net/http through netHTTPRequestSites. A parameter built into a larger
// path, including every argument of a variadic one, is recorded as a runtime
// value. The guard fails instead of recording the wrong endpoint or none when
// a call in the package passes path text for such a parameter, when a request
// site is used other than by calling it from a function declaration or is
// called as a method expression, and when a method named like a request site
// is called through an interface or on a value whose type comes from an
// import.
//
// Three cases still record the wrong endpoint or none. Only this package is
// read, so path text that a caller elsewhere passes to an exported method,
// such as "Counts" for an item ID, selects a route the guard does not see. A
// path operand is read as text only when it is a constant or a local variable
// whose assignments it can read; anything else, such as a call result, a
// field, an index expression or a package variable, is recorded as a runtime
// value even when it always holds the same text. So is a constant format
// argument that fmt may not print as its value: one of a named type, which
// may have a String method, or one whose verb takes a width or precision
// from another argument. And a request that enters net/http any other way,
// such as through an aliased import of net/http or an http.Request built by
// hand, is not seen at all.
func packageAPICalls(fset *token.FileSet, files []*ast.File) ([]apiCall, error) {
	info := &types.Info{
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
		Types: map[ast.Expr]types.TypeAndValue{},
	}
	// Only the package's own names, methods and constants are needed, so imports
	// stay unresolved and the type errors that causes are ignored.
	conf := types.Config{Error: func(error) {}}
	_, _ = conf.Check("client", fset, files, info)

	var funcs []*ast.FuncDecl
	for _, f := range files {
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				funcs = append(funcs, fn)
			}
		}
	}

	f := &requestFinder{fset: fset, info: info, sites: map[types.Object][]requestSite{}, embeds: map[types.Object][]int{}}
	for {
		var calls []apiCall
		f.reached = map[types.Object]bool{}
		changed := false
		for _, fn := range funcs {
			fnCalls, fnSites, fnEmbeds, err := f.requestsIn(fn)
			if err != nil {
				return nil, err
			}
			calls = append(calls, fnCalls...)
			key := info.Defs[fn.Name]
			for _, site := range fnSites {
				if !slices.Contains(f.sites[key], site) {
					f.sites[key] = append(f.sites[key], site)
					changed = true
				}
			}
			for _, i := range fnEmbeds {
				if !slices.Contains(f.embeds[key], i) {
					f.embeds[key] = append(f.embeds[key], i)
					changed = true
				}
			}
		}
		if changed {
			continue
		}

		for _, file := range files {
			for _, decl := range file.Decls {
				if err := f.siteValues(decl); err != nil {
					return nil, err
				}
			}
		}
		for _, fn := range funcs {
			if key := info.Defs[fn.Name]; len(f.sites[key]) > 0 && !f.reached[key] {
				return nil, fmt.Errorf("%s: %s passes a parameter on as a request path, but nothing in its package calls it, so the guard cannot see the endpoints it requests", fset.Position(fn.Pos()), fn.Name.Name)
			}
		}
		return calls, nil
	}
}

// requestSite describes a function that sends a request: the method is fixed
// or read from argument methodArg, and the path is read from argument pathArg.
type requestSite struct {
	method    string
	methodArg int
	pathArg   int
}

// netHTTPRequestSites are where a request enters net/http, as a package
// function or as a method of the client's *http.Client.
var netHTTPRequestSites = map[string]requestSite{
	"NewRequest":            {methodArg: 0, pathArg: 1},
	"NewRequestWithContext": {methodArg: 1, pathArg: 2},
	"Get":                   {method: http.MethodGet, pathArg: 0},
	"Head":                  {method: http.MethodHead, pathArg: 0},
	"Post":                  {method: http.MethodPost, pathArg: 0},
	"PostForm":              {method: http.MethodPost, pathArg: 0},
}

// requestFinder holds, by the function's object, the request sites found so
// far and the parameters each function builds into a request path. Keying by
// object keeps a method of another type, or the builtin delete, apart from a
// request helper of the same name.
type requestFinder struct {
	fset    *token.FileSet
	info    *types.Info
	sites   map[types.Object][]requestSite
	embeds  map[types.Object][]int
	reached map[types.Object]bool
}

func (f *requestFinder) callSites(call *ast.CallExpr) (types.Object, []requestSite, error) {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		if isNetHTTP(fun.X) {
			if site, ok := netHTTPRequestSites[fun.Sel.Name]; ok {
				return nil, []requestSite{site}, nil
			}
			return nil, nil, nil
		}
		obj := f.info.Uses[fun.Sel]
		if len(f.sites[obj]) > 0 && f.info.Types[fun.X].IsType() {
			return nil, nil, fmt.Errorf("%s is called as a method expression, which takes the receiver as its first argument, so the guard would read the path from the wrong argument; call it on the client instead", types.ExprString(fun))
		}
		if err := f.unresolvedSite(fun); err != nil {
			return nil, nil, err
		}
		return obj, f.sites[obj], nil
	case *ast.Ident:
		obj := f.info.Uses[fun]
		return obj, f.sites[obj], nil
	}
	return nil, nil, nil
}

// unresolvedSite rejects a method named like a request helper when the type
// check cannot tell which method it is: one called through an interface, or on
// a value whose type comes from an import, which the check does not load.
func (f *requestFinder) unresolvedSite(sel *ast.SelectorExpr) error {
	switch obj := f.info.Uses[sel.Sel].(type) {
	case nil:
	case *types.Func:
		if recv := obj.Signature().Recv(); recv == nil || !types.IsInterface(recv.Type()) {
			return nil
		}
	default:
		return nil
	}
	for site := range f.sites {
		if site.Name() == sel.Sel.Name {
			return fmt.Errorf("the guard cannot tell whether %s is the request helper %s, since it is called through an interface or on a value whose type comes from an import; call the request helper on the client directly", types.ExprString(sel), site.Name())
		}
	}
	return nil
}

func isNetHTTP(expr ast.Expr) bool {
	if isIdent(expr, "http") {
		return true
	}
	sel, ok := expr.(*ast.SelectorExpr)
	return ok && (sel.Sel.Name == "HTTPClient" || sel.Sel.Name == "DefaultClient")
}

// A request in fn whose path is one of fn's parameters is not an endpoint; it
// makes fn a request site for its callers. The parameters fn builds into a
// request path, itself or through a function it calls, are returned too.
func (f *requestFinder) requestsIn(fn *ast.FuncDecl) ([]apiCall, []requestSite, []int, error) {
	r := newPathResolver(fn, f.info)
	var calls []apiCall
	var sites []requestSite
	var embeds []int
	var firstErr error
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if firstErr != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		key, callee, err := f.callSites(call)
		if err != nil {
			firstErr = fmt.Errorf("%s: %w", f.fset.Position(call.Pos()), err)
			return false
		}
		if len(callee) > 0 {
			f.reached[key] = true
		}
		for _, site := range callee {
			siteCalls, forwarded, embedded, err := r.request(call, site)
			if err != nil {
				firstErr = fmt.Errorf("%s: %w", f.fset.Position(call.Pos()), err)
				return false
			}
			calls = append(calls, siteCalls...)
			sites = append(sites, forwarded...)
			embeds = append(embeds, embedded...)
		}
		for _, i := range f.embeds[key] {
			embedded, err := r.embeddedArg(call, key, i)
			if err != nil {
				firstErr = fmt.Errorf("%s: %w", f.fset.Position(call.Pos()), err)
				return false
			}
			embeds = append(embeds, embedded...)
		}
		return true
	})
	return calls, sites, embeds, firstErr
}

// siteValues rejects a request site that decl uses other than by calling it
// from a function declaration, as in f := c.get or a package variable's
// initializer, since the guard cannot see what is requested through it.
func (f *requestFinder) siteValues(decl ast.Decl) error {
	fn, inFunc := decl.(*ast.FuncDecl)
	if inFunc && fn.Body == nil {
		return nil
	}
	skip := map[ast.Node]bool{}
	ast.Inspect(decl, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.CallExpr:
			skip[e.Fun] = inFunc
		case *ast.SelectorExpr:
			skip[e.Sel] = true
		}
		return true
	})

	var err error
	ast.Inspect(decl, func(n ast.Node) bool {
		if err != nil || skip[n] {
			return err == nil
		}
		var name string
		switch e := n.(type) {
		case *ast.SelectorExpr:
			if _, ok := netHTTPRequestSites[e.Sel.Name]; (ok && isNetHTTP(e.X)) || len(f.sites[f.info.Uses[e.Sel]]) > 0 {
				name = types.ExprString(e)
			} else if unresolved := f.unresolvedSite(e); unresolved != nil {
				err = fmt.Errorf("%s: %w", f.fset.Position(n.Pos()), unresolved)
			}
		case *ast.Ident:
			if len(f.sites[f.info.Uses[e]]) > 0 {
				name = e.Name
			}
		}
		switch {
		case name != "" && inFunc:
			err = fmt.Errorf("%s: %s sends requests but is used here as a value, so the guard cannot see what it requests; call it directly", f.fset.Position(n.Pos()), name)
		case name != "":
			err = fmt.Errorf("%s: %s sends requests but is used here outside a function declaration, where the guard does not look; call it from a function", f.fset.Position(n.Pos()), name)
		}
		return err == nil
	})
	return err
}

// pathResolver's patterns use the placeholders described at pathPlaceholder.
type pathResolver struct {
	fn       *ast.FuncDecl
	info     *types.Info
	params   map[types.Object]int
	visiting map[types.Object]bool
}

var errPathCycle = errors.New("request path is built from its own value")

func newPathResolver(fn *ast.FuncDecl, info *types.Info) *pathResolver {
	params := map[types.Object]int{}
	i := 0
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			i++
			continue
		}
		for _, id := range field.Names {
			if obj := info.Defs[id]; obj != nil {
				params[obj] = i
			}
			i++
		}
	}
	return &pathResolver{fn: fn, info: info, params: params, visiting: map[types.Object]bool{}}
}

func (r *pathResolver) request(call *ast.CallExpr, site requestSite) ([]apiCall, []requestSite, []int, error) {
	if len(call.Args) <= max(site.methodArg, site.pathArg) {
		return nil, nil, nil, fmt.Errorf("request call %s has too few arguments", types.ExprString(call))
	}

	method, methodParam := site.method, -1
	if method == "" {
		arg := call.Args[site.methodArg]
		if m, ok := arg.(*ast.SelectorExpr); ok && isIdent(m.X, "http") && strings.HasPrefix(m.Sel.Name, "Method") {
			method = strings.ToUpper(strings.TrimPrefix(m.Sel.Name, "Method"))
		} else if i, ok := r.paramOf(arg); ok {
			methodParam = i
		} else {
			return nil, nil, nil, fmt.Errorf("cannot resolve request method %s", types.ExprString(arg))
		}
	}

	pathExpr := call.Args[site.pathArg]
	values, err := r.values(pathExpr)
	if err != nil {
		return nil, nil, nil, err
	}

	var calls []apiCall
	var forwarded []requestSite
	var embedded []int
	for _, v := range values {
		if m := wholeParamPath.FindStringSubmatch(v); m != nil {
			i, _ := strconv.Atoi(m[1])
			forwarded = append(forwarded, requestSite{method: method, methodArg: methodParam, pathArg: i})
			continue
		}
		if methodParam >= 0 {
			return nil, nil, nil, fmt.Errorf("request method comes from a parameter but path %s does not", types.ExprString(pathExpr))
		}
		path, _, _ := strings.Cut(v, "?")
		if !strings.HasPrefix(path, "/") {
			return nil, nil, nil, fmt.Errorf("request path %q does not start with /", untagPath(path))
		}
		embedded = append(embedded, placeholderParams(path)...)
		calls = append(calls, apiCall{method: method, path: untagPath(path)})
	}
	return calls, forwarded, embedded, nil
}

// embeddedArg checks argument i of a call to callee, which builds that
// argument into a request path, and returns the parameters of fn it is
// computed from, which fn then builds into a request path as well. For a
// variadic parameter, that is every argument from i on.
func (r *pathResolver) embeddedArg(call *ast.CallExpr, callee types.Object, i int) ([]int, error) {
	args := call.Args
	if sig, ok := callee.Type().(*types.Signature); !ok || !sig.Variadic() || i != sig.Params().Len()-1 {
		if i >= len(args) {
			return nil, fmt.Errorf("call %s has too few arguments", types.ExprString(call))
		}
		args = args[:i+1]
	}

	var params []int
	for j := i; j < len(args); j++ {
		values, err := r.operand(args[j])
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			if text := untagPath(v); strings.ReplaceAll(text, "{}", "") != "" {
				return nil, fmt.Errorf("%s builds argument %d into a request path and the guard records it as a runtime value, so it cannot see which endpoint %q selects; pass the whole path to a request helper instead", callee.Name(), j, text)
			}
			params = append(params, placeholderParams(v)...)
		}
	}
	return params, nil
}

func (r *pathResolver) values(expr ast.Expr) ([]string, error) {
	if tv := r.info.Types[expr]; tv.Value != nil && tv.Value.Kind() == constant.String {
		return []string{constant.StringVal(tv.Value)}, nil
	}
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			s, err := strconv.Unquote(e.Value)
			return []string{s}, err
		}
	case *ast.ParenExpr:
		return r.values(e.X)
	case *ast.Ident:
		return r.identValues(e)
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok && isIdent(sel.X, "fmt") && sel.Sel.Name == "Sprintf" && len(e.Args) > 0 {
			if lit, ok := e.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				format, err := strconv.Unquote(lit.Value)
				if err != nil {
					return nil, err
				}
				return r.sprintfValues(format, e.Args[1:])
			}
		}
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			left, err := r.operand(e.X)
			if err != nil {
				return nil, err
			}
			right, err := r.operand(e.Y)
			if err != nil {
				return nil, err
			}
			return concatEach(left, right), nil
		}
	}
	return nil, fmt.Errorf("cannot resolve request path %s", types.ExprString(expr))
}

func (r *pathResolver) sprintfValues(format string, args []ast.Expr) ([]string, error) {
	out := []string{""}
	argIndexes := formatArgs(format)
	last := 0
	for n, loc := range formatVerb.FindAllStringIndex(format, -1) {
		printed := []string{"%"}
		if i := argIndexes[n]; i >= 0 {
			printed = []string{"{}"}
			if i < len(args) && r.info.Types[args[i]].Value != nil {
				printed = []string{formatConstant(format[loc[0]:loc[1]], r.info.Types[args[i]])}
			} else if i < len(args) {
				var err error
				if printed, err = r.operand(args[i]); err != nil {
					return nil, err
				}
			}
		}
		out = concatEach(concatEach(out, []string{format[last:loc[0]]}), printed)
		last = loc[1]
	}
	return concatEach(out, []string{format[last:]}), nil
}

// formatConstant prints a constant format argument the way fmt prints it with
// verb, so a number in a path reads as its text. It prints a runtime value
// instead for a constant of a named type, since fmt calls any String method
// that type has, and for a verb that takes its width or precision from
// another argument.
func formatConstant(verb string, tv types.TypeAndValue) string {
	basic, ok := types.Unalias(tv.Type).(*types.Basic)
	if !ok || strings.Contains(verb, "*") {
		return "{}"
	}
	var v any
	switch info := basic.Info(); {
	case info&types.IsString != 0:
		v = constant.StringVal(tv.Value)
	case info&types.IsBoolean != 0:
		v = constant.BoolVal(tv.Value)
	case info&types.IsInteger != 0:
		n, exact := constant.Int64Val(tv.Value)
		if !exact {
			return "{}"
		}
		v = n
	case info&types.IsFloat != 0:
		v, _ = constant.Float64Val(tv.Value)
	default:
		return "{}"
	}
	return fmt.Sprintf(formatArgIndex.ReplaceAllString(verb, ""), v)
}

// formatArgs returns the index of the argument each verb of format prints, or
// -1 for "%%", following fmt: "[n]" picks argument n and each "*" consumes one.
func formatArgs(format string) []int {
	var out []int
	next := 0
	for _, verb := range formatVerb.FindAllString(format, -1) {
		if verb == "%%" {
			out = append(out, -1)
			continue
		}
		for i := 1; i < len(verb)-1; i++ {
			switch verb[i] {
			case '[':
				end := i + strings.IndexByte(verb[i:], ']')
				n, _ := strconv.Atoi(verb[i+1 : end])
				next = n - 1
				i = end
			case '*':
				next++
			}
		}
		out = append(out, next)
		next++
	}
	return out
}

// identValues follows every assignment to the variable id refers to, since a
// path variable can be reassigned on some branch; a parameter also stands for
// itself.
func (r *pathResolver) identValues(id *ast.Ident) ([]string, error) {
	obj := r.info.Uses[id]
	if obj == nil {
		return nil, fmt.Errorf("cannot resolve request path %s", id.Name)
	}
	if r.visiting[obj] {
		return nil, fmt.Errorf("%w: %s", errPathCycle, id.Name)
	}
	r.visiting[obj] = true
	defer delete(r.visiting, obj)

	var values []string
	if i, ok := r.params[obj]; ok {
		values = append(values, placeholder([]int{i}))
	}

	exprs, err := r.assignments(obj)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve request path %s: %w", id.Name, err)
	}
	for _, expr := range exprs {
		v, err := r.values(expr)
		if err != nil {
			return nil, err
		}
		values = append(values, v...)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("cannot resolve request path %s", id.Name)
	}
	return values, nil
}

// assignments lists what fn assigns to obj. On a form that cannot be split
// per variable it fails but still lists every right-hand side, which is all
// paramsIn needs.
func (r *pathResolver) assignments(obj types.Object) ([]ast.Expr, error) {
	var exprs []ast.Expr
	var err error
	ast.Inspect(r.fn.Body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range s.Lhs {
				if r.objectOf(lhs) != obj {
					continue
				}
				if (s.Tok == token.ASSIGN || s.Tok == token.DEFINE) && len(s.Lhs) == len(s.Rhs) {
					exprs = append(exprs, s.Rhs[i])
					continue
				}
				exprs = append(exprs, s.Rhs...)
				if err == nil {
					err = fmt.Errorf("unsupported assignment %s", s.Tok)
				}
			}
		case *ast.ValueSpec:
			for i, id := range s.Names {
				if r.info.Defs[id] != obj || len(s.Values) == 0 {
					continue
				}
				if len(s.Values) == len(s.Names) {
					exprs = append(exprs, s.Values[i])
					continue
				}
				exprs = append(exprs, s.Values...)
				if err == nil {
					err = errors.New("unsupported declaration")
				}
			}
		case *ast.RangeStmt:
			if r.objectOf(s.Key) == obj || r.objectOf(s.Value) == obj {
				exprs = append(exprs, s.X)
				if err == nil {
					err = errors.New("it is a range variable")
				}
			}
		}
		return true
	})
	return exprs, err
}

// In a path concatenation or format argument the base URL adds nothing and
// anything else unresolvable is a runtime value computed from the parameters
// it mentions. A cycle is not, since only a path variable is built from its
// own value.
func (r *pathResolver) operand(expr ast.Expr) ([]string, error) {
	if sel, ok := expr.(*ast.SelectorExpr); ok && sel.Sel.Name == "BaseURL" {
		return []string{""}, nil
	}
	values, err := r.values(expr)
	if err == nil {
		return values, nil
	}
	if errors.Is(err, errPathCycle) {
		return nil, err
	}
	return []string{placeholder(r.paramsIn(expr, map[types.Object]bool{}))}, nil
}

// paramsIn lists the parameters expr is computed from, following local
// variables to what is assigned to them.
func (r *pathResolver) paramsIn(expr ast.Expr, seen map[types.Object]bool) []int {
	var params []int
	ast.Inspect(expr, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := r.info.Uses[id]
		if obj == nil || seen[obj] {
			return true
		}
		seen[obj] = true
		if i, ok := r.params[obj]; ok {
			params = append(params, i)
			return true
		}
		exprs, _ := r.assignments(obj)
		for _, e := range exprs {
			params = append(params, r.paramsIn(e, seen)...)
		}
		return true
	})
	return params
}

func (r *pathResolver) paramOf(expr ast.Expr) (int, bool) {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return 0, false
	}
	i, ok := r.params[r.info.Uses[id]]
	return i, ok
}

func (r *pathResolver) objectOf(expr ast.Expr) types.Object {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return nil
	}
	if obj := r.info.Defs[id]; obj != nil {
		return obj
	}
	return r.info.Uses[id]
}

func placeholder(params []int) string {
	slices.Sort(params)
	var b strings.Builder
	b.WriteByte('{')
	for _, i := range slices.Compact(params) {
		fmt.Fprintf(&b, "#%d", i)
	}
	b.WriteByte('}')
	return b.String()
}

func placeholderParams(pattern string) []int {
	var params []int
	for _, m := range pathPlaceholder.FindAllStringSubmatch(pattern, -1) {
		for _, s := range strings.Split(m[1], "#")[1:] {
			i, _ := strconv.Atoi(s)
			params = append(params, i)
		}
	}
	return params
}

func untagPath(pattern string) string {
	return pathPlaceholder.ReplaceAllString(pattern, "{}")
}

func concatEach(left, right []string) []string {
	var out []string
	for _, l := range left {
		for _, r := range right {
			out = append(out, l+r)
		}
	}
	return out
}

func isIdent(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}

func jsonString(m map[string]json.RawMessage, key string) string {
	raw, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func jsonBool(m map[string]json.RawMessage, key string) bool {
	raw, ok := m[key]
	if !ok {
		return false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false
	}
	return b
}

func checkSchemaGolden(t *testing.T, guard schemaGuard, actual []string) {
	t.Helper()

	sort.Strings(actual)
	actual = dedupStrings(actual)

	if os.Getenv("SCHEMA_GUARD_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(guard.golden), 0755); err != nil {
			t.Fatalf("creating golden directory: %v", err)
		}
		if err := os.WriteFile(guard.golden, []byte(strings.Join(actual, "\n")+"\n"), 0600); err != nil {
			t.Fatalf("writing golden file: %v", err)
		}
		t.Logf("updated golden file: %s", guard.golden)
		return
	}

	wantBytes, err := os.ReadFile(guard.golden)
	if err != nil {
		t.Fatalf("reading golden file %s: %v\n\n%s", guard.golden, err, guard.regenerateHelp())
	}

	want := strings.Split(strings.TrimSpace(string(wantBytes)), "\n")
	wantSet := map[string]bool{}
	for _, line := range want {
		wantSet[line] = true
	}
	actualSet := map[string]bool{}
	for _, line := range actual {
		actualSet[line] = true
	}

	var missing []string
	for _, line := range want {
		if !actualSet[line] {
			missing = append(missing, line)
		}
	}
	var unexpected []string
	for _, line := range actual {
		if !wantSet[line] {
			unexpected = append(unexpected, line)
		}
	}

	if len(missing) > 0 || len(unexpected) > 0 {
		var msg strings.Builder
		msg.WriteString("schema guard mismatch against " + guard.golden + ":\n")
		if len(missing) > 0 {
			msg.WriteString("\nin the golden, not served now:\n")
			for _, line := range missing {
				msg.WriteString("  - ")
				msg.WriteString(line)
				msg.WriteString("\n")
			}
		}
		if len(unexpected) > 0 {
			msg.WriteString("\nserved now, not in the golden:\n")
			for _, line := range unexpected {
				msg.WriteString("  + ")
				msg.WriteString(line)
				msg.WriteString("\n")
			}
		}
		msg.WriteString("\n")
		msg.WriteString(guard.regenerateHelp())
		t.Fatal(msg.String())
	}
}

func (g schemaGuard) regenerateHelp() string {
	env := "SCHEMA_GUARD_UPDATE=1 TF_ACC=1"
	if g.env != "" {
		env += " " + g.env
	}
	ci := "CI does not check this golden."
	if g.ciStep != "" {
		ci = "CI checks the golden in this step of .github/workflows/test.yml:\n\n  " + g.ciStep
	}
	return fmt.Sprintf(`%[1]s

%[3]s

To regenerate it, run from the repository root against a fresh server of the
Jellyfin release %[7]s names (down -v drops the volumes
a previous run left behind):

  docker compose --env-file %[7]s down -v
  docker compose --env-file %[7]s up -d
  eval "$(./scripts/setup_jellyfin.sh | grep '^export ')"
  %[4]s \
    go test -count=1 -run '^%[2]s$' ./internal/provider/

A maintainer must review the resulting diff before it is committed:

  git diff %[5]s

%[6]s
`, g.records, g.test, ci, env, filepath.ToSlash(filepath.Join("internal", "provider", g.golden)), g.followUp, g.envFile)
}

func dedupStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func TestAccSecurityPluginConfigSchemaGuard(t *testing.T) {
	testAccSecurityPluginPreCheck(t)
	c := testAccInstallSecurityPlugin(t)

	checkSchemaGolden(t, securityPluginPayloadGuard, testAccSecurityPluginPayloadShape(t, c))
}

func TestUnitReduceOpenAPISpec(t *testing.T) {
	spec := `{
  "paths": {
    "/System/Configuration/{key}": {
      "get": {
        "parameters": [{"name": "key", "in": "path", "required": true}],
        "responses": {"200": {"content": {"application/json": {"schema": {"type": "string", "format": "binary"}}}}}
      },
      "post": {
        "parameters": [{"name": "key", "in": "path", "required": true}],
        "requestBody": {"content": {"application/json": {"schema": {}}}},
        "responses": {"204": {}}
      }
    },
    "/System/Configuration/Branding": {
      "post": {
        "requestBody": {"content": {"application/json": {"schema": {"allOf": [{"$ref": "#/components/schemas/BrandingOptionsDto"}]}}}},
        "responses": {"204": {}}
      }
    },
    "/Users/{userId}": {
      "get": {
        "parameters": [{"name": "userId", "in": "path", "required": true}],
        "responses": {
          "200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/UserDto"}}}},
          "404": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/ProblemDetails"}}}}
        }
      }
    },
    "/Users/New": {
      "post": {"responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/UserDto"}}}}}}
    },
    "/Users/AuthenticateByName": {
      "post": {"responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/AuthenticationResult"}}}}}}
    },
    "/Sessions": {
      "get": {"responses": {"200": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Unused"}}}}}}}
    }
  },
  "components": {
    "schemas": {
      "AuthenticationResult": {"type": "object", "properties": {
        "AccessToken": {"type": "string", "nullable": true},
        "SessionInfo": {"allOf": [{"$ref": "#/components/schemas/SessionInfoDto"}], "nullable": true},
        "User": {"allOf": [{"$ref": "#/components/schemas/UserDto"}]}
      }},
      "BaseItemDto": {"type": "object"},
      "BrandingOptionsDto": {"type": "object", "properties": {"CustomCss": {"type": "string", "nullable": true}}},
      "EncodingOptions": {"type": "object", "properties": {
        "HardwareAccelerationType": {"enum": ["vaapi", "none"], "allOf": [{"$ref": "#/components/schemas/HardwareAccelerationType"}]}
      }},
      "HardwareAccelerationType": {"enum": ["vaapi", "none"], "type": "string"},
      "ProblemDetails": {"type": "object"},
      "SessionInfoDto": {"type": "object", "properties": {"NowPlayingItem": {"$ref": "#/components/schemas/BaseItemDto"}}},
      "Unused": {"type": "object"},
      "UserConfiguration": {"type": "object"},
      "UserDto": {"type": "object", "properties": {
        "Configuration": {"$ref": "#/components/schemas/UserConfiguration"},
        "Id": {"type": "string", "format": "uuid"},
        "Policy": {"$ref": "#/components/schemas/UserPolicy"},
        "Self": {"$ref": "#/components/schemas/UserDto"}
      }},
      "UserPolicy": {"type": "object", "properties": {
        "BlockedTags": {"type": "array", "items": {"type": "string"}},
        "Limits": {"type": "object", "additionalProperties": {"type": "integer", "format": "int32"}}
      }, "additionalProperties": {"type": "string"}}
    }
  }
}`

	calls := []apiCall{
		{http.MethodGet, "/System/Configuration/encoding"},
		{http.MethodGet, "/System/Configuration/network"},
		{http.MethodGet, "/System/Configuration/branding"},
		{http.MethodPost, "/System/Configuration/branding"},
		{http.MethodPost, "/System/Configuration/encoding"},
		{http.MethodGet, "/Users/{}"},
		{http.MethodPost, "/Users/{}"},
		{http.MethodPost, "/Users/AuthenticateByName"},
	}

	lines, err := reduceOpenAPISpec(spec, calls)
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}

	want := []string{
		"op GET /System/Configuration/{key} | params=key:path:required=true | resp=string:binary",
		"op GET /Users/{userId} | params=userId:path:required=true | resp=#UserDto",
		"op POST /System/Configuration/Branding | body=#BrandingOptionsDto",
		"op POST /System/Configuration/{key} | params=key:path:required=true | body=any",
		"op POST /Users/AuthenticateByName | resp=#AuthenticationResult",
		"schema AuthenticationResult.AccessToken: string",
		"schema AuthenticationResult.SessionInfo: #SessionInfoDto",
		"schema AuthenticationResult.User: #UserDto",
		"schema BrandingOptionsDto.CustomCss: string",
		"schema EncodingOptions.HardwareAccelerationType: #HardwareAccelerationType",
		"schema HardwareAccelerationType: string enum(none|vaapi)",
		"schema NetworkConfiguration: <missing>",
		"schema UserConfiguration: {object}",
		"schema UserDto.Configuration: #UserConfiguration",
		"schema UserDto.Id: string:uuid",
		"schema UserDto.Policy: #UserPolicy",
		"schema UserDto.Self: #UserDto",
		"schema UserPolicy.BlockedTags: []string",
		"schema UserPolicy.Limits: {object,map=integer:int32}",
		"schema UserPolicy: {object,map=string}",
		"undocumented POST /Users/{}",
	}

	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected lines:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

func TestUnitReduceOpenAPISpecRejectsUnmappedNamedConfiguration(t *testing.T) {
	spec := `{"paths": {"/System/Configuration/{key}": {"get": {}}}}`

	_, err := reduceOpenAPISpec(spec, []apiCall{{http.MethodGet, "/System/Configuration/unknown"}})
	if err == nil || !strings.Contains(err.Error(), "namedConfigurationSchemas") {
		t.Fatalf("expected an unmapped named configuration error, got %v", err)
	}
}

const testClientHelpersSource = `package client

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := c.BaseURL + path
	return http.NewRequestWithContext(ctx, method, url, body)
}

func (c *Client) get(ctx context.Context, path string, decode func(io.Reader) error) error {
	_, err := c.doRequest(ctx, http.MethodGet, path, nil)
	return err
}

func (c *Client) getRaw(ctx context.Context, path string) (string, error) {
	_, err := c.doRequest(ctx, http.MethodGet, path, nil)
	return "", err
}

func (c *Client) postRaw(ctx context.Context, path string, rawJSON string) error {
	_, err := c.doRequest(ctx, http.MethodPost, path, strings.NewReader(rawJSON))
	return err
}

func (c *Client) delete(ctx context.Context, path string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	return err
}
`

func parseTestClient(t *testing.T, sources ...string) (*token.FileSet, []*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	var files []*ast.File
	for i, src := range sources {
		f, err := parser.ParseFile(fset, fmt.Sprintf("client%d.go", i), src, 0)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		files = append(files, f)
	}
	return fset, files
}

func TestUnitPackageAPICalls(t *testing.T) {
	calls := `package client

const (
	countsSegment = "Counts"
	pingPath      = "/System/" + "Ping"
	primaryIndex  = 0
)

type imageType string

func (t imageType) String() string { return "Primary" }

const backdrop imageType = "Backdrop"

type cache struct{}

func (k *cache) get(ctx context.Context, key string, v any) error {
	return nil
}

func (c *Client) Calls(ctx context.Context, id string, existing bool, index int, k *cache) {
	c.get(ctx, "/System/Info", nil)
	c.getRaw(ctx, fmt.Sprintf("/Users/%s/Items?limit=%d", url.PathEscape(id), 5))
	c.get(ctx, fmt.Sprintf("/Items/%[1]s/Images/%-5d", id, index), nil)
	c.delete(ctx, "/Items/"+id)
	apiPath := "/Library/VirtualFolders?" + params.Encode()
	c.postRaw(ctx, apiPath, "")
	u := c.BaseURL + "/Users/AuthenticateByName"
	http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	path := "/Users/New"
	if existing {
		path = fmt.Sprintf("/Users/%s", id)
	}
	c.postRaw(ctx, path, "")
	c.put(ctx, "/Users/Foo", nil)
	c.getConfig(ctx, "/System/Configuration/encoding")
	c.send(ctx, http.MethodHead, "/System/Ping")
	c.HTTPClient.Get(c.BaseURL + "/Health")
	name := "Public"
	c.getRaw(ctx, fmt.Sprintf("/System/Info/%s", name))
	c.getItem(ctx, id)
	c.createKey(ctx, "Terraform")
	c.get(ctx, "/Items/"+countsSegment, nil)
	c.getRaw(ctx, pingPath)
	c.getRaw(ctx, fmt.Sprintf("/Items/%s/Images/%s/%[3]d", id, "Backdrop", primaryIndex))
	c.getRaw(ctx, fmt.Sprintf("/Items/%s/Images/%s", id, backdrop))
	c.getRaw(ctx, "/Items/"+id+"/Images/"+string(backdrop))
	k.get(ctx, "/not/a/request", nil)
	delete(map[string]string{}, "/not/a/request")
}

func (c *Client) Scoped(ctx context.Context, id string) {
	{
		path := "/Startup/User"
		c.getRaw(ctx, path)
	}
	{
		path := "/Plugins/" + id
		c.delete(ctx, path)
	}
}
`
	wrappers := `package client

func (c *Client) put(ctx context.Context, path string, body io.Reader) error {
	_, err := c.doRequest(ctx, http.MethodPut, path, body)
	return err
}

func (c *Client) getConfig(ctx context.Context, path string) (string, error) {
	return c.getRaw(ctx, path)
}

func (c *Client) send(ctx context.Context, method, path string) error {
	_, err := c.doRequest(ctx, method, path, nil)
	return err
}

func (c *Client) getItem(ctx context.Context, itemID string) error {
	return c.get(ctx, "/Items/"+url.PathEscape(itemID), nil)
}

func (c *Client) createKey(ctx context.Context, app string) error {
	return c.postRaw(ctx, fmt.Sprintf("/Auth/Keys?app=%s", url.QueryEscape(app)), "")
}
`
	fset, files := parseTestClient(t, calls, wrappers, testClientHelpersSource)

	got, err := packageAPICalls(fset, files)
	if err != nil {
		t.Fatalf("packageAPICalls: %v", err)
	}

	want := []apiCall{
		{http.MethodGet, "/System/Info"},
		{http.MethodGet, "/Users/{}/Items"},
		{http.MethodGet, "/Items/{}/Images/{}"},
		{http.MethodDelete, "/Items/{}"},
		{http.MethodPost, "/Library/VirtualFolders"},
		{http.MethodPost, "/Users/AuthenticateByName"},
		{http.MethodPost, "/Users/New"},
		{http.MethodPost, "/Users/{}"},
		{http.MethodPut, "/Users/Foo"},
		{http.MethodGet, "/System/Configuration/encoding"},
		{http.MethodHead, "/System/Ping"},
		{http.MethodGet, "/Health"},
		{http.MethodGet, "/System/Info/Public"},
		{http.MethodGet, "/Items/Counts"},
		{http.MethodGet, "/System/Ping"},
		{http.MethodGet, "/Items/{}/Images/Backdrop/0"},
		{http.MethodGet, "/Items/{}/Images/{}"},
		{http.MethodGet, "/Items/{}/Images/Backdrop"},
		{http.MethodGet, "/Startup/User"},
		{http.MethodDelete, "/Plugins/{}"},
		{http.MethodGet, "/Items/{}"},
		{http.MethodPost, "/Auth/Keys"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected calls:\n%v\nwant:\n%v", got, want)
	}
}

func TestUnitPackageAPICallsRejectsUnfollowableRequests(t *testing.T) {
	tests := map[string]struct {
		src     string
		wantErr string
	}{
		"unresolvable path": {
			src: `package client

func (c *Client) Calls(ctx context.Context) {
	c.get(ctx, routes.Users, nil)
}
`,
			wantErr: "routes.Users",
		},
		"path built from itself": {
			src: `package client

func (c *Client) Calls(ctx context.Context, q string) {
	path := "/Users"
	path = path + "?" + q
	c.get(ctx, path, nil)
}
`,
			wantErr: "built from its own value: path",
		},
		"method forwarded without its path": {
			src: `package client

func (c *Client) ping(ctx context.Context, method string) error {
	_, err := c.doRequest(ctx, method, "/System/Ping", nil)
	return err
}
`,
			wantErr: "request method comes from a parameter",
		},
		"forwarding function without callers": {
			src: `package client

func (c *Client) GetRaw(ctx context.Context, path string) (string, error) {
	return c.getRaw(ctx, path)
}
`,
			wantErr: "GetRaw passes a parameter on as a request path",
		},
		"path text for a parameter built into a path": {
			src: `package client

func (c *Client) getItem(ctx context.Context, sub string) error {
	return c.get(ctx, "/Items/"+sub, nil)
}

func (c *Client) Counts(ctx context.Context) error {
	return c.getItem(ctx, "Counts")
}
`,
			wantErr: `getItem builds argument 1 into a request path and the guard records it as a runtime value, so it cannot see which endpoint "Counts" selects`,
		},
		"constant path text for a parameter built into a path": {
			src: `package client

const countsSegment = "Counts"

func (c *Client) getItem(ctx context.Context, sub string) error {
	return c.get(ctx, "/Items/"+sub, nil)
}

func (c *Client) Counts(ctx context.Context) error {
	return c.getItem(ctx, countsSegment)
}
`,
			wantErr: `getItem builds argument 1 into a request path and the guard records it as a runtime value, so it cannot see which endpoint "Counts" selects`,
		},
		"path text for a later variadic argument built into a path": {
			src: `package client

func (c *Client) getPath(ctx context.Context, parts ...string) error {
	return c.get(ctx, "/"+strings.Join(parts, "/"), nil)
}

func (c *Client) Counts(ctx context.Context, id string) error {
	return c.getPath(ctx, id, "Counts")
}
`,
			wantErr: `getPath builds argument 2 into a request path and the guard records it as a runtime value, so it cannot see which endpoint "Counts" selects`,
		},
		"request helper called through an interface": {
			src: `package client

type getter interface {
	get(ctx context.Context, path string, decode func(io.Reader) error) error
}

func (c *Client) Hidden(ctx context.Context) {
	var g getter = c
	g.get(ctx, "/Hidden", nil)
}
`,
			wantErr: "the guard cannot tell whether g.get is the request helper get",
		},
		"request helper called on a value whose type comes from an import": {
			src: `package client

func (c *Client) Hidden(ctx context.Context) {
	lo.Must(c, nil).get(ctx, "/Hidden", nil)
}
`,
			wantErr: `the guard cannot tell whether lo.Must(c, nil).get is the request helper get`,
		},
		"request helper called as a method expression": {
			src: `package client

func (c *Client) Hidden(ctx context.Context) {
	(*Client).get(c, ctx, "/Hidden", nil)
}
`,
			wantErr: "(*Client).get is called as a method expression",
		},
		"request helper held by a package variable": {
			src: `package client

var getHidden = (*Client).get

func (c *Client) Hidden(ctx context.Context) {
	getHidden(c, ctx, "/Hidden", nil)
}
`,
			wantErr: "(*Client).get sends requests but is used here outside a function declaration",
		},
		"request helper called from a package variable's function literal": {
			src: `package client

var hidden = func(ctx context.Context, c *Client) error {
	return c.get(ctx, "/Hidden", nil)
}
`,
			wantErr: "c.get sends requests but is used here outside a function declaration",
		},
		"path text passed on to a parameter built into a path": {
			src: `package client

func (c *Client) getItem(ctx context.Context, sub string) error {
	return c.get(ctx, fmt.Sprintf("/Items/%s", url.PathEscape(sub)), nil)
}

func (c *Client) getNamed(ctx context.Context, name string) error {
	lower := strings.ToLower(name)
	return c.getItem(ctx, lower)
}

func (c *Client) Counts(ctx context.Context) error {
	return c.getNamed(ctx, "Counts")
}
`,
			wantErr: "getNamed builds argument 1 into a request path",
		},
		"request helper used as a value": {
			src: `package client

func (c *Client) Hidden(ctx context.Context) {
	f := c.get
	f(ctx, "/Hidden", nil)
}
`,
			wantErr: "c.get sends requests but is used here as a value",
		},
		"net/http request function used as a value": {
			src: `package client

func (c *Client) Hidden(ctx context.Context) {
	newRequest := http.NewRequestWithContext
	newRequest(ctx, http.MethodGet, c.BaseURL+"/Hidden", nil)
}
`,
			wantErr: "http.NewRequestWithContext sends requests but is used here as a value",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			fset, files := parseTestClient(t, tt.src, testClientHelpersSource)
			if _, err := packageAPICalls(fset, files); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected an error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestUnitFormatArgs(t *testing.T) {
	tests := map[string][]int{
		"/Users/%s/Items/%5d":   {0, 1},
		"/Users/%[2]s/%[1]s/%s": {1, 0, 1},
		"/Items/%-5.2f":         {0},
		"/Items/%*d/%s":         {1, 2},
		"/Items/%[3]*.[2]*[1]f": {0},
		"/Search/100%%/%s":      {-1, 0},
	}
	for format, want := range tests {
		if got := formatArgs(format); !slices.Equal(got, want) {
			t.Errorf("formatArgs(%q) = %v, want %v", format, got, want)
		}
	}
}

func TestUnitGoldenClientDrift(t *testing.T) {
	golden := []string{
		"op GET /System/Configuration/{key} | params=key:path:required=true | resp=string:binary",
		"op GET /Users/{userId} | params=userId:path:required=true | resp=#UserDto",
		"op POST /Startup/Complete",
		"schema EncodingOptions.EnableAudioVbr: boolean",
		"undocumented POST /Users/{}",
	}
	matching := []apiCall{
		{http.MethodGet, "/System/Configuration/encoding"},
		{http.MethodGet, "/Users/{}"},
		{http.MethodPost, "/Users/{}"},
		{http.MethodPost, "/Startup/Complete"},
	}
	if drift := goldenClientDrift(golden, matching); len(drift) > 0 {
		t.Fatalf("expected no drift, got:\n%s", strings.Join(drift, "\n"))
	}

	changed := []apiCall{
		{http.MethodGet, "/System/Configuration/encoding"},
		{http.MethodGet, "/System/Configuration/network"},
		{http.MethodGet, "/Users/{}"},
		{http.MethodPost, "/Users/{}"},
		{http.MethodPost, "/Environment/ValidatePath"},
	}
	want := []string{
		`  + internal/client uses named configuration "network", whose schema NetworkConfiguration the golden does not list`,
		"  + internal/client calls POST /Environment/ValidatePath, which the golden does not list",
		"  - the golden lists op POST /Startup/Complete, which internal/client no longer calls",
	}
	if got := goldenClientDrift(golden, changed); !slices.Equal(got, want) {
		t.Fatalf("unexpected drift:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestUnitJellyfinAPISchemaGoldenMatchesClientCalls(t *testing.T) {
	calls, err := clientAPICalls(clientSourceDir)
	if err != nil {
		t.Fatalf("clientAPICalls: %v", err)
	}
	golden, err := os.ReadFile(jellyfinAPISchemaGolden)
	if err != nil {
		t.Fatalf("reading golden file: %v", err)
	}

	if drift := goldenClientDrift(strings.Split(strings.TrimSpace(string(golden)), "\n"), calls); len(drift) > 0 {
		t.Fatalf("%s does not match the endpoints internal/client calls:\n\n%s\n\n%s", jellyfinAPISchemaGolden, strings.Join(drift, "\n"), jellyfinAPISchemaGuard.regenerateHelp())
	}
}

func TestUnitSchemaGuardsRunInTheWorkflowStepTheyName(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/test.yml")
	if err != nil {
		t.Fatalf("reading the workflow: %v", err)
	}

	for _, g := range []schemaGuard{jellyfinAPISchemaGuard, securityPluginPayloadGuard} {
		step := workflowStep(string(raw), g.ciStep)
		if step == "" {
			t.Errorf("%s: .github/workflows/test.yml has no step named %q", g.test, g.ciStep)
			continue
		}
		if runs, err := stepRunsTest(step, g.test); err != nil {
			t.Errorf("%s: step %q: %v", g.test, g.ciStep, err)
		} else if !runs {
			t.Errorf("%s: no go test command in step %q runs it in ./internal/provider/", g.test, g.ciStep)
		}
		for _, kv := range strings.Fields("TF_ACC=1 " + g.env) {
			key, value, _ := strings.Cut(kv, "=")
			if !strings.Contains(step, fmt.Sprintf("%s: %q", key, value)) {
				t.Errorf("%s: step %q does not set %s", g.test, g.ciStep, kv)
			}
		}
	}
}

func workflowStep(workflow, name string) string {
	lines := strings.Split(workflow, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "- name: "+name {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
		end := i + 1
		for end < len(lines) && (strings.TrimSpace(lines[end]) == "" || strings.HasPrefix(lines[end], indent+"  ")) {
			end++
		}
		return strings.Join(lines[i:end], "\n")
	}
	return ""
}

func TestUnitStepRunsTestReadsRunAndSkip(t *testing.T) {
	tests := map[string]struct {
		step string
		want bool
	}{
		"no filter":            {"run: go test -count=1 -v -timeout 10m ./internal/provider/", true},
		"single-quoted -run":   {"run: go test -run 'TestAccRestartResource|TestAccSecurityPlugin' ./internal/provider/", true},
		"double-quoted -run":   {`run: go test -run "TestAccRestartResource" ./internal/provider/`, false},
		"-run=":                {"run: go test -run=TestAccRestartResource ./internal/provider/", false},
		"--test.run":           {"run: go test --test.run TestAccRestartResource ./internal/provider/", false},
		"-run of subtests":     {"run: go test -run 'TestAccSecurityPlugin.*/Sub' ./internal/provider/", true},
		"-skip":                {"run: go test -skip TestAccSecurityPluginConfigSchemaGuard ./internal/provider/", false},
		"-skip of subtests":    {"run: go test -skip 'TestAccSecurityPluginConfigSchemaGuard/Sub' ./internal/provider/", true},
		"later -run wins":      {"run: go test -run TestAccSecurityPlugin -run TestAccRestartResource ./internal/provider/", false},
		"other package":        {"run: go test -run TestAccSecurityPlugin ./cmd/jellyfin-import/", false},
		"second command":       {"run: go vet ./... && go test -run TestAccSecurityPlugin ./internal/provider/", true},
		"continued line":       {"run: |\n  go test -v \\\n    -run TestAccRestartResource ./internal/provider/", false},
		"commented-out filter": {"run: go test ./internal/provider/ # -run TestAccRestartResource", true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := stepRunsTest(tt.step, "TestAccSecurityPluginConfigSchemaGuard")
			if err != nil {
				t.Fatalf("stepRunsTest: %v", err)
			}
			if got != tt.want {
				t.Errorf("stepRunsTest = %t, want %t", got, tt.want)
			}
		})
	}
}

// stepRunsTest reports whether a go test command in a workflow step runs the
// top-level test name in ./internal/provider/.
func stepRunsTest(step, name string) (bool, error) {
	for _, cmd := range goTestCommands(step) {
		if !slices.Contains(cmd.args, "./internal/provider/") {
			continue
		}
		if ok, err := cmd.selects(name); err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

// goTestCommand is one go test invocation: the words that are not flags, among
// them the packages, and the last -run and -skip patterns.
type goTestCommand struct {
	args      []string
	run, skip string
}

// goTestCommands reads -run and -skip in every spelling the go command takes:
// one or two dashes, with or without the test. prefix, and the pattern after
// "=" or as the next word.
func goTestCommands(script string) []goTestCommand {
	var cmds []goTestCommand
	for _, line := range strings.Split(strings.ReplaceAll(script, "\\\n", " "), "\n") {
		words := shellWords(line)
		for i := 0; i+1 < len(words); i++ {
			if words[i] != "go" || words[i+1] != "test" {
				continue
			}
			var cmd goTestCommand
			for i += 2; i < len(words) && !slices.Contains([]string{"&&", "||", ";", "|"}, words[i]); i++ {
				if !strings.HasPrefix(words[i], "-") {
					cmd.args = append(cmd.args, words[i])
					continue
				}
				flag, value, hasValue := strings.Cut(strings.TrimLeft(words[i], "-"), "=")
				flag = strings.TrimPrefix(flag, "test.")
				if flag != "run" && flag != "skip" {
					continue
				}
				if !hasValue && i+1 < len(words) {
					i++
					value = words[i]
				}
				if flag == "run" {
					cmd.run = value
				} else {
					cmd.skip = value
				}
			}
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

// selects follows go test for a top-level test: a pattern holds one regexp per
// level, split at "/", so -run matches its first level against the name, while
// -skip with more than one level skips only subtests. go test does not split
// at a "/" inside brackets or parentheses; this does, which no step needs.
func (c goTestCommand) selects(name string) (bool, error) {
	if c.run != "" {
		top, _, _ := strings.Cut(c.run, "/")
		re, err := regexp.Compile(top)
		if err != nil {
			return false, fmt.Errorf("-run %q: %w", c.run, err)
		}
		if !re.MatchString(name) {
			return false, nil
		}
	}
	if c.skip != "" && !strings.Contains(c.skip, "/") {
		re, err := regexp.Compile(c.skip)
		if err != nil {
			return false, fmt.Errorf("-skip %q: %w", c.skip, err)
		}
		if re.MatchString(name) {
			return false, nil
		}
	}
	return true, nil
}

// shellWords splits a line into words at blanks outside quotes, strips single
// and double quotes and drops a comment. Unlike sh it expands nothing and reads
// no backslash escapes, which the workflow's go test commands do not use.
func shellWords(line string) []string {
	var words []string
	var word strings.Builder
	inWord := false
	var quote rune
	for _, c := range line {
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote != 0:
			word.WriteRune(c)
		case c == '\'' || c == '"':
			quote, inWord = c, true
		case c == ' ' || c == '\t':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		case c == '#' && !inWord:
			return words
		default:
			word.WriteRune(c)
			inWord = true
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	return words
}

func TestUnitClientAPICallsNamedConfigurationsAreMapped(t *testing.T) {
	calls, err := clientAPICalls(clientSourceDir)
	if err != nil {
		t.Fatalf("clientAPICalls: %v", err)
	}

	for _, call := range calls {
		key, ok := strings.CutPrefix(call.path, "/System/Configuration/")
		if !ok || strings.Contains(key, "/") {
			continue
		}
		if _, ok := namedConfigurationSchemas[strings.ToLower(key)]; !ok {
			t.Errorf("%s %s: named configuration %q is missing from namedConfigurationSchemas", call.method, call.path, key)
		}
	}
}
