// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func TestUnitJellyfinSecurityNotificationAuthRoundTrip(t *testing.T) {
	ctx := t.Context()
	b := mustWire(t, securityPluginWire)
	fixture := `{"NtfyToken":"tk_abc","NtfyUsername":"alice","NtfyPassword":"s3cret","WebhookHeaders":["X-Api-Key: k","Authorization: Bearer t"]}`

	data := readWire[JellyfinSecurityPluginConfigurationResourceModel](t, b, fixture)

	if got := data.NtfyToken.ValueString(); got != "tk_abc" {
		t.Errorf("ntfy_token = %q, want %q", got, "tk_abc")
	}
	if got := data.NtfyUsername.ValueString(); got != "alice" {
		t.Errorf("ntfy_username = %q, want %q", got, "alice")
	}
	if got := data.NtfyPassword.ValueString(); got != "s3cret" {
		t.Errorf("ntfy_password = %q, want %q", got, "s3cret")
	}
	var headers []string
	if d := data.WebhookHeaders.ElementsAs(ctx, &headers, false); d.HasError() {
		t.Fatalf("webhook_headers: %v", d.Errors())
	}
	if len(headers) != 2 || headers[0] != "X-Api-Key: k" || headers[1] != "Authorization: Bearer t" {
		t.Errorf("webhook_headers = %q", headers)
	}

	base := writeWire(t, b, &data)

	for key, want := range map[string]string{
		"NtfyToken":      `"tk_abc"`,
		"NtfyUsername":   `"alice"`,
		"NtfyPassword":   `"s3cret"`,
		"WebhookHeaders": `["X-Api-Key: k","Authorization: Bearer t"]`,
	} {
		if got := string(base[key]); got != want {
			t.Errorf("%s = %s, want %s", key, got, want)
		}
	}
}

func TestUnitOidcProviderRpInitiatedLogoutRoundTrip(t *testing.T) {
	ctx := t.Context()
	b := mustWire(t, securityPluginWire)
	data := readWire[JellyfinSecurityPluginConfigurationResourceModel](t, b, `{"OidcProviders":[{"Id":"idp","RpInitiatedLogoutEnabled":true,"RpInitiatedLogoutRedirectUri":"https://example.com/bye"}]}`)

	var providers []OidcProviderModel
	if d := data.OidcProviders.ElementsAs(ctx, &providers, false); d.HasError() {
		t.Fatalf("oidc_providers: %v", d.Errors())
	}
	if len(providers) != 1 || !providers[0].RpInitiatedLogoutEnabled.ValueBool() {
		t.Fatalf("oidc_providers = %+v, want one provider with rp_initiated_logout_enabled", providers)
	}
	if got := providers[0].RpInitiatedLogoutRedirectURI.ValueString(); got != "https://example.com/bye" {
		t.Errorf("rp_initiated_logout_redirect_uri = %q", got)
	}

	base := writeWire(t, b, &data)
	var written []map[string]json.RawMessage
	if err := json.Unmarshal(base["OidcProviders"], &written); err != nil {
		t.Fatalf("OidcProviders: %v", err)
	}
	if got := string(written[0]["RpInitiatedLogoutEnabled"]); got != "true" {
		t.Errorf("RpInitiatedLogoutEnabled = %s, want true", got)
	}
	if got := string(written[0]["RpInitiatedLogoutRedirectUri"]); got != `"https://example.com/bye"` {
		t.Errorf("RpInitiatedLogoutRedirectUri = %s", got)
	}
}

