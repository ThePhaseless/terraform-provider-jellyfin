// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

// pendingWireMigration holds the bindings of the resources that still map
// their keys by hand. TestUnitWireBindings checks them like the others, and
// each moves into its resource's Wire method when the resource switches.
var pendingWireMigration = map[string]func() (*wire.Binding, error){}
