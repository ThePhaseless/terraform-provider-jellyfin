// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

func TestUnitJellyfinSecurityNotificationAuthRoundTrip(t *testing.T) {
	ctx := context.Background()
	b := testUnitSecurityPluginWire(t)
	fixture := `{"NtfyToken":"tk_abc","NtfyUsername":"alice","NtfyPassword":"s3cret","WebhookHeaders":["X-Api-Key: k","Authorization: Bearer t"]}`

	data := testUnitSecurityPluginRead(t, b, fixture)

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

	base := map[string]json.RawMessage{}
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("overlay: %v", d.Errors())
	}

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
	ctx := context.Background()
	b := testUnitSecurityPluginWire(t)
	data := testUnitSecurityPluginRead(t, b, `{"OidcProviders":[{"Id":"idp","RpInitiatedLogoutEnabled":true,"RpInitiatedLogoutRedirectUri":"https://example.com/bye"}]}`)

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

	base := map[string]json.RawMessage{}
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("overlay: %v", d.Errors())
	}
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
	ctx := context.Background()
	b := testUnitSecurityPluginWire(t)
	fixture := `{"PairDeviceOnSecondScreenApproval":true,"PublicBaseUrl":"https://jf.example.com","OidcProviders":[{"Id":"idp","LinkExistingUsersByUsername":true}]}`

	data := testUnitSecurityPluginRead(t, b, fixture)

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

	base := map[string]json.RawMessage{}
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("overlay: %v", d.Errors())
	}
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
	ctx := context.Background()
	b := testUnitSecurityPluginWire(t)

	data := testUnitSecurityPluginRead(t, b, `{"Enabled":true}`)

	if !data.PairDeviceOnSecondScreenApproval.IsNull() || !data.PublicBaseURL.IsNull() {
		t.Errorf("pair_device_on_second_screen_approval = %v, public_base_url = %v, want both null", data.PairDeviceOnSecondScreenApproval, data.PublicBaseURL)
	}

	base := map[string]json.RawMessage{}
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("overlay: %v", d.Errors())
	}
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
	ctx := context.Background()
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

func TestUnitJellyfinSecurityWritesBackExactlyTheServedKeys(t *testing.T) {
	ctx := context.Background()
	b := testUnitSecurityPluginWire(t)

	raw, err := os.ReadFile(securityPluginPayloadGolden)
	if err != nil {
		t.Fatalf("reading %s: %v", securityPluginPayloadGolden, err)
	}
	golden := strings.Split(strings.TrimSpace(string(raw)), "\n")

	payload, err := securityPluginPayloadFromShape(golden)
	if err != nil {
		t.Fatalf("building payload from golden: %v", err)
	}
	data := testUnitSecurityPluginRead(t, b, payload)

	// Apply writes over the served configuration, as here: the keys no
	// attribute claims go back as served, and each OIDC provider, user email
	// and role mapping is rebuilt from its attributes and the carried
	// CreatedAt.
	written, err := parseJSONObject(payload)
	if err != nil {
		t.Fatalf("parsing payload: %v", err)
	}
	if d := b.OverlayModel(ctx, written, &data); d.HasError() {
		t.Fatalf("overlay: %v", d.Errors())
	}
	got, err := reduceSecurityPluginPayload(string(mustJSON(written)))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}

	gotSet := map[string]bool{}
	for _, line := range got {
		gotSet[line] = true
	}
	goldenSet := map[string]bool{}
	for _, line := range golden {
		goldenSet[line] = true
		if !gotSet[line] {
			t.Errorf("served %q is not written back by the resource", line)
		}
	}
	for _, line := range got {
		if !goldenSet[line] {
			t.Errorf("resource writes %q, which the plugin does not serve", line)
		}
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
	sort.Strings(out)
	return dedupStrings(out), nil
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
	sort.Strings(filled)
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

func testUnitSecurityPluginWire(t *testing.T) *wire.Binding {
	t.Helper()
	b, err := securityPluginWire()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// testUnitSecurityPluginRead reads raw with no prior value to compare with.
func testUnitSecurityPluginRead(t *testing.T, b *wire.Binding, raw string) JellyfinSecurityPluginConfigurationResourceModel {
	t.Helper()
	ctx := context.Background()
	doc, err := parseJSONObject(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	obj, d := b.Flatten(ctx, doc, types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatalf("read: %v", d.Errors())
	}
	var data JellyfinSecurityPluginConfigurationResourceModel
	if d := obj.As(ctx, &data, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatalf("model: %v", d.Errors())
	}
	return data
}