func TestUnitJellyfinSecurityPlugin263FieldsRoundTrip(t *testing.T) {
	ctx := t.Context()
	b := mustWire(t, securityPluginWire)
	fixture := `{"PairDeviceOnSecondScreenApproval":true,"PublicBaseUrl":"https://jf.example.com","OidcProviders":[{"Id":"idp","LinkExistingUsersByUsername":true}]}`

	data := readWire[JellyfinSecurityPluginConfigurationResourceModel](t, b, fixture)

	if !data.PairDeviceOnSecondScreenApproval.ValueBool() {
		t.Errorf("pair_device_on_second_screen_approval = %v, want true", data.PairDeviceOnSecondScreenApproval)
	}
	if got := data.PublicBaseURL.ValueString(); got != "https://jf.example.com" {
		t.Errorf("public_base_url = %q", got)
	}
	var providers []OidcProviderModel
	if d := data.OidcProviders.ElementsAs(ctx, &providers, false); d.HasError() {
		t.Fatalf("oidc_providers: %v", d.Errors())
	}
	if len(providers) != 1 || !providers[0].LinkExistingUsersByUsername.ValueBool() {
		t.Fatalf("oidc_providers = %+v, want one provider with link_existing_users_by_username", providers)
	}

	base := writeWire(t, b, &data)
	if got := string(base["PairDeviceOnSecondScreenApproval"]); got != "true" {
		t.Errorf("PairDeviceOnSecondScreenApproval = %s", got)
	}
	if got := string(base["PublicBaseUrl"]); got != `"https://jf.example.com"` {
		t.Errorf("PublicBaseUrl = %s", got)
	}
	var written []map[string]json.RawMessage
	if err := json.Unmarshal(base["OidcProviders"], &written); err != nil {
		t.Fatalf("OidcProviders: %v", err)
	}
	if len(written) != 1 || string(written[0]["LinkExistingUsersByUsername"]) != "true" {
		t.Errorf("OidcProviders = %s", base["OidcProviders"])
	}
}

func TestUnitJellyfinSecurityPlugin263FieldsAbsentStayNullAndUnwritten(t *testing.T) {
	b := mustWire(t, securityPluginWire)

	data := readWire[JellyfinSecurityPluginConfigurationResourceModel](t, b, `{"Enabled":true}`)

	if !data.PairDeviceOnSecondScreenApproval.IsNull() || !data.PublicBaseURL.IsNull() {
		t.Errorf("pair_device_on_second_screen_approval = %v, public_base_url = %v, want both null", data.PairDeviceOnSecondScreenApproval, data.PublicBaseURL)
	}

	base := writeWire(t, b, &data)
	for _, key := range []string{"PairDeviceOnSecondScreenApproval", "PublicBaseUrl"} {
		if raw, ok := base[key]; ok {
			t.Errorf("%s written as %s, want it left out", key, raw)
		}
	}
}

func TestUnitKeepSameInstant(t *testing.T) {
	cases := []struct {
		name   string
		prior  types.String
		served types.String
		want   types.String
	}{
		{"dotnet layout of the configured instant", types.StringValue("2030-01-01T00:00:00Z"), types.StringValue("2030-01-01T00:00:00.0000000Z"), types.StringValue("2030-01-01T00:00:00Z")},
		{"offset spelling of the same instant", types.StringValue("2030-01-01T02:00:00+02:00"), types.StringValue("2030-01-01T00:00:00.0000000Z"), types.StringValue("2030-01-01T02:00:00+02:00")},
		{"different instant", types.StringValue("2030-01-01T00:00:00Z"), types.StringValue("2031-01-01T00:00:00.0000000Z"), types.StringValue("2031-01-01T00:00:00.0000000Z")},
		{"nothing configured", types.StringNull(), types.StringValue("2030-01-01T00:00:00.0000000Z"), types.StringValue("2030-01-01T00:00:00.0000000Z")},
		{"cleared on the server", types.StringValue("2030-01-01T00:00:00Z"), types.StringNull(), types.StringNull()},
		{"unparseable prior", types.StringValue("soon"), types.StringValue("2030-01-01T00:00:00.0000000Z"), types.StringValue("2030-01-01T00:00:00.0000000Z")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := keepSameInstant(c.prior, c.served); !got.Equal(c.want) {
				t.Errorf("keepSameInstant(%v, %v) = %v, want %v", c.prior, c.served, got, c.want)
			}
		})
	}
}

func TestUnitSameInstantPlanModifier(t *testing.T) {
	ctx := t.Context()
	state := types.StringValue("2030-01-01T00:00:00.0000000Z")

	for _, c := range []struct {
		name   string
		config types.String
		want   types.String
	}{
		{"same instant plans the state value", types.StringValue("2030-01-01T00:00:00Z"), state},
		{"different instant plans the configured value", types.StringValue("2031-01-01T00:00:00Z"), types.StringValue("2031-01-01T00:00:00Z")},
	} {
		t.Run(c.name, func(t *testing.T) {
			req := planmodifier.StringRequest{ConfigValue: c.config, StateValue: state, PlanValue: c.config}
			resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
			sameInstantPlanModifier{}.PlanModifyString(ctx, req, resp)
			if !resp.PlanValue.Equal(c.want) {
				t.Errorf("plan = %v, want %v", resp.PlanValue, c.want)
			}
		})
	}
}

