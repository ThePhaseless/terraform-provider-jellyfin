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
	"jellyfin_user": func() (*wire.Binding, error) {
		return wire.Bind(schemaOf(NewUserResource()), "UserDto",
			wire.Identity("id"),
			wire.Elsewhere("password", "written by POST /Users/Password"),
			wire.Document("Policy"),
			wire.Key("is_administrator", "Policy.IsAdministrator"),
			wire.Key("is_disabled", "Policy.IsDisabled"),
			wire.Key("enable_all_folders", "Policy.EnableAllFolders"),
			// The resource reads these three through the typed client, which
			// reads a missing or null flag as false.
			wire.ReadMissingAs("is_administrator", types.BoolValue(false)),
			wire.ReadMissingAs("is_disabled", types.BoolValue(false)),
			wire.ReadMissingAs("enable_all_folders", types.BoolValue(false)),
			wire.Unmanaged("AccessSchedule", "Id", "Jellyfin numbers access schedules itself"),
			wire.Unmanaged("AccessSchedule", "UserId", "Jellyfin fills in the user the policy belongs to"))
	},
	"jellyfin_library": func() (*wire.Binding, error) {
		opts := []wire.Option{
			wire.Identity("id"),
			wire.Document("LibraryOptions"),
			wire.Key("paths", "Locations"),
			wire.Key("library_options.extract_chapters_during_library_scan", "ExtractChapterImagesDuringLibraryScan"),
			wire.Inverted("library_options.disabled", "Enabled"),
			wire.Legacy("library_options.path_infos.network_path", "NetworkPath", "10.10", networkPathRemovedMessage),
			wire.NeverSent("library_options.path_infos.username", unsupportedLibraryOptionMessage),
			wire.NeverSent("library_options.path_infos.password", unsupportedLibraryOptionMessage),
			wire.MergeByKey("library_options.type_options", "type"),
			wire.MergeByKey("library_options.type_options.image_options", "type"),
			wire.VersionMessage("library_options.type_options.similar_item_providers", similarItemsVersionMessage),
			wire.VersionMessage("library_options.type_options.similar_item_provider_order", similarItemsVersionMessage),
			wire.VersionMessage("library_options.path_infos.network_path", func(g wire.VersionGap) (string, string) {
				return "Network paths not supported",
					fmt.Sprintf("The server runs Jellyfin %s, and Jellyfin %s removed network paths, so the server would drop the value. Remove %s from the configuration.", g.ServerVersion, g.Until, g.Path)
			}),
		}
		for _, name := range []string{
			"enable_emby_photos", "enable_photo_subtitle", "chapter_image_interval_seconds",
			"extract_media_information_during_library_scan", "download_images_in_advance",
			"cache_images_in_library", "enable_media_conversion", "disabled_metadata_savers",
			"disabled_metadata_fetchers", "metadata_fetcher_order", "disabled_image_fetchers",
			"image_fetcher_order", "save_local_thumbnail_sets", "import_missing_episodes",
			"metadata_refresh_mode",
		} {
			opts = append(opts, wire.NeverSent("library_options."+name, unsupportedLibraryOptionMessage))
		}
		return wire.Bind(schemaOf(NewLibraryResource()), "VirtualFolderInfo", opts...)
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

func similarItemsVersionMessage(g wire.VersionGap) (string, string) {
	return "Similar item settings not supported",
		fmt.Sprintf("The server runs Jellyfin %s, and similar item providers need Jellyfin 12 or later. Remove %s for this server.", g.ServerVersion, g.Path)
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
