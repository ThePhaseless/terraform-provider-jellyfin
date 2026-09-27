// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

// These tests compare each binding with the hand-written mapping it replaces,
// and go away with the last of them. They run on payloads synthesized from
// the goldens, with a distinct value for every key, and on payloads captured
// from Jellyfin 12.1, 10.11.11 and 10.9.11.

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

type wireDiffCase struct {
	name     string
	resource resource.Resource
	// document names the Document view the resource writes, if any.
	document string
	// object is the golden object the payloads are synthesized from.
	object string
	// captured lists files under testdata/wire_diff/<version>/.
	captured []string
	// extract takes the document out of a captured file.
	extract func(raw []byte) (string, bool)
	// extra adds keys the goldens lack to a synthesized payload.
	extra func(payload map[string]any)
	// servedBase is false for a document written from nothing.
	servedBase bool
	// priorTweak derives one more prior value to flatten against.
	priorTweak func(ctx context.Context, prior types.Object) types.Object
	// accepted lists attribute paths where the old and new reads may
	// differ, with the reason.
	accepted   map[string]string
	oldFlatten func(ctx context.Context, raw string, prior types.Object) (types.Object, diag.Diagnostics)
	oldOverlay func(ctx context.Context, doc map[string]json.RawMessage, obj types.Object) diag.Diagnostics
}

// modelOf reads the named nested objects as null when unknown, as the
// resources do, since a pointer field of their models cannot hold unknown.
func modelOf[T any](ctx context.Context, obj types.Object, unknownToNull ...string) (T, diag.Diagnostics) {
	var m T
	for _, name := range unknownToNull {
		if v, ok := obj.Attributes()[name]; ok && v.IsUnknown() {
			obj = withAttr(ctx, obj, name, nullValue(ctx, v.Type(ctx)))
		}
	}
	d := obj.As(ctx, &m, basetypes.ObjectAsOptions{})
	return m, d
}

func objectOf(ctx context.Context, attrTypes map[string]attr.Type, model any) (types.Object, diag.Diagnostics) {
	return types.ObjectValueFrom(ctx, attrTypes, model)
}

func withAttr(ctx context.Context, obj types.Object, name string, v attr.Value) types.Object {
	attrs := map[string]attr.Value{}
	for k, old := range obj.Attributes() {
		attrs[k] = old
	}
	attrs[name] = v
	out, _ := types.ObjectValue(obj.AttributeTypes(ctx), attrs)
	return out
}

func nullValue(ctx context.Context, t attr.Type) attr.Value {
	v, _ := t.ValueFromTerraform(ctx, tftypes.NewValue(t.TerraformType(ctx), nil))
	return v
}

func unknownValue(ctx context.Context, t attr.Type) attr.Value {
	v, _ := t.ValueFromTerraform(ctx, tftypes.NewValue(t.TerraformType(ctx), tftypes.UnknownValue))
	return v
}

func flatAdapter[T any](flatten func(ctx context.Context, raw string, m *T, diags *diag.Diagnostics)) func(context.Context, string, types.Object) (types.Object, diag.Diagnostics) {
	return func(ctx context.Context, raw string, prior types.Object) (types.Object, diag.Diagnostics) {
		m, diags := modelOf[T](ctx, prior)
		if diags.HasError() {
			return prior, diags
		}
		flatten(ctx, raw, &m, &diags)
		obj, d := objectOf(ctx, prior.AttributeTypes(ctx), &m)
		return obj, append(diags, d...)
	}
}

func overlayAdapter[T any](overlay func(ctx context.Context, doc map[string]json.RawMessage, m *T) diag.Diagnostics) func(context.Context, map[string]json.RawMessage, types.Object) diag.Diagnostics {
	return func(ctx context.Context, doc map[string]json.RawMessage, obj types.Object) diag.Diagnostics {
		m, diags := modelOf[T](ctx, obj)
		if diags.HasError() {
			return diags
		}
		return overlay(ctx, doc, &m)
	}
}

func readVirtualFolderOptions(raw []byte) (string, bool) {
	var folders []map[string]json.RawMessage
	if json.Unmarshal(raw, &folders) != nil || len(folders) == 0 {
		return "", false
	}
	opts, ok := folders[0]["LibraryOptions"]
	return string(opts), ok
}

