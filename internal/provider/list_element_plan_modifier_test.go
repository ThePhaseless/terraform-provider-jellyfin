// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var testHostType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"url":   types.StringType,
	"type":  types.StringType,
	"count": types.Int64Type,
}}

func testHost(t *testing.T, url, typ string, count types.Int64) attr.Value {
	t.Helper()
	return testHostURL(t, types.StringValue(url), typ, count)
}

func testHostURL(t *testing.T, url types.String, typ string, count types.Int64) attr.Value {
	t.Helper()
	obj, d := types.ObjectValue(testHostType.AttrTypes, map[string]attr.Value{
		"url":   url,
		"type":  types.StringValue(typ),
		"count": count,
	})
	if d.HasError() {
		t.Fatalf("object: %v", d)
	}
	return obj
}

func testHostList(t *testing.T, hosts ...attr.Value) types.List {
	t.Helper()
	list, d := types.ListValue(testHostType, hosts)
	if d.HasError() {
		t.Fatalf("list: %v", d)
	}
	return list
}

func TestUseStateForUnknownByKey(t *testing.T) {
	t.Parallel()

	unknown := types.Int64Unknown()
	omitted := types.Int64Null()
	unknownURL := types.StringUnknown()
	tests := map[string]struct {
		state  types.List
		config types.List
		plan   types.List
		want   types.List
	}{
		"appended element stays unknown": {
			state:  testHostList(t, testHost(t, "a", "m3u", types.Int64Value(2))),
			config: testHostList(t, testHost(t, "a", "m3u", omitted), testHost(t, "b", "hdhomerun", omitted)),
			plan:   testHostList(t, testHost(t, "a", "m3u", unknown), testHost(t, "b", "hdhomerun", unknown)),
			want:   testHostList(t, testHost(t, "a", "m3u", types.Int64Value(2)), testHost(t, "b", "hdhomerun", unknown)),
		},
		"reordered elements follow their key, not their index": {
			state:  testHostList(t, testHost(t, "a", "m3u", types.Int64Value(2)), testHost(t, "b", "m3u", types.Int64Value(3))),
			config: testHostList(t, testHost(t, "b", "m3u", omitted), testHost(t, "a", "m3u", omitted)),
			plan:   testHostList(t, testHost(t, "b", "m3u", unknown), testHost(t, "a", "m3u", unknown)),
			want:   testHostList(t, testHost(t, "b", "m3u", types.Int64Value(3)), testHost(t, "a", "m3u", types.Int64Value(2))),
		},
		"exact key is claimed before an earlier element falls back": {
			state:  testHostList(t, testHost(t, "a", "m3u", types.Int64Value(2))),
			config: testHostList(t, testHost(t, "new", "m3u", omitted), testHost(t, "a", "m3u", omitted)),
			plan:   testHostList(t, testHost(t, "new", "m3u", unknown), testHost(t, "a", "m3u", unknown)),
			want:   testHostList(t, testHost(t, "new", "m3u", unknown), testHost(t, "a", "m3u", types.Int64Value(2))),
		},
		"fallback key matches an edited element": {
			state:  testHostList(t, testHost(t, "a", "m3u", types.Int64Value(2))),
			config: testHostList(t, testHost(t, "moved", "m3u", omitted)),
			plan:   testHostList(t, testHost(t, "moved", "m3u", unknown)),
			want:   testHostList(t, testHost(t, "moved", "m3u", types.Int64Value(2))),
		},
		"no prior state leaves the plan alone": {
			state:  types.ListNull(testHostType),
			config: testHostList(t, testHost(t, "a", "m3u", omitted)),
			plan:   testHostList(t, testHost(t, "a", "m3u", unknown)),
			want:   testHostList(t, testHost(t, "a", "m3u", unknown)),
		},
		"value unknown in config stays unknown": {
			state:  testHostList(t, testHost(t, "a", "m3u", types.Int64Value(2))),
			config: testHostList(t, testHost(t, "a", "m3u", unknown)),
			plan:   testHostList(t, testHost(t, "a", "m3u", unknown)),
			want:   testHostList(t, testHost(t, "a", "m3u", unknown)),
		},
		"duplicate key prefers the element at the same index": {
			state:  testHostList(t, testHost(t, "a", "m3u", types.Int64Value(2)), testHost(t, "a", "m3u", types.Int64Value(3))),
			config: testHostList(t, testHostURL(t, types.StringNull(), "m3u", omitted), testHost(t, "a", "m3u", omitted)),
			plan:   testHostList(t, testHostURL(t, unknownURL, "m3u", unknown), testHost(t, "a", "m3u", unknown)),
			want:   testHostList(t, testHost(t, "a", "m3u", types.Int64Value(2)), testHost(t, "a", "m3u", types.Int64Value(3))),
		},
		"key unknown in config leaves every element unknown": {
			state:  testHostList(t, testHost(t, "a", "m3u", types.Int64Value(2)), testHost(t, "b", "m3u", types.Int64Value(3))),
			config: testHostList(t, testHost(t, "a", "m3u", omitted), testHostURL(t, unknownURL, "m3u", omitted)),
			plan:   testHostList(t, testHost(t, "a", "m3u", unknown), testHostURL(t, unknownURL, "m3u", unknown)),
			want:   testHostList(t, testHost(t, "a", "m3u", unknown), testHostURL(t, unknownURL, "m3u", unknown)),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resp := planmodifier.ListResponse{PlanValue: test.plan}
			useStateForUnknownByKey([]string{"url"}, []string{"type"}).PlanModifyList(context.Background(), planmodifier.ListRequest{
				StateValue:  test.state,
				ConfigValue: test.config,
				PlanValue:   test.plan,
			}, &resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			if !resp.PlanValue.Equal(test.want) {
				t.Fatalf("plan = %s, want %s", resp.PlanValue, test.want)
			}
		})
	}
}

