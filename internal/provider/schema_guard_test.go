// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	jellyfinAPISchemaGolden     = "testdata/jellyfin_api_schema.golden"
	ssoPluginConfigSchemaGolden = "testdata/sso_plugin_config_schema.golden"
	clientSourceDir             = "../client"
	namedConfigurationPath      = "/System/Configuration/{key}"
)

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

var clientRequestHelpers = map[string]string{
	"get":           http.MethodGet,
	"getRaw":        http.MethodGet,
	"post":          http.MethodPost,
	"postRaw":       http.MethodPost,
	"postAndDecode": http.MethodPost,
	"delete":        http.MethodDelete,
}

var formatVerb = regexp.MustCompile(`%[a-zA-Z]`)

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

	checkSchemaGolden(t, jellyfinAPISchemaGolden, lines)
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
		sig, err := typeSignature(s, refs)
		if err != nil {
			return nil, fmt.Errorf("signature for %s: %w", name, err)
		}
		out = append(out, fmt.Sprintf("schema %s: %s", name, sig))

		for ref := range refs {
			queue = append(queue, ref)
		}
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
	var calls []apiCall
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			return nil, err
		}
		fileCalls, err := fileAPICalls(fset, f)
		if err != nil {
			return nil, err
		}
		calls = append(calls, fileCalls...)
	}
	if len(calls) == 0 {
		return nil, fmt.Errorf("no API calls found in %s", dir)
	}
	return calls, nil
}

func fileAPICalls(fset *token.FileSet, f *ast.File) ([]apiCall, error) {
	var calls []apiCall
	var firstErr error
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if firstErr != nil {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			method, pathExpr := requestCall(call)
			if method == "" {
				return true
			}
			path, forwarded, err := pathPattern(fn, pathExpr)
			if err == nil && !forwarded {
				path, _, _ = strings.Cut(path, "?")
				if !strings.HasPrefix(path, "/") {
					err = fmt.Errorf("request path %q does not start with /", path)
				}
			}
			if err != nil {
				firstErr = fmt.Errorf("%s: %w", fset.Position(pathExpr.Pos()), err)
				return false
			}
			if !forwarded {
				calls = append(calls, apiCall{method: method, path: path})
			}
			return true
		})
	}
	return calls, firstErr
}

// requestCall recognizes the client's request helpers and requests built with
// an explicit http.Method* constant. A method passed in as a variable belongs to
// the shared plumbing, whose callers are recorded instead.
func requestCall(call *ast.CallExpr) (string, ast.Expr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", nil
	}
	if method, ok := clientRequestHelpers[sel.Sel.Name]; ok && len(call.Args) >= 2 {
		return method, call.Args[1]
	}
	if (sel.Sel.Name == "doRequest" || sel.Sel.Name == "NewRequestWithContext") && len(call.Args) >= 3 {
		m, ok := call.Args[1].(*ast.SelectorExpr)
		if ok && isIdent(m.X, "http") && strings.HasPrefix(m.Sel.Name, "Method") {
			return strings.ToUpper(strings.TrimPrefix(m.Sel.Name, "Method")), call.Args[2]
		}
	}
	return "", nil
}

// pathPattern evaluates a request path expression with "{}" standing in for
// every runtime value. forwarded reports a path that is a parameter of fn,
// i.e. plumbing whose callers carry the real path.
func pathPattern(fn *ast.FuncDecl, expr ast.Expr) (pattern string, forwarded bool, err error) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			s, err := strconv.Unquote(e.Value)
			return s, false, err
		}
	case *ast.Ident:
		if isParam(fn, e.Name) {
			return "", true, nil
		}
		if rhs := localAssignment(fn, e.Name); rhs != nil {
			return pathPattern(fn, rhs)
		}
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok && isIdent(sel.X, "fmt") && sel.Sel.Name == "Sprintf" && len(e.Args) > 0 {
			if lit, ok := e.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				s, err := strconv.Unquote(lit.Value)
				return formatVerb.ReplaceAllString(s, "{}"), false, err
			}
		}
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			left, leftForwarded := concatOperand(fn, e.X)
			right, rightForwarded := concatOperand(fn, e.Y)
			if (leftForwarded && right == "") || (rightForwarded && left == "") {
				return "", true, nil
			}
			if leftForwarded {
				left = "{}"
			}
			if rightForwarded {
				right = "{}"
			}
			return left + right, false, nil
		}
	}
	return "", false, fmt.Errorf("cannot resolve request path %s", types.ExprString(expr))
}

func concatOperand(fn *ast.FuncDecl, expr ast.Expr) (string, bool) {
	if sel, ok := expr.(*ast.SelectorExpr); ok && sel.Sel.Name == "BaseURL" {
		return "", false
	}
	s, forwarded, err := pathPattern(fn, expr)
	if err != nil {
		return "{}", false
	}
	return s, forwarded
}

func isParam(fn *ast.FuncDecl, name string) bool {
	for _, field := range fn.Type.Params.List {
		for _, n := range field.Names {
			if n.Name == name {
				return true
			}
		}
	}
	return false
}

func localAssignment(fn *ast.FuncDecl, name string) ast.Expr {
	var rhs ast.Expr
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if rhs != nil {
			return false
		}
		if assign, ok := n.(*ast.AssignStmt); ok && len(assign.Lhs) == len(assign.Rhs) {
			for i, lhs := range assign.Lhs {
				if isIdent(lhs, name) {
					rhs = assign.Rhs[i]
					return false
				}
			}
		}
		return true
	})
	return rhs
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

