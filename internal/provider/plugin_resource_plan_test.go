// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func pluginResourcePlan(t *testing.T, m PluginResourceModel) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: pluginResourceSchema(t)}
	if diags := plan.Set(context.Background(), &m); diags.HasError() {
		t.Fatalf("plan: %v", diags.Errors())
	}
	return plan
}

func modifyPluginPlan(t *testing.T, r *PluginResource, state, plan PluginResourceModel) *resource.ModifyPlanResponse {
	t.Helper()
	req := resource.ModifyPlanRequest{
		Plan:  pluginResourcePlan(t, plan),
		State: pluginResourceState(t, state),
	}
	resp := &resource.ModifyPlanResponse{Plan: req.Plan}
	r.ModifyPlan(context.Background(), req, resp)
	return resp
}

func bookshelfPackage(versions ...string) []client.PackageInfo {
	pkg := client.PackageInfo{Name: "Bookshelf"}
	for _, v := range versions {
		pkg.Versions = append(pkg.Versions, client.VersionInfo{Version: v, RepositoryURL: stableRepoURL})
	}
	return []client.PackageInfo{pkg}
}

func TestUnitPluginVersionChangeReplacesUnlessItNamesInstalledVersion(t *testing.T) {
	installed := PluginResourceModel{
		ID:               types.StringValue(bookshelfID),
		Name:             types.StringValue("Bookshelf"),
		Version:          types.StringValue("13.0.0.0"),
		InstalledVersion: types.StringValue("13.0.0.0"),
		RepositoryURL:    types.StringValue(stableRepoURL),
	}
	withVersion := func(m PluginResourceModel, version types.String) PluginResourceModel {
		m.Version = version
		return m
	}
	fromKeyword := withVersion(installed, types.StringValue("latest"))
	beforeInstalledVersion := installed
	beforeInstalledVersion.InstalledVersion = types.StringNull()

	cases := []struct {
		name     string
		packages []client.PackageInfo
		state    PluginResourceModel
		version  types.String
		replace  bool
		fails    bool
	}{
		{"keyword resolving to the installed version", bookshelfPackage("13.0.0.0", "12.0.0.0"), installed, types.StringValue("latest"), false, false},
		{"keyword resolving to a newer version", bookshelfPackage("14.0.0.0", "13.0.0.0"), installed, types.StringValue("latest"), true, false},
		{"keyword the repositories cannot resolve", nil, installed, types.StringValue("latest"), false, true},
		{"installed version in place of a keyword", nil, fromKeyword, types.StringValue("13.0.0"), false, false},
		{"another version in place of a keyword", nil, fromKeyword, types.StringValue("12.0.0.0"), true, false},
		{"unknown until apply", nil, installed, types.StringUnknown(), true, false},
		{"keyword over state from before installed_version", bookshelfPackage("13.0.0.0"), beforeInstalledVersion, types.StringValue("latest"), false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newFakePluginResource(t, &fakePluginServer{packages: c.packages})

			resp := modifyPluginPlan(t, r, c.state, withVersion(c.state, c.version))

			if resp.Diagnostics.HasError() != c.fails {
				t.Fatalf("errors = %v, want failure %t", resp.Diagnostics.Errors(), c.fails)
			}
			replace := resp.RequiresReplace.Contains(path.Root("version"))
			if replace != c.replace {
				t.Errorf("replace = %t, want %t", replace, c.replace)
			}
		})
	}
}

func TestUnitPluginUpdateOnlyChangesState(t *testing.T) {
	fake := &fakePluginServer{}
	r := newFakePluginResource(t, fake)
	ctx := context.Background()

	prior := PluginResourceModel{
		ID:               types.StringValue(bookshelfID),
		Name:             types.StringValue("Bookshelf"),
		Version:          types.StringValue("13.0.0.0"),
		InstalledVersion: types.StringValue("13.0.0.0"),
		RepositoryURL:    types.StringNull(),
	}
	planned := prior
	planned.Version = types.StringValue("latest")
	planned.RepositoryURL = types.StringUnknown()

	state := pluginResourceState(t, prior)
	resp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: pluginResourcePlan(t, planned), State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics.Errors())
	}

	var got PluginResourceModel
	resp.State.Get(ctx, &got)
	want := prior
	want.Version = types.StringValue("latest")
	if got != want {
		t.Errorf("state = %+v, want %+v", got, want)
	}
	if fake.maxInFlight != 0 {
		t.Error("Update sent an install or uninstall request")
	}
}
