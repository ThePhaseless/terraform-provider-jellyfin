// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package wire

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

type miniModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Fresh types.String `tfsdk:"fresh"`
	Hosts types.List   `tfsdk:"hosts"`
}

func TestUnitModelReadsAndWrites(t *testing.T) {
	ctx := t.Context()
	hostAttrs := map[string]schema.Attribute{"url": optString(), "kind": optString()}
	b, err := Bind(schema.Schema{Attributes: map[string]schema.Attribute{
		"id":    schema.StringAttribute{Computed: true},
		"name":  optString(),
		"fresh": optString(),
		"hosts": schema.ListNestedAttribute{Optional: true, Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: hostAttrs}},
	}}, "Doc", Identity("id"), Unmanaged("Hosts[]", "Extra", "not managed"), Unmanaged("Hosts[]", "Created", "not managed"))
	if err != nil {
		t.Fatal(err)
	}
	hostType := types.ObjectType{AttrTypes: map[string]attr.Type{"url": types.StringType, "kind": types.StringType}}
	plan := miniModel{
		ID:    types.StringUnknown(),
		Name:  types.StringValue("n"),
		Fresh: types.StringValue("f"),
		Hosts: types.ListValueMust(hostType, []attr.Value{types.ObjectValueMust(hostType.AttrTypes, map[string]attr.Value{"url": types.StringValue("u"), "kind": types.StringNull()})}),
	}
	served := doc(t, `{"Kept": "k"}`)
	if d := b.OverlayModel(ctx, served, &plan); d.HasError() {
		t.Fatal(d)
	}
	if got, want := canonical(t, served), `{"Fresh":"f","Hosts":[{"Url":"u"}],"Kept":"k","Name":"n"}`; got != want {
		t.Errorf("OverlayModel wrote %s, want %s", got, want)
	}

	applied := plan
	d := b.FlattenAfterApply(ctx, `{"Name": "n", "Hosts": [{"Url": "u", "Kind": ""}]}`, &applied)
	if !d.HasError() || !strings.Contains(d[0].Detail(), "did not keep the value") {
		t.Errorf("the dropped fresh is not reported: %v", d)
	}
	if !applied.Fresh.IsNull() || !applied.ID.IsUnknown() || applied.Name.ValueString() != "n" || len(applied.Hosts.Elements()) != 1 {
		t.Fatalf("FlattenAfterApply read %+v", applied)
	}
	host, _ := applied.Hosts.Elements()[0].(basetypes.ObjectValue)
	if !host.Attributes()["kind"].IsNull() {
		t.Errorf("the planned null kind reads as %s", host.Attributes()["kind"])
	}

	state := miniModel{ID: types.StringValue("the-id"), Hosts: types.ListNull(hostType)}
	if d := b.FlattenInto(ctx, `{"Name": "m", "Hosts": [{"Url": "u", "Kind": ""}]}`, &state); d.HasError() {
		t.Fatal(d)
	}
	if state.ID.ValueString() != "the-id" || state.Name.ValueString() != "m" || len(state.Hosts.Elements()) != 1 {
		t.Fatalf("FlattenInto read %+v", state)
	}
	host, _ = state.Hosts.Elements()[0].(basetypes.ObjectValue)
	if got := host.Attributes()["kind"].String(); got != `""` {
		t.Errorf("a refresh reads the empty kind as %s", got)
	}
	if d := b.FlattenInto(ctx, `{`, &state); !d.HasError() {
		t.Error("an unparseable document reads")
	}
}