var testGroupType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"id":    types.StringType,
	"hosts": types.ListType{ElemType: testHostType},
}}

// testGroups returns a list holding one group of hosts.
func testGroups(t *testing.T, hosts ...attr.Value) types.List {
	t.Helper()
	group, d := types.ObjectValue(testGroupType.AttrTypes, map[string]attr.Value{
		"id":    types.StringValue("g"),
		"hosts": testHostList(t, hosts...),
	})
	if d.HasError() {
		t.Fatalf("object: %v", d)
	}
	list, d := types.ListValue(testGroupType, []attr.Value{group})
	if d.HasError() {
		t.Fatalf("list: %v", d)
	}
	return list
}

func TestUseStateForUnknownByKeyFillsNestedLists(t *testing.T) {
	t.Parallel()

	unknown := types.Int64Unknown()
	omitted := types.Int64Null()
	tests := map[string]struct {
		state  types.List
		config types.List
		plan   types.List
		want   types.List
	}{
		"reordered elements follow the values they configure": {
			state:  testGroups(t, testHost(t, "a", "m3u", types.Int64Value(2)), testHost(t, "b", "m3u", types.Int64Value(3))),
			config: testGroups(t, testHost(t, "b", "m3u", omitted), testHost(t, "a", "m3u", omitted)),
			plan:   testGroups(t, testHost(t, "b", "m3u", unknown), testHost(t, "a", "m3u", unknown)),
			want:   testGroups(t, testHost(t, "b", "m3u", types.Int64Value(3)), testHost(t, "a", "m3u", types.Int64Value(2))),
		},
		"element with no match stays unknown": {
			state:  testGroups(t, testHost(t, "a", "m3u", types.Int64Value(2))),
			config: testGroups(t, testHost(t, "new", "m3u", omitted)),
			plan:   testGroups(t, testHost(t, "new", "m3u", unknown)),
			want:   testGroups(t, testHost(t, "new", "m3u", unknown)),
		},
		"value unknown in config leaves every element unknown": {
			state:  testGroups(t, testHost(t, "a", "m3u", types.Int64Value(2)), testHost(t, "b", "m3u", types.Int64Value(3))),
			config: testGroups(t, testHost(t, "a", "m3u", omitted), testHostURL(t, types.StringUnknown(), "m3u", omitted)),
			plan:   testGroups(t, testHost(t, "a", "m3u", unknown), testHostURL(t, types.StringUnknown(), "m3u", unknown)),
			want:   testGroups(t, testHost(t, "a", "m3u", unknown), testHostURL(t, types.StringUnknown(), "m3u", unknown)),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resp := planmodifier.ListResponse{PlanValue: test.plan}
			useStateForUnknownByKey([]string{"id"}).PlanModifyList(context.Background(), planmodifier.ListRequest{
				StateValue:  test.state,
				ConfigValue: test.config,
				PlanValue:   test.plan,
			}, &resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			if !resp.PlanValue.Equal(test.want) {
				t.Fatalf("plan = %s, want %s", resp.PlanValue, test.want)
			}
		})
	}
}