func checkSchemaGolden(t *testing.T, goldenPath string, actual []string) {
	t.Helper()

	sort.Strings(actual)
	actual = dedupStrings(actual)

	if os.Getenv("SCHEMA_GUARD_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("creating golden directory: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(strings.Join(actual, "\n")+"\n"), 0600); err != nil {
			t.Fatalf("writing golden file: %v", err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	regenerate := schemaGuardRegenerateHelp(t.Name(), goldenPath)

	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden file %s: %v\n\n%s", goldenPath, err, regenerate)
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
		msg.WriteString("schema guard mismatch against " + goldenPath + ":\n")
		if len(missing) > 0 {
			msg.WriteString("\nremoved or renamed (in golden, not served):\n")
			for _, line := range missing {
				msg.WriteString("  - ")
				msg.WriteString(line)
				msg.WriteString("\n")
			}
		}
		if len(unexpected) > 0 {
			msg.WriteString("\nadded (served, not in golden):\n")
			for _, line := range unexpected {
				msg.WriteString("  + ")
				msg.WriteString(line)
				msg.WriteString("\n")
			}
		}
		msg.WriteString("\n")
		msg.WriteString(regenerate)
		t.Fatal(msg.String())
	}
}

func schemaGuardRegenerateHelp(testName, goldenPath string) string {
	return fmt.Sprintf(`The golden changes when the served API changes or when internal/client starts
or stops calling an endpoint. Regenerate it from the repository root against a
fresh server of the supported Jellyfin version:

  docker compose --env-file internal/provider/supported_jellyfin_version.env up -d
  eval "$(./scripts/setup_jellyfin.sh | grep '^export ')"
  SCHEMA_GUARD_UPDATE=1 TF_ACC=1 go test -count=1 -run '^%[1]s$' ./internal/provider/

A maintainer must review the resulting diff before it is committed:

  git diff internal/provider/%[2]s

Every line is an endpoint the provider calls or a schema it sends or reads, so
a change can break a resource even though this test passes again. Fix the
affected resources in the same change.
`, testName, goldenPath)
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

func TestAccSSOPluginConfigSchemaGuard(t *testing.T) {
	t.Skip("SSO plugin payload schema guard requires plugin installation; run manually with SCHEMA_GUARD_UPDATE=1 after installing the SSO-Auth plugin")
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
      "UserDto": {"type": "object", "properties": {
        "Id": {"type": "string", "format": "uuid"},
        "Policy": {"$ref": "#/components/schemas/UserPolicy"},
        "Self": {"$ref": "#/components/schemas/UserDto"}
      }},
      "UserPolicy": {"type": "object", "properties": {
        "BlockedTags": {"type": "array", "items": {"type": "string"}},
        "Limits": {"type": "object", "additionalProperties": {"type": "integer", "format": "int32"}}
      }}
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
		"schema AuthenticationResult: {object,AccessToken=string,SessionInfo=#SessionInfoDto,User=#UserDto}",
		"schema BrandingOptionsDto: {object,CustomCss=string}",
		"schema EncodingOptions: {object,HardwareAccelerationType=#HardwareAccelerationType}",
		"schema HardwareAccelerationType: string enum(none|vaapi)",
		"schema NetworkConfiguration: <missing>",
		"schema UserDto: {object,Id=string:uuid,Policy=#UserPolicy,Self=#UserDto}",
		"schema UserPolicy: {object,BlockedTags=[]string,Limits={object,map=integer:int32}}",
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

func TestUnitFileAPICalls(t *testing.T) {
	src := `package client

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	return http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
}

func (c *Client) get(ctx context.Context, path string, decode func(io.Reader) error) error {
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	return err
}

func (c *Client) Calls(ctx context.Context, id string) {
	c.get(ctx, "/System/Info", nil)
	c.getRaw(ctx, fmt.Sprintf("/Users/%s/Items?limit=%d", url.PathEscape(id), 5))
	c.delete(ctx, "/Items/"+id)
	apiPath := "/Library/VirtualFolders?" + params.Encode()
	c.postRaw(ctx, apiPath, "")
	u := c.BaseURL + "/Users/AuthenticateByName"
	http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "client.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	calls, err := fileAPICalls(fset, f)
	if err != nil {
		t.Fatalf("fileAPICalls: %v", err)
	}

	want := []apiCall{
		{http.MethodGet, "/System/Info"},
		{http.MethodGet, "/Users/{}/Items"},
		{http.MethodDelete, "/Items/{}"},
		{http.MethodPost, "/Library/VirtualFolders"},
		{http.MethodPost, "/Users/AuthenticateByName"},
	}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Fatalf("unexpected calls:\n%v\nwant:\n%v", calls, want)
	}
}

func TestUnitFileAPICallsRejectsUnresolvablePath(t *testing.T) {
	src := `package client

func (c *Client) Calls(ctx context.Context) {
	c.get(ctx, routes.Users, nil)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "client.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if _, err := fileAPICalls(fset, f); err == nil || !strings.Contains(err.Error(), "routes.Users") {
		t.Fatalf("expected an unresolvable path error, got %v", err)
	}
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