func TestUnitReduceSecurityPluginPayload(t *testing.T) {
	lines, err := reduceSecurityPluginPayload(`{"Enabled":true,"Port":587,"Name":"x","Empty":[],"Cidrs":["10.0.0.0/8"],"Deadline":null,"Providers":[{"Id":"a","Maps":[{"Role":"r"}]}]}`)
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}

	want := []string{
		"Cidrs: []string",
		"Deadline: null",
		"Empty: array",
		"Enabled: boolean",
		"Name: string",
		"Port: number",
		"Providers: []object",
		"Providers[].Id: string",
		"Providers[].Maps: []object",
		"Providers[].Maps[].Role: string",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected lines:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

func TestUnitFillEmptyPayloadLists(t *testing.T) {
	filled, paths, err := fillEmptyPayloadLists(`{"Enabled":true,"Entries":9007199254740993,"Empty":[],"Cidrs":["10.0.0.0/8"],"Ports":[587],"Smtp":{"Recipients":[]},"Providers":[{"Scopes":[],"Maps":[{"Role":"r","Libraries":[]}]}]}`)
	if err != nil {
		t.Fatalf("fill: %v", err)
	}

	if want := []string{"Empty", "Providers[].Maps[].Libraries", "Providers[].Scopes", "Smtp.Recipients"}; !slices.Equal(paths, want) {
		t.Errorf("filled paths = %v, want %v", paths, want)
	}
	want := `{"Cidrs":["10.0.0.0/8"],"Empty":["1"],"Enabled":true,"Entries":9007199254740993,"Ports":[587],"Providers":[{"Maps":[{"Libraries":["1"],"Role":"r"}],"Scopes":["1"]}],"Smtp":{"Recipients":["1"]}}`
	if filled != want {
		t.Errorf("filled payload:\n%s\nwant:\n%s", filled, want)
	}
}

func TestUnitJellyfinSecurityWriteKeepsTheServedShape(t *testing.T) {
	ctx := t.Context()
	b := mustWire(t, securityPluginWire)

	raw, err := os.ReadFile(securityPluginPayloadGolden)
	if err != nil {
		t.Fatalf("reading %s: %v", securityPluginPayloadGolden, err)
	}
	golden := strings.Split(strings.TrimSpace(string(raw)), "\n")

	payload, err := securityPluginPayloadFromShape(golden)
	if err != nil {
		t.Fatalf("building payload from golden: %v", err)
	}
	data := readWire[JellyfinSecurityPluginConfigurationResourceModel](t, b, payload)

	// Apply overlays the served configuration, as here, so a top-level key
	// stays in the payload whether or not an attribute claims it; the
	// bindings golden lists the unclaimed ones. What this checks is that each
	// rebuilt OIDC provider, role mapping and user email keeps every served
	// key, and that every value goes out as the JSON type the plugin serves.
	written, err := parseJSONObject(payload)
	if err != nil {
		t.Fatalf("parsing payload: %v", err)
	}
	if d := b.OverlayModel(ctx, written, &data); d.HasError() {
		t.Fatalf("overlay: %v", d.Errors())
	}
	payloadWritten, err := json.Marshal(written)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reduceSecurityPluginPayload(string(payloadWritten))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}

	for _, line := range linesNotIn(golden, got) {
		t.Errorf("served %q is not written back by the resource", line)
	}
	for _, line := range linesNotIn(got, golden) {
		t.Errorf("resource writes %q, which the plugin does not serve", line)
	}
}

// reduceSecurityPluginPayload flattens a plugin configuration payload into
// sorted "path: type" lines; elements of an array of objects share "Key[]".
func reduceSecurityPluginPayload(raw string) ([]string, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return nil, fmt.Errorf("parsing plugin configuration: %w", err)
	}

	var out []string
	reducePayloadObject(root, "", &out)
	slices.Sort(out)
	return slices.Compact(out), nil
}

