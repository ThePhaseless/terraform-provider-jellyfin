// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func pluginResourcePlan(t *testing.T, m PluginResourceModel) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: schemaOf(NewPluginResource())}
	if diags := plan.Set(t.Context(), &m); diags.HasError() {
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
	r.ModifyPlan(t.Context(), req, resp)
	return resp
}

func bookshelfPackage(versions ...string) []client.PackageInfo {
	pkg := client.PackageInfo{Name: "Bookshelf"}
	for _, v := range versions {
		pkg.Versions = append(pkg.Versions, client.VersionInfo{Version: v, RepositoryURL: stableRepoURL})
	}
	return []client.PackageInfo{pkg}
}

func plannedBookshelf(version string) PluginResourceModel {
	return PluginResourceModel{
		ID:               types.StringUnknown(),
		Name:             types.StringValue("Bookshelf"),
		Version:          types.StringValue(version),
		InstalledVersion: types.StringUnknown(),
		RepositoryURL:    types.StringValue(stableRepoURL),
	}
}

func installedBookshelf(version, installedVersion string) PluginResourceModel {
	m := plannedBookshelf(version)
	m.ID = types.StringValue(bookshelfID)
	m.InstalledVersion = types.StringValue(installedVersion)
	return m
}

func TestUnitPluginVersionChangeReplacesUnlessItNamesInstalledVersion(t *testing.T) {
	installed := installedBookshelf("13.0.0.0", "13.0.0.0")
	fromKeyword := installedBookshelf("latest", "13.0.0.0")
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

			plan := c.state
			plan.Version = c.version
			resp := modifyPluginPlan(t, r, c.state, plan)

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
	ctx := t.Context()

	prior := installedBookshelf("13.0.0.0", "13.0.0.0")
	prior.RepositoryURL = types.StringNull()
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

func TestUnitPluginSupportedKeywordFailsPlanUnlessPackagesResolveIt(t *testing.T) {
	unpinnedBuild := pluginRelease(supportedSecurityPluginVersion()) + ".99"
	installed := PluginResourceModel{
		ID:               types.StringValue("94879a0cda244eb1aa06f28b4b9333b1"),
		Name:             types.StringValue(securityPluginName),
		Version:          types.StringValue(unpinnedBuild),
		InstalledVersion: types.StringValue(unpinnedBuild),
		RepositoryURL:    types.StringValue(securityPluginRepoURL),
	}
	supported := installed
	supported.Version = types.StringValue(pluginVersionSupported)

	cases := []struct {
		name  string
		fake  *fakePluginServer
		fails bool
	}{
		{"packages list the installed build", &fakePluginServer{packages: []client.PackageInfo{{Name: securityPluginName, Versions: []client.VersionInfo{{Version: unpinnedBuild}}}}}, false},
		{"packages cannot be listed", &fakePluginServer{packagesStatus: http.StatusInternalServerError}, true},
		{"repositories do not offer the plugin", &fakePluginServer{packages: bookshelfPackage("13.0.0.0")}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := modifyPluginPlan(t, newFakePluginResource(t, c.fake), installed, supported)

			if resp.Diagnostics.HasError() != c.fails {
				t.Fatalf("errors = %v, want failure %t", resp.Diagnostics.Errors(), c.fails)
			}
			if resp.RequiresReplace.Contains(path.Root("version")) {
				t.Error("plan replaces the plugin")
			}
		})
	}
}

// TestUnitPluginVersionPlan runs version's plan modifiers in order, handing
// each the previous one's plan value as the framework does.
func TestUnitPluginVersionPlan(t *testing.T) {
	ctx := t.Context()
	attr, ok := schemaOf(NewPluginResource()).Attributes["version"].(schema.StringAttribute)
	if !ok {
		t.Fatal("version is not a schema.StringAttribute")
	}
	stateWith := func(version string) *PluginResourceModel {
		m := installedBookshelf(version, "13.0.0.0")
		return &m
	}

	cases := []struct {
		name   string
		state  *PluginResourceModel
		config types.String
		want   types.String
	}{
		{"keyword removed from the configuration", stateWith("latest"), types.StringNull(), types.StringValue("13.0.0.0")},
		{"keyword kept in the configuration", stateWith("latest"), types.StringValue("latest"), types.StringValue("latest")},
		{"version removed from the configuration", stateWith("13.0.0.0"), types.StringNull(), types.StringValue("13.0.0.0")},
		{"create without a version", nil, types.StringNull(), types.StringUnknown()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// The framework plans an unset Optional and Computed value as
			// unknown before the plan modifiers run.
			planned := c.config
			if planned.IsNull() {
				planned = types.StringUnknown()
			}
			req := planmodifier.StringRequest{
				Path:        path.Root("version"),
				ConfigValue: c.config,
				StateValue:  types.StringNull(),
				PlanValue:   planned,
				State:       pluginResourceNullState(ctx),
			}
			if c.state != nil {
				req.State = pluginResourceState(t, *c.state)
				req.StateValue = c.state.Version
			}
			for _, m := range attr.PlanModifiers {
				resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
				m.PlanModifyString(ctx, req, resp)
				if resp.Diagnostics.HasError() {
					t.Fatalf("plan modifier: %v", resp.Diagnostics.Errors())
				}
				req.PlanValue = resp.PlanValue
			}
			if !req.PlanValue.Equal(c.want) {
				t.Errorf("plan = %v, want %v", req.PlanValue, c.want)
			}
		})
	}
}
