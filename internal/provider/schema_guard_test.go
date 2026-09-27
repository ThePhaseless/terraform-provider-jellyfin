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
)

const (
	jellyfinAPISchemaGolden     = "testdata/jellyfin_api_schema.golden"
	securityPluginPayloadGolden = "testdata/security_plugin_config_schema.golden"
)

const (
	clientSourceDir        = "../client"
	namedConfigurationPath = "/System/Configuration/{key}"
)

// schemaGuard is what a mismatch against one golden file has to tell the
// maintainer: what the lines record, which acceptance test regenerates them
// and with what environment, and what has to change along with them.
type schemaGuard struct {
	golden   string
	test     string
	ciStep   string
	env      string
	records  string
	followUp string
}

var jellyfinAPISchemaGuard = schemaGuard{
	golden: jellyfinAPISchemaGolden,
	test:   "TestAccJellyfinAPISchemaGuard",
	ciStep: "Run acceptance tests",
	records: `Each line is an endpoint internal/client calls ("op", or "undocumented" when
the server's OpenAPI document does not describe it) or a property of a schema
those endpoints send or return ("schema"), as that document describes it. The
golden changes when a Jellyfin release changes one of those, or when
internal/client starts or stops calling an endpoint.`,
	followUp: `A changed line can break a resource even though the test passes again, so fix
the affected resources in the same change.`,
}

var securityPluginPayloadGuard = schemaGuard{
	golden: securityPluginPayloadGolden,
	test:   "TestAccSecurityPluginConfigSchemaGuard",
	ciStep: "Run restart acceptance tests (isolated)",
	env:    "JELLYFIN_RESTART_ACC=1",
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
	followUp: `jellyfin_security_plugin_configuration maps each key by hand, so update it in
the same change: TestUnitJellyfinSecurityWritesBackExactlyTheServedKeys fails
until the resource writes back exactly the keys the golden lists, less those
securityPluginUnmanagedKeys excuses.`,
}

// The spec types /System/Configuration/{key} as an opaque blob, so the schema
// behind each key the client reads or writes has to be named by hand.
var namedConfigurationSchemas = map[string]string{
	"branding": "BrandingOptionsDto",
	"encoding": "EncodingOptions",
	"livetv":   "LiveTvOptions",
	"metadata": "MetadataConfiguration",
	"network":  "NetworkConfiguration",
}

// The provider reads only AccessToken from AuthenticationResult; expanding its
// SessionInfo would drag the whole playback model (BaseItemDto,
// MediaSourceInfo, DeviceProfile, ...) into the guard.
var unexpandedSchemas = map[string]bool{
	"SessionInfoDto": true,
}

// formatVerb matches a whole fmt verb, with any flags, argument index, width
// and precision, so "%[1]s" and "%5d" each print one argument.
var formatVerb = regexp.MustCompile(`%[-+# 0]*(?:\[\d+\])?(?:\*|\d+)?(?:\.(?:\[\d+\])?(?:\*|\d+)?)?(?:\[\d+\])?[a-zA-Z%]`)

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
	c := testAccClient(t)

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

	checkSchemaGolden(t, jellyfinAPISchemaGuard, lines)
}

// reduceOpenAPISpec keeps only the operations the client calls and the schemas
// they reach, so a Jellyfin release trips the guard only when it changes
// something the provider depends on.
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