func reducePayloadObject(obj map[string]any, prefix string, out *[]string) {
	for key, value := range obj {
		path := prefix + key
		switch v := value.(type) {
		case map[string]any:
			*out = append(*out, path+": object")
			reducePayloadObject(v, path+".", out)
		case []any:
			*out = append(*out, path+": "+payloadArrayType(v))
			for _, elem := range v {
				if m, ok := elem.(map[string]any); ok {
					reducePayloadObject(m, path+"[].", out)
				}
			}
		default:
			*out = append(*out, path+": "+payloadScalarType(v))
		}
	}
}

func payloadArrayType(v []any) string {
	if len(v) == 0 {
		return "array"
	}
	switch v[0].(type) {
	case map[string]any:
		return "[]object"
	case []any:
		return "[]array"
	default:
		return "[]" + payloadScalarType(v[0])
	}
}

// payloadListPlaceholder is the entry the probe writes into each list the
// plugin serves empty. Jellyfin reads a numeric string into a list of strings
// and into a list of numbers alike, so either comes back typed by its element.
const payloadListPlaceholder = "1"

// fillEmptyPayloadLists puts payloadListPlaceholder in every empty list and
// returns their paths. An empty list carries no element type, so without this
// a list's line in the golden would depend on whether the plugin's default for
// it happens to be empty. A list that already holds entries types itself.
func fillEmptyPayloadLists(raw string) (string, []string, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var root map[string]any
	if err := dec.Decode(&root); err != nil {
		return "", nil, fmt.Errorf("parsing plugin configuration: %w", err)
	}

	var filled []string
	fillEmptyObjectLists(root, "", &filled)
	slices.Sort(filled)
	out, err := json.Marshal(root)
	return string(out), filled, err
}

func fillEmptyObjectLists(obj map[string]any, prefix string, filled *[]string) {
	for key, value := range obj {
		path := prefix + key
		switch v := value.(type) {
		case map[string]any:
			fillEmptyObjectLists(v, path+".", filled)
		case []any:
			if len(v) == 0 {
				obj[key] = []any{payloadListPlaceholder}
				*filled = append(*filled, path)
				continue
			}
			for _, elem := range v {
				if m, ok := elem.(map[string]any); ok {
					fillEmptyObjectLists(m, path+"[].", filled)
				}
			}
		}
	}
}

func payloadScalarType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// securityPluginPayloadFromShape builds a payload holding one sample value for
// every line of a reduced payload, so each served key reaches flatten.
func securityPluginPayloadFromShape(lines []string) (string, error) {
	root := map[string]any{}
	for _, line := range lines {
		path, typ, ok := strings.Cut(line, ": ")
		if !ok {
			return "", fmt.Errorf("malformed line %q", line)
		}
		segments := strings.Split(path, ".")
		obj := root
		for _, segment := range segments[:len(segments)-1] {
			key, isArray := strings.CutSuffix(segment, "[]")
			if !isArray {
				next, ok := obj[key].(map[string]any)
				if !ok {
					next = map[string]any{}
					obj[key] = next
				}
				obj = next
				continue
			}
			elems, _ := obj[key].([]any)
			if len(elems) == 0 {
				elems = []any{map[string]any{}}
				obj[key] = elems
			}
			next, ok := elems[0].(map[string]any)
			if !ok {
				return "", fmt.Errorf("line %q nests under %s, which is not an array of objects", line, key)
			}
			obj = next
		}
		leaf := segments[len(segments)-1]
		if _, seen := obj[leaf]; !seen {
			obj[leaf] = samplePayloadValue(typ)
		}
	}
	out, err := json.Marshal(root)
	return string(out), err
}

func samplePayloadValue(typ string) any {
	switch typ {
	case "string":
		return "x"
	case "number":
		return 1
	case "boolean":
		return true
	case "object":
		return map[string]any{}
	case "array":
		return []any{}
	case "[]object":
		return []any{map[string]any{}}
	case "[]string", "[]number", "[]boolean":
		return []any{samplePayloadValue(strings.TrimPrefix(typ, "[]"))}
	default:
		return nil
	}
}