func wireDiffCases() []wireDiffCase {
	return []wireDiffCase{
		{
			name: "user_policy", resource: NewUserResource(), document: "Policy", object: "UserPolicy",
			captured: []string{"user_policy.json"}, servedBase: true,
			accepted: map[string]string{
				"policy.access_schedules": "the old read turned a JSON null AccessSchedules into an empty list, because it never checked for null; every captured policy serves []",
			},
			oldFlatten: func(ctx context.Context, raw string, prior types.Object) (types.Object, diag.Diagnostics) {
				m, diags := modelOf[UserResourceModel](ctx, prior, "policy")
				if diags.HasError() {
					return prior, diags
				}
				m.Policy = policyFromRaw(ctx, raw, &diags)
				// The resource reads the three flags from the typed user,
				// whose Policy is this same document.
				var flags client.UserPolicy
				_ = json.Unmarshal([]byte(raw), &flags)
				m.IsAdministrator = types.BoolValue(flags.IsAdministrator)
				m.IsDisabled = types.BoolValue(flags.IsDisabled)
				m.EnableAllFolders = types.BoolValue(flags.EnableAllFolders)
				obj, d := objectOf(ctx, prior.AttributeTypes(ctx), &m)
				return obj, append(diags, d...)
			},
			oldOverlay: func(ctx context.Context, doc map[string]json.RawMessage, obj types.Object) diag.Diagnostics {
				m, diags := modelOf[UserResourceModel](ctx, obj, "policy")
				if diags.HasError() {
					return diags
				}
				putJSONBool(doc, "IsAdministrator", m.IsAdministrator)
				putJSONBool(doc, "IsDisabled", m.IsDisabled)
				putJSONBool(doc, "EnableAllFolders", m.EnableAllFolders)
				if m.Policy != nil {
					return overlayPolicyIntoJSON(ctx, doc, m.Policy)
				}
				return nil
			},
		},
		{
			name: "library_options", resource: NewLibraryResource(), document: "LibraryOptions", object: "LibraryOptions",
			captured: []string{"virtual_folders.json"}, extract: readVirtualFolderOptions, servedBase: true,
			extra: func(payload map[string]any) {
				infos, _ := payload["PathInfos"].([]any)
				for i, e := range infos {
					if info, ok := e.(map[string]any); ok {
						info["NetworkPath"] = fmt.Sprintf("//nas/share%d", i)
					}
				}
			},
			oldFlatten: func(ctx context.Context, raw string, prior types.Object) (types.Object, diag.Diagnostics) {
				m, diags := modelOf[LibraryResourceModel](ctx, prior, "library_options")
				if diags.HasError() {
					return prior, diags
				}
				m.LibraryOptions = flattenLibraryOptions(ctx, raw, &diags)
				obj, d := objectOf(ctx, prior.AttributeTypes(ctx), &m)
				return obj, append(diags, d...)
			},
			oldOverlay: func(ctx context.Context, doc map[string]json.RawMessage, obj types.Object) diag.Diagnostics {
				m, diags := modelOf[LibraryResourceModel](ctx, obj, "library_options")
				if diags.HasError() {
					return diags
				}
				return overlayLibraryOptions(ctx, doc, m.LibraryOptions)
			},
		},
		{
			name: "security_plugin", resource: NewJellyfinSecurityPluginConfigurationResource(), object: wire.SecurityPluginRoot,
			servedBase: true,
			extra: func(payload map[string]any) {
				payload["EnrollmentDeadline"] = "2030-01-01T00:00:00.0000000Z"
			},
			priorTweak: func(ctx context.Context, prior types.Object) types.Object {
				return withAttr(ctx, prior, "enrollment_deadline", types.StringValue("2030-01-01T00:00:00Z"))
			},
			oldFlatten: flatAdapter(flattenJellyfinSecurity),
			oldOverlay: overlayAdapter(overlayJellyfinSecurity),
		},
	}
}

func findWireDiffCase(t *testing.T, name string) wireDiffCase {
	t.Helper()
	for _, c := range wireDiffCases() {
		if c.name == name {
			return c
		}
	}
	t.Fatalf("no differential case %s", name)
	return wireDiffCase{}
}

