// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnitSSOPluginConfigurationOverlay(t *testing.T) {
	ctx := context.Background()
	fixture := `{"OidConfigs":{"authentik":{"OidEndpoint":"https://auth.example.com/application/o/jellyfin","OidClientId":"client","OidSecret":"secret","Enabled":true,"EnableAuthorization":true,"EnableAllFolders":true,"EnabledFolders":[],"AdminRoles":["admins"],"Roles":["watchers","admins"],"EnableFolderRoles":false,"EnableLiveTvRoles":false,"EnableLiveTv":false,"EnableLiveTvManagement":false,"LiveTvRoles":[],"LiveTvManagementRoles":[],"FolderRoleMapping":[],"RoleClaim":"groups","OidScopes":["groups"],"DefaultProvider":"","SchemeOverride":"https","PortOverride":0,"NewPath":true,"CanonicalLinks":{"ThePhaseless":"bc2a3d07-5f7b-42bc-8252-1b32e7ba18a1"},"DefaultUsernameClaim":"preferred_username","AvatarURLFormat":"","DisableHTTPS":false,"DisablePushedAuthorization":false,"DoNotValidateEndpoints":false,"DoNotValidateIssuerName":false,"DoNotLoadProfile":false}},"SamlConfigs":{}}`

	var data SSOPluginConfigurationResourceModel
	data.PluginID = types.StringValue(ssoPluginID)
	flattenSSOPluginConfiguration(ctx, fixture, &data, nil)

	base := map[string]json.RawMessage{}
	overlaySSOPluginConfiguration(ctx, base, &data)

	result, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	var want map[string]interface{}
	if err := json.Unmarshal([]byte(fixture), &want); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("round-trip mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}