// securityPluginServer does not list the JellyfinSecurity plugin when version
// is "", and a POST replaces config with afterPost when that is set.
type securityPluginServer struct {
	version   string
	postFails bool
	afterPost string

	mu       sync.Mutex
	config   string
	requests []string
}

func (f *securityPluginServer) client(t *testing.T) *client.Client {
	t.Helper()
	configPath := "/Plugins/" + jellyfinSecurityPluginID + "/Configuration"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Plugins":
			plugins := []client.InstalledPlugin{}
			if f.version != "" {
				plugins = append(plugins, client.InstalledPlugin{ID: jellyfinSecurityPluginID, Name: "JellyfinSecurity", Version: f.version, Status: "Active"})
			}
			_ = json.NewEncoder(w).Encode(plugins)
		case r.Method == http.MethodGet && r.URL.Path == configPath:
			_, _ = io.WriteString(w, f.config)
		case r.Method == http.MethodPost && r.URL.Path == configPath:
			if f.postFails {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			f.config = cmp.Or(f.afterPost, string(body))
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return client.NewClient(srv.URL, "k")
}

func TestUnitSecurityPluginApplyChecksThePlugin(t *testing.T) {
	const newer = "999.0.0.0"
	cfg := "/Plugins/" + jellyfinSecurityPluginID + "/Configuration"
	written := []string{"GET /Plugins", "GET " + cfg, "POST " + cfg, "GET " + cfg}

	for _, test := range []struct {
		name         string
		version      string
		postFails    bool
		afterPost    string
		wantRequests []string
		wantErrors   []string
		wantWarnings []string
	}{
		{
			name:         "not installed",
			wantRequests: []string{"GET /Plugins"},
			wantErrors:   []string{"JellyfinSecurity plugin not installed"},
		},
		{
			name:         "at the supported version",
			version:      supportedSecurityPluginVersion(),
			wantRequests: written,
		},
		{
			name:         "newer than supported",
			version:      newer,
			wantRequests: written,
			wantWarnings: []string{"JellyfinSecurity plugin version newer than supported"},
		},
		{
			name:         "newer, and the write fails",
			version:      newer,
			postFails:    true,
			wantRequests: written[:3],
			wantErrors:   []string{"Failed to update JellyfinSecurity plugin configuration"},
		},
		{
			name:         "newer, and the write reads back unparsable",
			version:      newer,
			afterPost:    "not json",
			wantRequests: written,
			wantErrors:   []string{"Failed to parse the Jellyfin JellyfinSecurity"},
		},
	} {
		for _, op := range []string{"create", "update"} {
			t.Run(test.name+"/"+op, func(t *testing.T) {
				ctx := t.Context()
				srv := &securityPluginServer{version: test.version, postFails: test.postFails, afterPost: test.afterPost, config: `{"Enabled":true}`}
				r := &JellyfinSecurityPluginConfigurationResource{client: srv.client(t)}
				s := schemaOf(r)
				data := readWire[JellyfinSecurityPluginConfigurationResourceModel](t, mustWire(t, securityPluginWire), srv.config)
				data.PluginID = types.StringValue(jellyfinSecurityPluginID)
				plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
				if d := plan.Set(ctx, &data); d.HasError() {
					t.Fatal(d)
				}

				var diags diag.Diagnostics
				if op == "create" {
					resp := resource.CreateResponse{State: tfsdk.State(plan)}
					r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
					diags = resp.Diagnostics
				} else {
					resp := resource.UpdateResponse{State: tfsdk.State(plan)}
					r.Update(ctx, resource.UpdateRequest{Plan: plan, State: tfsdk.State(plan)}, &resp)
					diags = resp.Diagnostics
				}

				var errs, warnings []string
				for _, d := range diags {
					if d.Severity() == diag.SeverityError {
						errs = append(errs, d.Summary())
					} else {
						warnings = append(warnings, d.Summary())
					}
				}
				if !slices.Equal(errs, test.wantErrors) || !slices.Equal(warnings, test.wantWarnings) {
					t.Errorf("errors %q and warnings %q, want %q and %q", errs, warnings, test.wantErrors, test.wantWarnings)
				}
				if !slices.Equal(srv.requests, test.wantRequests) {
					t.Errorf("requests %q, want %q", srv.requests, test.wantRequests)
				}
			})
		}
	}
}

// Removing an OIDC provider must not plan the next one with the removed
// provider's values: its secret and settings come from the prior entry with
// its id, and a new entry takes the server's values.
func TestUnitSecurityPluginPlansOIDCProvidersByID(t *testing.T) {
	ctx := t.Context()
	s := schemaOf(&JellyfinSecurityPluginConfigurationResource{})
	providers, ok := s.Attributes["oidc_providers"].(rschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("oidc_providers is a %T", s.Attributes["oidc_providers"])
	}
	for name, a := range providers.NestedObject.Attributes {
		if hasPlanModifiers(a) {
			t.Errorf("oidc_providers.%s pairs list elements by index with a plan modifier of its own", name)
		}
	}
	elemType, ok := providers.NestedObject.Type().(types.ObjectType)
	if !ok {
		t.Fatalf("oidc_providers elements are %T", providers.NestedObject.Type())
	}
	// element returns an entry whose attributes hold base (nil for null, or
	// tftypes.UnknownValue), except those set holds.
	element := func(base any, set map[string]attr.Value) attr.Value {
		attrs := map[string]attr.Value{}
		for name, typ := range elemType.AttrTypes {
			v, err := typ.ValueFromTerraform(ctx, tftypes.NewValue(typ.TerraformType(ctx), base))
			if err != nil {
				t.Fatal(err)
			}
			attrs[name] = v
		}
		maps.Copy(attrs, set)
		return types.ObjectValueMust(elemType.AttrTypes, attrs)
	}
	mappingType, ok := elemType.AttrTypes["role_library_mappings"].(types.ListType)
	if !ok {
		t.Fatalf("role_library_mappings is a %s", elemType.AttrTypes["role_library_mappings"])
	}
	mappingObjType, ok := mappingType.ElemType.(types.ObjectType)
	if !ok {
		t.Fatalf("role_library_mappings elements are %s", mappingType.ElemType)
	}
	mapping := func(role string, libraryIDs attr.Value) attr.Value {
		return types.ObjectValueMust(mappingObjType.AttrTypes, map[string]attr.Value{
			"role":        types.StringValue(role),
			"library_ids": libraryIDs,
		})
	}
	mappings := func(entries ...attr.Value) attr.Value {
		return types.ListValueMust(mappingObjType, entries)
	}
	libraries := func(id string) attr.Value {
		return types.ListValueMust(types.StringType, []attr.Value{types.StringValue(id)})
	}
	prior := func(id, secret string, autoCreate bool, createdAt string, roleMappings attr.Value) attr.Value {
		return element(nil, map[string]attr.Value{
			"id":                    types.StringValue(id),
			"client_secret":         types.StringValue(secret),
			"auto_create_users":     types.BoolValue(autoCreate),
			"created_at":            types.StringValue(createdAt),
			"role_library_mappings": roleMappings,
		})
	}
	// b moves to the index a held and swaps its role mappings, leaving their
	// libraries unset.
	configured := map[string]attr.Value{
		"id":                    types.StringValue("b"),
		"display_name":          types.StringValue("B"),
		"role_library_mappings": mappings(mapping("adults", types.ListNull(types.StringType)), mapping("kids", types.ListNull(types.StringType))),
	}
	planned := maps.Clone(configured)
	planned["role_library_mappings"] = mappings(mapping("adults", types.ListUnknown(types.StringType)), mapping("kids", types.ListUnknown(types.StringType)))

	state := types.ListValueMust(elemType, []attr.Value{
		prior("a", "secret-a", true, "tA", mappings(mapping("adults", libraries("a-adults")), mapping("kids", libraries("a-kids")))),
		prior("b", "secret-b", false, "tB", mappings(mapping("kids", libraries("b-kids")), mapping("adults", libraries("b-adults")))),
	})
	config := types.ListValueMust(elemType, []attr.Value{element(nil, configured)})
	plan := types.ListValueMust(elemType, []attr.Value{element(tftypes.UnknownValue, planned)})

	existing := tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{})
	resp := planmodifier.ListResponse{PlanValue: plan}
	for _, m := range providers.PlanModifiers {
		m.PlanModifyList(ctx, planmodifier.ListRequest{
			Path:        path.Root("oidc_providers"),
			State:       tfsdk.State{Raw: existing},
			Plan:        tfsdk.Plan{Raw: existing},
			ConfigValue: config,
			PlanValue:   resp.PlanValue,
			StateValue:  state,
		}, &resp)
	}
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	got, ok := resp.PlanValue.Elements()[0].(types.Object)
	if !ok {
		t.Fatalf("planned %s", resp.PlanValue)
	}
	for name, want := range map[string]attr.Value{
		"client_secret":         types.StringValue("secret-b"),
		"auto_create_users":     types.BoolValue(false),
		"created_at":            types.StringValue("tB"),
		"display_name":          types.StringValue("B"),
		"role_library_mappings": mappings(mapping("adults", libraries("b-adults")), mapping("kids", libraries("b-kids"))),
	} {
		if v := got.Attributes()[name]; !v.Equal(want) {
			t.Errorf("planned oidc_providers[0].%s = %s, want %s", name, v, want)
		}
	}
}