// schemaLines puts each property of an object schema on a line of its own, so
// a changed field is one short line in the golden diff rather than an edit
// inside a line of several kilobytes.
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
// source so the guard follows the client without a hand-kept endpoint list.
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
// for its callers, repeated until none turns up. A parameter built into a
// larger path is recorded as a runtime value. A call in the package that
// passes path text for such a parameter, or a request site used as a value,
// fails instead, since the guard would record the wrong endpoint or none.
//
// Two cases still record the wrong endpoint. Only this package is read, so
// path text that a caller elsewhere passes to an exported method, such as
// "Counts" for an item ID, selects a route the guard does not see. And a path
// operand that is not a constant, a parameter or a local variable it can
// follow, such as a call result, a field or a package variable, is recorded as
// a runtime value even when it always holds the same text.
func packageAPICalls(fset *token.FileSet, files []*ast.File) ([]apiCall, error) {
	info := &types.Info{
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
		Types: map[ast.Expr]types.TypeAndValue{},
	}
	// Only the scopes of local names and the values of constants are needed, so
	// imports stay unresolved and the type errors that causes are ignored.
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

	f := &requestFinder{fset: fset, info: info, sites: map[string][]requestSite{}, embeds: map[string][]int{}}
	for {
		var calls []apiCall
		f.reached = map[string]bool{}
		changed := false
		for _, fn := range funcs {
			fnCalls, fnSites, fnEmbeds, err := f.requestsIn(fn)
			if err != nil {
				return nil, err
			}
			calls = append(calls, fnCalls...)
			key := siteKey(fn)
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

		for _, fn := range funcs {
			if err := f.siteValues(fn); err != nil {
				return nil, err
			}
		}
		for _, fn := range funcs {
			if key := siteKey(fn); len(f.sites[key]) > 0 && !f.reached[key] {
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

// requestFinder holds, by siteKey, the request sites found so far and the
// parameters each function builds into a request path.
type requestFinder struct {
	fset    *token.FileSet
	info    *types.Info
	sites   map[string][]requestSite
	embeds  map[string][]int
	reached map[string]bool
}

// siteKey names a function the way a call spells it, so the client's delete
// method stays apart from the builtin delete.
func siteKey(fn *ast.FuncDecl) string {
	if fn.Recv != nil {
		return "." + fn.Name.Name
	}
	return fn.Name.Name
}

func (f *requestFinder) callSites(call *ast.CallExpr) (string, []requestSite) {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		if isNetHTTP(fun.X) {
			if site, ok := netHTTPRequestSites[fun.Sel.Name]; ok {
				return "", []requestSite{site}
			}
			return "", nil
		}
		return "." + fun.Sel.Name, f.sites["."+fun.Sel.Name]
	case *ast.Ident:
		return fun.Name, f.sites[fun.Name]
	}
	return "", nil
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
		key, callee := f.callSites(call)
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
			embedded, err := r.embeddedArg(call, strings.TrimPrefix(key, "."), i)
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

// siteValues rejects a request site that fn uses other than by calling it, as
// in f := c.get, since the guard cannot tell what is requested through it.
func (f *requestFinder) siteValues(fn *ast.FuncDecl) error {
	skip := map[ast.Node]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.CallExpr:
			skip[e.Fun] = true
		case *ast.SelectorExpr:
			skip[e.Sel] = true
		}
		return true
	})

	var err error
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if err != nil || skip[n] {
			return err == nil
		}
		var name string
		switch e := n.(type) {
		case *ast.SelectorExpr:
			if _, ok := netHTTPRequestSites[e.Sel.Name]; (ok && isNetHTTP(e.X)) || len(f.sites["."+e.Sel.Name]) > 0 {
				name = types.ExprString(e)
			}
		case *ast.Ident:
			if _, ok := f.info.Uses[e].(*types.Func); ok && len(f.sites[e.Name]) > 0 {
				name = e.Name
			}
		}
		if name != "" {
			err = fmt.Errorf("%s: %s sends requests but is used here as a value, so the guard cannot see what it requests; call it directly", f.fset.Position(n.Pos()), name)
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
// computed from, which fn then builds into a request path as well.
func (r *pathResolver) embeddedArg(call *ast.CallExpr, callee string, i int) ([]int, error) {
	if i >= len(call.Args) {
		return nil, fmt.Errorf("call %s has too few arguments", types.ExprString(call))
	}
	values, err := r.operand(call.Args[i])
	if err != nil {
		return nil, err
	}
	var params []int
	for _, v := range values {
		if text := untagPath(v); strings.ReplaceAll(text, "{}", "") != "" {
			return nil, fmt.Errorf("%s builds argument %d into a request path and the guard records it as a runtime value, so it cannot see which endpoint %q selects; pass the whole path to a request helper instead", callee, i, text)
		}
		params = append(params, placeholderParams(v)...)
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
			if i < len(args) {
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
	return fmt.Sprintf(`%[1]s

CI checks the golden in this step of .github/workflows/test.yml:

  %[3]s

To regenerate it, run from the repository root against a fresh server of the
supported Jellyfin version (down -v drops the volumes a previous run left
behind):

  docker compose --env-file internal/provider/supported_jellyfin_version.env down -v
  docker compose --env-file internal/provider/supported_jellyfin_version.env up -d
  eval "$(./scripts/setup_jellyfin.sh | grep '^export ')"
  %[4]s \
    go test -count=1 -run '^%[2]s$' ./internal/provider/

A maintainer must review the resulting diff before it is committed:

  git diff internal/provider/%[5]s

%[6]s
`, g.records, g.test, g.ciStep, env, g.golden, g.followUp)
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
)

func (c *Client) Calls(ctx context.Context, id string, existing bool) {
	c.get(ctx, "/System/Info", nil)
	c.getRaw(ctx, fmt.Sprintf("/Users/%s/Items?limit=%d", url.PathEscape(id), 5))
	c.get(ctx, fmt.Sprintf("/Items/%[1]s/Images/%-5d", id, 1), nil)
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
	runFilter := regexp.MustCompile(`go test .*-run '([^']+)'`)

	for _, g := range []schemaGuard{jellyfinAPISchemaGuard, securityPluginPayloadGuard} {
		step := workflowStep(string(raw), g.ciStep)
		if step == "" {
			t.Errorf("%s: .github/workflows/test.yml has no step named %q", g.test, g.ciStep)
			continue
		}
		if !strings.Contains(step, "./internal/provider/") {
			t.Errorf("%s: step %q does not test ./internal/provider/", g.test, g.ciStep)
		}
		if m := runFilter.FindStringSubmatch(step); m != nil && !regexp.MustCompile(m[1]).MatchString(g.test) {
			t.Errorf("%s: step %q runs only -run '%s'", g.test, g.ciStep, m[1])
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
