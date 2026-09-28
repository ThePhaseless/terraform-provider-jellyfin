// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnitMetadataConfigurationRoundTrip(t *testing.T) {
	ctx := context.Background()
	b, err := metadataWire()
	if err != nil {
		t.Fatal(err)
	}
	fixture := `{"UseFileCreationTimeForDateAdded":true}`

	data := MetadataConfigurationResourceModel{ID: types.StringValue("metadata")}
	if d := b.FlattenInto(ctx, fixture, &data); d.HasError() {
		t.Fatalf("read: %v", d)
	}

	base := map[string]json.RawMessage{}
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("write: %v", d)
	}
	checkSameJSON(t, base, fixture)
}