func (c wireDiffCase) binding(t *testing.T) *wire.Binding {
	t.Helper()
	bind := pendingWireMigration["jellyfin_"+c.resourceName()]
	if bound, ok := c.resource.(wireBound); ok {
		bind = bound.Wire
	}
	root, err := bind()
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	if c.document == "" {
		return root
	}
	b, err := root.Document(c.document)
	if err != nil {
		t.Fatalf("document: %v", err)
	}
	return b
}

func (c wireDiffCase) resourceName() string {
	var meta resource.MetadataResponse
	c.resource.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jellyfin"}, &meta)
	return strings.TrimPrefix(meta.TypeName, "jellyfin_")
}

func fnvInt(s string) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return int64(h.Sum32() % 100000)
}

// synthesize gives every property of object a value of its type, distinct
// per key and element, with two elements in every list. Strings hold a comma
// and a space so that a delimited list splits them.
func synthesize(ix wire.Index, object, seed string, depth int) map[string]any {
	out := map[string]any{}
	for key, p := range ix[object] {
		one := func(i int) any {
			s := fmt.Sprintf("%s/%s.%s/%d", seed, object, key, i)
			if p.Ref != "" {
				if depth >= 4 {
					return map[string]any{}
				}
				return synthesize(ix, p.Ref, s, depth+1)
			}
			n := fnvInt(s)
			switch p.Scalar {
			case "boolean":
				return n%2 == 0
			case "integer":
				return n
			case "number":
				if p.Format == "" {
					return n
				}
				return float64(n) + 0.5
			default:
				return fmt.Sprintf("v%d,x y", n)
			}
		}
		if p.List {
			out[key] = []any{one(0), one(1)}
		} else {
			out[key] = one(0)
		}
	}
	return out
}

// mapLeaves rewrites every scalar and scalar list of payload with leaf, and
// drops the keys drop picks, at every depth.
func mapLeaves(v any, leaf func(any) any, drop func(i int) bool) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := map[string]any{}
		for i, k := range keys {
			if drop != nil && drop(i) {
				continue
			}
			out[k] = mapLeaves(x[k], leaf, drop)
		}
		return out
	case []any:
		if len(x) > 0 {
			if _, isObject := x[0].(map[string]any); isObject {
				out := make([]any, len(x))
				for i, e := range x {
					out[i] = mapLeaves(e, leaf, drop)
				}
				return out
			}
		}
		return leaf(x)
	default:
		return leaf(x)
	}
}

type namedPayload struct {
	name string
	raw  string
}

func (c wireDiffCase) payloads(t *testing.T) []namedPayload {
	t.Helper()
	full := synthesize(wire.Pinned(), c.object, c.name, 0)
	if c.extra != nil {
		c.extra(full)
	}
	nulls := map[string]any{}
	for k := range full {
		nulls[k] = nil
	}
	variants := map[string]any{
		"synthesized":             full,
		"synthesized-nulls":       nulls,
		"synthesized-null-leaves": mapLeaves(full, func(any) any { return nil }, nil),
		"synthesized-sparse":      mapLeaves(full, func(v any) any { return v }, func(i int) bool { return i%2 == 1 }),
		"synthesized-empties": mapLeaves(full, func(v any) any {
			switch v.(type) {
			case []any, nil:
				return []any{}
			case string:
				return ""
			}
			return v
		}, nil),
	}
	var out []namedPayload
	for _, name := range []string{"synthesized", "synthesized-nulls", "synthesized-null-leaves", "synthesized-sparse", "synthesized-empties"} {
		raw, err := json.Marshal(variants[name])
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, namedPayload{name, string(raw)})
	}
	if c.name == "security_plugin" {
		raw, err := os.ReadFile(securityPluginPayloadGolden)
		if err != nil {
			t.Fatal(err)
		}
		shaped, err := securityPluginPayloadFromShape(strings.Split(strings.TrimSpace(string(raw)), "\n"))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, namedPayload{"golden-shaped", shaped})
	}
	for _, version := range []string{"12.1", "10.11.11", "10.9.11"} {
		for _, file := range c.captured {
			raw, err := os.ReadFile(filepath.Join("testdata", "wire_diff", version, file))
			if err != nil {
				continue
			}
			doc := string(raw)
			if c.extract != nil {
				var ok bool
				if doc, ok = c.extract(raw); !ok {
					t.Fatalf("%s/%s holds no %s document", version, file, c.object)
				}
			}
			out = append(out, namedPayload{version + "/" + file, doc})
		}
	}
	return out
}