func hasPlanModifiers(a rschema.Attribute) bool {
	switch a := a.(type) {
	case rschema.StringAttribute:
		return len(a.PlanModifiers) > 0
	case rschema.BoolAttribute:
		return len(a.PlanModifiers) > 0
	case rschema.ListAttribute:
		return len(a.PlanModifiers) > 0
	case rschema.ListNestedAttribute:
		if len(a.PlanModifiers) > 0 {
			return true
		}
		for _, n := range a.NestedObject.Attributes {
			if hasPlanModifiers(n) {
				return true
			}
		}
	}
	return false
}

// An empty enrollment_deadline clears the deadline: the plugin reads the
// empty string as none and leaves the key out, which reads back as empty.
func TestUnitSecurityPluginEnrollmentDeadlineClears(t *testing.T) {
	ctx := t.Context()
	b := mustWire(t, securityPluginWire)
	data := readWire[JellyfinSecurityPluginConfigurationResourceModel](t, b, `{"EnrollmentDeadline":"2030-01-01T00:00:00Z"}`)
	data.EnrollmentDeadline = types.StringValue("")
	doc := map[string]json.RawMessage{"EnrollmentDeadline": json.RawMessage(`"2030-01-01T00:00:00Z"`)}
	if d := b.OverlayModel(ctx, doc, &data); d.HasError() {
		t.Fatal(d)
	}
	if got := string(doc["EnrollmentDeadline"]); got != `""` {
		t.Errorf("wrote EnrollmentDeadline %s, want the empty string that clears it", got)
	}
	if got := readWire[JellyfinSecurityPluginConfigurationResourceModel](t, b, `{}`).EnrollmentDeadline; !got.Equal(types.StringValue("")) {
		t.Errorf("no deadline reads as %s, want the empty string", got)
	}

	a, ok := schemaOf(&JellyfinSecurityPluginConfigurationResource{}).Attributes["enrollment_deadline"].(rschema.StringAttribute)
	if !ok {
		t.Fatal("enrollment_deadline is not a string attribute")
	}
	testUnitAssertStringValidation(t, a, map[string]bool{"": false, "2030-01-01T00:00:00Z": false, "tomorrow": true})

	// State from releases that read no deadline as null plans it unknown,
	// as apply reads it as "".
	resp := planmodifier.StringResponse{PlanValue: types.StringUnknown()}
	for _, m := range a.PlanModifiers {
		m.PlanModifyString(ctx, planmodifier.StringRequest{
			Path:        path.Root("enrollment_deadline"),
			State:       tfsdk.State{Raw: tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{})},
			ConfigValue: types.StringNull(),
			PlanValue:   resp.PlanValue,
			StateValue:  types.StringNull(),
		}, &resp)
	}
	if !resp.PlanValue.IsUnknown() {
		t.Errorf("a null prior deadline plans %s, want unknown", resp.PlanValue)
	}
}

