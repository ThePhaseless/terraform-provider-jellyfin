// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import "testing"

func TestUnitMetadataConfigurationRoundTrip(t *testing.T) {
	checkRoundTrip[MetadataConfigurationResourceModel](t, mustWire(t, metadataWire), `{"UseFileCreationTimeForDateAdded":true}`)
}