func requiredPaths(attrs map[string]schema.Attribute, prefix string, out map[string]bool) {
	for name, a := range attrs {
		p := name
		if prefix != "" {
			p = prefix + "." + name
		}
		if a.IsRequired() {
			out[p] = true
		}
		switch n := a.(type) {
		case schema.ListNestedAttribute:
			requiredPaths(n.NestedObject.Attributes, p, out)
		case schema.SingleNestedAttribute:
			requiredPaths(n.Attributes, p, out)
		}
	}
}

// vary sets attributes of obj null or unknown as pick says for each path, at
// every depth, leaving required ones alone.
func vary(ctx context.Context, obj types.Object, required map[string]bool, prefix string, pick func(p string, v attr.Value) (null, unknown bool)) types.Object {
	if obj.IsNull() || obj.IsUnknown() {
		return obj
	}
	attrs := map[string]attr.Value{}
	for name, v := range obj.Attributes() {
		p := name
		if prefix != "" {
			p = prefix + "." + name
		}
		null, unknown := pick(p, v)
		switch {
		case required[withoutIndexes(p)]:
		case null:
			v = nullValue(ctx, v.Type(ctx))
		case unknown:
			v = unknownValue(ctx, v.Type(ctx))
		}
		switch x := v.(type) {
		case basetypes.ObjectValue:
			v = vary(ctx, x, required, p, pick)
		case basetypes.ListValue:
			if et, ok := x.ElementType(ctx).(basetypes.ObjectType); ok && !x.IsNull() && !x.IsUnknown() {
				elems := make([]attr.Value, len(x.Elements()))
				for j, e := range x.Elements() {
					elems[j] = e
					if o, ok := e.(basetypes.ObjectValue); ok {
						elems[j] = vary(ctx, o, required, fmt.Sprintf("%s[%d]", p, j), pick)
					}
				}
				v, _ = types.ListValue(et, elems)
			}
		}
		attrs[name] = v
	}
	out, _ := types.ObjectValue(obj.AttributeTypes(ctx), attrs)
	return out
}

func isContainer(ctx context.Context, v attr.Value) bool {
	switch x := v.(type) {
	case basetypes.ObjectValue:
		return true
	case basetypes.ListValue:
		_, ok := x.ElementType(ctx).(basetypes.ObjectType)
		return ok
	}
	return false
}

func valueDiffs(ctx context.Context, a, b attr.Value, at string, out *[]string) {
	if a.Equal(b) {
		return
	}
	ao, ok1 := a.(basetypes.ObjectValue)
	bo, ok2 := b.(basetypes.ObjectValue)
	if ok1 && ok2 && !ao.IsNull() && !bo.IsNull() && !ao.IsUnknown() && !bo.IsUnknown() {
		for name, av := range ao.Attributes() {
			valueDiffs(ctx, av, bo.Attributes()[name], strings.TrimPrefix(at+"."+name, "."), out)
		}
		return
	}
	al, ok1 := a.(basetypes.ListValue)
	bl, ok2 := b.(basetypes.ListValue)
	if ok1 && ok2 && len(al.Elements()) == len(bl.Elements()) && len(al.Elements()) > 0 {
		if _, isObject := al.ElementType(ctx).(basetypes.ObjectType); isObject {
			for i := range al.Elements() {
				valueDiffs(ctx, al.Elements()[i], bl.Elements()[i], fmt.Sprintf("%s[%d]", at, i), out)
			}
			return
		}
	}
	*out = append(*out, fmt.Sprintf("%s: old %s, new %s", at, a, b))
}