// A list the plugin stores joined into one string must not hold values that
// the join would change.
func TestUnitSecurityPluginDelimitedListsRejectWhatTheJoinChanges(t *testing.T) {
	ctx := t.Context()
	s := schemaOf(&JellyfinSecurityPluginConfigurationResource{})
	providers, ok := s.Attributes["oidc_providers"].(rschema.ListNestedAttribute)
	if !ok {
		t.Fatal("oidc_providers is not a nested list")
	}
	for _, test := range []struct {
		attr   string
		values []string
		want   bool
	}{
		{"scopes", []string{"openid", "profile"}, false},
		{"scopes", []string{"openid profile"}, true},
		{"scopes", []string{""}, true},
		{"allowed_groups", []string{"a b", "c"}, false},
		{"allowed_groups", []string{"a,b"}, true},
	} {
		a, ok := providers.NestedObject.Attributes[test.attr].(rschema.ListAttribute)
		if !ok {
			t.Fatalf("%s is not a list", test.attr)
		}
		elems := make([]attr.Value, len(test.values))
		for i, v := range test.values {
			elems[i] = types.StringValue(v)
		}
		resp := validator.ListResponse{}
		for _, v := range a.Validators {
			v.ValidateList(ctx, validator.ListRequest{Path: path.Root(test.attr), ConfigValue: types.ListValueMust(types.StringType, elems)}, &resp)
		}
		if resp.Diagnostics.HasError() != test.want {
			t.Errorf("%s = %q: error %t, want %t (%v)", test.attr, test.values, resp.Diagnostics.HasError(), test.want, resp.Diagnostics)
		}
	}
}

