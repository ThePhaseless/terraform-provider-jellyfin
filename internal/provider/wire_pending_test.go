// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

// pendingWireMigration holds the bindings of the resources that still map
// their keys by hand. TestUnitWireBindings checks them like the others, and
// each moves into its resource's Wire method when the resource switches.
var pendingWireMigration = map[string]func() (*wire.Binding, error){
	"jellyfin_encoding_configuration": func() (*wire.Binding, error) {
		return wire.Bind(schemaOf(NewEncodingConfigurationResource()), "EncodingOptions",
			wire.Identity("id"),
			wire.VersionMessage("hls_audio_seek_strategy", encodingVersionMessage),
			wire.VersionMessage("subtitle_extraction_timeout_minutes", encodingVersionMessage))
	},
	"jellyfin_networking_configuration": func() (*wire.Binding, error) {
		return wire.Bind(schemaOf(NewNetworkingConfigurationResource()), "NetworkConfiguration", wire.Identity("id"))
	},
	"jellyfin_system_configuration": func() (*wire.Binding, error) {
		return wire.Bind(schemaOf(NewSystemConfigurationResource()), "ServerConfiguration", wire.Identity("id"))
	},
	"jellyfin_livetv_configuration": func() (*wire.Binding, error) {
		return wire.Bind(schemaOf(NewLiveTVConfigurationResource()), "LiveTvOptions", wire.Identity("id"))
	},
	"jellyfin_scheduled_task": func() (*wire.Binding, error) {
		return wire.Bind(schemaOf(NewScheduledTaskResource()), "TaskInfo",
			wire.Identity("id", "task_id"),
			// The typed client reads missing or null triggers as an empty list.
			wire.ReadMissingAs("triggers", types.ListValueMust(scheduledTaskTriggerObjectType(), []attr.Value{})))
	},
	"jellyfin_security_plugin_configuration": func() (*wire.Binding, error) {
		opts := []wire.Option{
			wire.Identity("id", "plugin_id"),
			wire.Delimited("oidc_providers.scopes", " "),
			wire.Delimited("oidc_providers.acr_values", " "),
			wire.Delimited("oidc_providers.allowed_groups", ","),
			wire.Delimited("oidc_providers.admin_groups", ","),
			wire.Delimited("oidc_providers.additional_allowed_cidrs", ","),
			wire.Delimited("oidc_providers.role_library_mappings.library_ids", ","),
			wire.CarryServed("oidc_providers", "CreatedAt", "id"),
			wire.WithCodec("enrollment_deadline", sameInstantCodec{}),
		}
		// The plugin leaves these out when false.
		for _, name := range []string{
			"allow_indefinite_trust", "onboarding_password_require_uppercase",
			"onboarding_password_require_lowercase", "onboarding_password_require_digit",
			"onboarding_password_require_symbol",
		} {
			opts = append(opts, wire.ReadMissingAs(name, types.BoolValue(false)))
		}
		return wire.Bind(schemaOf(NewJellyfinSecurityPluginConfigurationResource()), wire.SecurityPluginRoot, opts...)
	},
}

func encodingVersionMessage(g wire.VersionGap) (string, string) {
	return "Unsupported Jellyfin server version",
		fmt.Sprintf("%s requires Jellyfin %s or later: the server's encoding configuration has no %s field, so it would discard the value. Remove %s from the configuration or upgrade the server.", g.Path, g.Since, g.Key, g.Path)
}

func scheduledTaskTriggerObjectType() types.ObjectType {
	attrs := schemaOf(NewScheduledTaskResource()).Attributes["triggers"].GetType()
	lt, _ := attrs.(types.ListType)
	ot, _ := lt.ElemType.(types.ObjectType)
	return ot
}

// sameInstantCodec reads a date-time back as its prior value when both name
// the same instant; see keepSameInstant.
type sameInstantCodec struct{}

func (sameInstantCodec) String() string { return "same-instant" }

func (sameInstantCodec) Encode(_ context.Context, v attr.Value) (json.RawMessage, diag.Diagnostics) {
	s, _ := v.(basetypes.StringValue)
	b, err := json.Marshal(s.ValueString())
	if err != nil {
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("Failed to encode a date-time", err.Error())}
	}
	return b, nil
}

func (sameInstantCodec) Decode(_ context.Context, raw json.RawMessage, prior attr.Value, _ attr.Type) (attr.Value, diag.Diagnostics) {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return types.StringNull(), nil
	}
	p, _ := prior.(basetypes.StringValue)
	return keepSameInstant(p, types.StringValue(s)), nil
}