func withoutIndexes(p string) string {
	var b strings.Builder
	skip := false
	for _, r := range p {
		switch {
		case r == '[':
			skip = true
		case r == ']':
			skip = false
		case !skip:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func canonicalJSON(t *testing.T, doc map[string]json.RawMessage) any {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func parseDoc(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	doc, err := parseJSONObject(raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil {
		doc = map[string]json.RawMessage{}
	}
	return doc
}

func TestUnitWireReadsAndWritesLikeTheHandWrittenMappings(t *testing.T) {
	ctx := context.Background()
	for _, c := range wireDiffCases() {
		t.Run(c.name, func(t *testing.T) {
			b := c.binding(t)
			s := schemaOf(c.resource)
			attrTypes := b.AttrTypes
			required := map[string]bool{}
			requiredPaths(s.Attributes, "", required)

			identity := map[string]attr.Value{}
			for name, at := range attrTypes {
				identity[name] = nullValue(ctx, at)
			}
			for _, name := range []string{"id", "task_id", "plugin_id", "name"} {
				if _, ok := identity[name]; ok {
					identity[name] = types.StringValue("prior-" + name)
				}
			}
			nullPrior, _ := types.ObjectValue(attrTypes, identity)

			payloads := c.payloads(t)
			fullModel, d := c.oldFlatten(ctx, payloads[0].raw, nullPrior)
			if d.HasError() {
				t.Fatalf("old read of the synthesized payload: %v", d)
			}
			priors := []types.Object{nullPrior, fullModel}
			if c.priorTweak != nil {
				priors = append(priors, c.priorTweak(ctx, fullModel))
			}

			reads, writes := 0, 0
			for _, p := range payloads {
				for pi, prior := range priors {
					reads++
					oldObj, oldDiags := c.oldFlatten(ctx, p.raw, prior)
					newObj, newDiags := b.Flatten(ctx, parseDoc(t, p.raw), prior)
					if oldDiags.HasError() != newDiags.HasError() {
						t.Errorf("read %s (prior %d): old errors %v, new errors %v", p.name, pi, oldDiags, newDiags)
						continue
					}
					var diffs []string
					valueDiffs(ctx, oldObj, newObj, "", &diffs)
					for _, diff := range diffs {
						at, _, _ := strings.Cut(diff, ":")
						if reason, ok := c.accepted[withoutIndexes(at)]; ok {
							t.Logf("read %s: accepted difference at %s: %s", p.name, at, reason)
							continue
						}
						t.Errorf("read %s (prior %d): %s", p.name, pi, diff)
					}
				}
			}

			models := map[string]types.Object{
				"full": fullModel,
				"halfnull-even": vary(ctx, fullModel, required, "", func(p string, _ attr.Value) (bool, bool) {
					return fnvInt("null "+p)%2 == 0, false
				}),
				"halfnull-odd": vary(ctx, fullModel, required, "", func(p string, _ attr.Value) (bool, bool) {
					return fnvInt("null "+p)%2 == 1, false
				}),
				"leafnull": vary(ctx, fullModel, required, "", func(_ string, v attr.Value) (bool, bool) {
					return !isContainer(ctx, v), false
				}),
				"allnull": vary(ctx, fullModel, required, "", func(string, attr.Value) (bool, bool) { return true, false }),
				"unknown": vary(ctx, fullModel, required, "", func(p string, _ attr.Value) (bool, bool) {
					return false, fnvInt("unknown "+p)%3 == 0
				}),
			}
			bases := []namedPayload{{"empty", "{}"}}
			if c.servedBase {
				bases = append(bases, payloads...)
			}
			for mname, model := range models {
				for _, base := range bases {
					oldDoc, newDoc := parseDoc(t, base.raw), parseDoc(t, base.raw)
					writes++
					oldDiags := c.oldOverlay(ctx, oldDoc, model)
					newDiags := b.Overlay(ctx, newDoc, model)
					if oldDiags.HasError() != newDiags.HasError() {
						t.Errorf("write %s onto %s: old errors %v, new errors %v", mname, base.name, oldDiags, newDiags)
						continue
					}
					if got, want := canonicalJSON(t, newDoc), canonicalJSON(t, oldDoc); !reflect.DeepEqual(got, want) {
						o, _ := json.Marshal(oldDoc)
						n, _ := json.Marshal(newDoc)
						t.Errorf("write %s onto %s differs:\nold %s\nnew %s", mname, base.name, o, n)
					}
				}
			}
			t.Logf("%d payloads: %d reads and %d writes compared", len(payloads), reads, writes)
		})
	}
}

func TestUnitWireKeepPlannedNullsMatchesLibraryReconcile(t *testing.T) {
	ctx := context.Background()
	c := findWireDiffCase(t, "library_options")
	b := c.binding(t)
	payloads := c.payloads(t)
	nullPrior := types.ObjectNull(b.AttrTypes)
	full, d := b.Flatten(ctx, parseDoc(t, payloads[0].raw), nullPrior)
	if d.HasError() {
		t.Fatal(d)
	}
	// Null what the old reconcile covers: the lists inside type_options and
	// network_path inside path_infos.
	opts, _ := full.Attributes()["library_options"].(basetypes.ObjectValue)
	nullIn := func(listName string, names ...string) {
		l, _ := opts.Attributes()[listName].(basetypes.ListValue)
		et, _ := l.ElementType(ctx).(basetypes.ObjectType)
		elems := make([]attr.Value, len(l.Elements()))
		for i, e := range l.Elements() {
			o, _ := e.(basetypes.ObjectValue)
			for _, n := range names {
				o = withAttr(ctx, o, n, nullValue(ctx, o.Attributes()[n].Type(ctx)))
			}
			elems[i] = o
		}
		nl, _ := types.ListValue(et, elems)
		opts = withAttr(ctx, opts, listName, nl)
	}
	nullIn("type_options", "metadata_fetchers", "metadata_fetcher_order", "image_fetchers", "image_options", "image_fetcher_order", "similar_item_providers", "similar_item_provider_order")
	nullIn("path_infos", "network_path")
	planned := withAttr(ctx, full, "library_options", opts)

	for _, p := range payloads {
		got, d := b.Flatten(ctx, parseDoc(t, p.raw), planned)
		if d.HasError() {
			t.Fatalf("%s: %v", p.name, d)
		}
		plannedModel, _ := modelOf[LibraryResourceModel](ctx, planned)
		gotModel, _ := modelOf[LibraryResourceModel](ctx, got)
		gotModel.LibraryOptions = keepPlannedNulls(ctx, plannedModel.LibraryOptions, gotModel.LibraryOptions)
		oldObj, _ := objectOf(ctx, b.AttrTypes, &gotModel)
		newObj := wire.KeepPlannedNulls(planned, got)
		var diffs []string
		valueDiffs(ctx, oldObj, newObj, "", &diffs)
		for _, diff := range diffs {
			t.Errorf("%s: %s", p.name, diff)
		}
	}
}

func TestUnitWireDroppedMatchesSimilarItemCheck(t *testing.T) {
	ctx := context.Background()
	c := findWireDiffCase(t, "library_options")
	b := c.binding(t)
	full := synthesize(wire.Pinned(), "LibraryOptions", c.name, 0)
	c.extra(full)
	raw, _ := json.Marshal(full)
	planned, d := b.Flatten(ctx, parseDoc(t, string(raw)), types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	typeOptions, _ := full["TypeOptions"].([]any)
	for _, e := range typeOptions {
		if entry, ok := e.(map[string]any); ok {
			delete(entry, "SimilarItemProviders")
			delete(entry, "SimilarItemProviderOrder")
		}
	}
	raw, _ = json.Marshal(full)
	got, d := b.Flatten(ctx, parseDoc(t, string(raw)), planned)
	if d.HasError() {
		t.Fatal(d)
	}

	plannedModel, _ := modelOf[LibraryResourceModel](ctx, planned)
	gotModel, _ := modelOf[LibraryResourceModel](ctx, got)
	var oldDiags diag.Diagnostics
	checkSimilarItemSettingsKept(ctx, plannedModel.LibraryOptions, gotModel.LibraryOptions, &oldDiags)

	var newPaths []string
	for _, e := range b.Dropped(planned, got) {
		if pe, ok := e.(diag.DiagnosticWithPath); ok {
			newPaths = append(newPaths, pe.Path().String())
		}
	}
	sort.Strings(newPaths)
	want := []string{
		"library_options.type_options[0].similar_item_provider_order",
		"library_options.type_options[0].similar_item_providers",
		"library_options.type_options[1].similar_item_provider_order",
		"library_options.type_options[1].similar_item_providers",
	}
	if !reflect.DeepEqual(newPaths, want) {
		t.Errorf("Dropped reports %v, want %v", newPaths, want)
	}
	if len(oldDiags) != 1 {
		t.Fatalf("old check reports %v", oldDiags)
	}
	for _, p := range want {
		if !strings.Contains(oldDiags[0].Detail(), strings.TrimPrefix(p, "library_options.")) {
			t.Errorf("old check does not name %s: %s", p, oldDiags[0].Detail())
		}
	}
}

func diagLines(diags diag.Diagnostics) []string {
	var out []string
	for _, e := range diags {
		p := "(no path)"
		if pe, ok := e.(diag.DiagnosticWithPath); ok {
			p = pe.Path().String()
		}
		out = append(out, fmt.Sprintf("%s | %s | %s | %s", p, e.Severity(), e.Summary(), e.Detail()))
	}
	sort.Strings(out)
	return out
}

func TestUnitWireVersionErrorsMatchTheHandWrittenChecks(t *testing.T) {
	ctx := context.Background()
	lib := findWireDiffCase(t, "library_options")
	libRoot, err := pendingWireMigration["jellyfin_library"]()
	if err != nil {
		t.Fatal(err)
	}
	libDoc := lib.binding(t)
	full := synthesize(wire.Pinned(), "LibraryOptions", lib.name, 0)
	lib.extra(full)
	raw, _ := json.Marshal(full)
	planned, d := libDoc.Flatten(ctx, parseDoc(t, string(raw)), types.ObjectNull(libDoc.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	libSchema := schemaOf(NewLibraryResource())
	tfValue, err := planned.ToTerraformValue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	libConfig := tfsdk.Config{Schema: libSchema, Raw: tfValue}
	for _, version := range []string{"10.9.11", "10.10.0", "10.11.11", "12.0.0", "12.1.0", "unstable"} {
		similar, network := configuredVersionedAttributes(ctx, libConfig)
		old := diagLines(versionedAttributeErrors(version, similar, network))
		got := diagLines(libRoot.VersionErrors(ctx, libConfig, func() (string, error) { return version, nil }))
		if !reflect.DeepEqual(old, got) {
			t.Errorf("library on %s:\nold %q\nnew %q", version, old, got)
		}
		if version == "10.11.11" && len(old) != 6 {
			t.Errorf("library on %s: %d errors, want one per similar item setting and network path of the two synthesized elements", version, len(old))
		}
	}
}

// renameUser posts the served user with only Name changed. The user binding
// also covers the policy and the three flags, which that request must not
// write, so the rename selects name alone.
func TestUnitWireSelectingNameWritesLikeRenameUser(t *testing.T) {
	ctx := context.Background()
	root, err := pendingWireMigration["jellyfin_user"]()
	if err != nil {
		t.Fatal(err)
	}
	rename, err := root.Select("name")
	if err != nil {
		t.Fatal(err)
	}
	model, d := root.Flatten(ctx, parseDoc(t, string(mustJSON(synthesize(wire.Pinned(), "UserDto", "model", 0)))), types.ObjectNull(root.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	model = withAttr(ctx, model, "name", types.StringValue("renamed"))
	for _, base := range []string{"{}", string(mustJSON(synthesize(wire.Pinned(), "UserDto", "served", 0)))} {
		oldDoc, newDoc := parseDoc(t, base), parseDoc(t, base)
		putJSONString(oldDoc, "Name", types.StringValue("renamed"))
		if d := rename.Overlay(ctx, newDoc, model); d.HasError() {
			t.Fatal(d)
		}
		if got, want := canonicalJSON(t, newDoc), canonicalJSON(t, oldDoc); !reflect.DeepEqual(got, want) {
			t.Errorf("rename onto %s:\nold %s\nnew %s", base, mustJSON(oldDoc), mustJSON(newDoc))
		}
	}
}