// OidcProviderModel reads an element of oidc_providers, which the resource
// itself handles through its binding alone.
type OidcProviderModel struct {
	ID                           types.String `tfsdk:"id"`
	DisplayName                  types.String `tfsdk:"display_name"`
	Preset                       types.String `tfsdk:"preset"`
	DiscoveryURL                 types.String `tfsdk:"discovery_url"`
	ClientID                     types.String `tfsdk:"client_id"`
	ClientSecret                 types.String `tfsdk:"client_secret"`
	Scopes                       types.List   `tfsdk:"scopes"`
	AcrValues                    types.List   `tfsdk:"acr_values"`
	UsernameClaim                types.String `tfsdk:"username_claim"`
	AllowedGroups                types.List   `tfsdk:"allowed_groups"`
	AdminGroups                  types.List   `tfsdk:"admin_groups"`
	AllowAdminGroupElevation     types.Bool   `tfsdk:"allow_admin_group_elevation"`
	TemplateUserID               types.String `tfsdk:"template_user_id"`
	AutoCreateUsers              types.Bool   `tfsdk:"auto_create_users"`
	LinkExistingUsersByUsername  types.Bool   `tfsdk:"link_existing_users_by_username"`
	RequireIdpMfa                types.Bool   `tfsdk:"require_idp_mfa"`
	BypassPluginTwoFa            types.Bool   `tfsdk:"bypass_plugin_two_fa"`
	Enabled                      types.Bool   `tfsdk:"enabled"`
	ShowLoginButton              types.Bool   `tfsdk:"show_login_button"`
	ForceHTTPS                   types.Bool   `tfsdk:"force_https"`
	AllowPrivateNetworks         types.Bool   `tfsdk:"allow_private_networks"`
	AdditionalAllowedCidrs       types.List   `tfsdk:"additional_allowed_cidrs"`
	SyncProfilePicture           types.Bool   `tfsdk:"sync_profile_picture"`
	PictureClaim                 types.String `tfsdk:"picture_claim"`
	PromptSelectAccount          types.Bool   `tfsdk:"prompt_select_account"`
	OmitPromptLogin              types.Bool   `tfsdk:"omit_prompt_login"`
	ApplyRoleLibraryAccess       types.Bool   `tfsdk:"apply_role_library_access"`
	RoleLibraryMappings          types.List   `tfsdk:"role_library_mappings"`
	EmailClaim                   types.String `tfsdk:"email_claim"`
	SyncEmailFromClaim           types.Bool   `tfsdk:"sync_email_from_claim"`
	ButtonText                   types.String `tfsdk:"button_text"`
	ButtonIconURL                types.String `tfsdk:"button_icon_url"`
	ForcePasswordSetup           types.Bool   `tfsdk:"force_password_setup"`
	RpInitiatedLogoutEnabled     types.Bool   `tfsdk:"rp_initiated_logout_enabled"`
	RpInitiatedLogoutRedirectURI types.String `tfsdk:"rp_initiated_logout_redirect_uri"`
	CreatedAt                    types.String `tfsdk:"created_at"`
}
