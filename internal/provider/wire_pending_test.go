// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

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
