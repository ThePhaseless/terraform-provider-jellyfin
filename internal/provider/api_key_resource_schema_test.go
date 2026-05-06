// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestAPIKeyResourceIDIsSensitive(t *testing.T) {
	t.Parallel()

	var resp resource.SchemaResponse
	(&APIKeyResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attr, ok := resp.Schema.Attributes["id"].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("id attribute type = %T, want schema.StringAttribute", resp.Schema.Attributes["id"])
	}
	if !attr.Sensitive {
		t.Fatal("id attribute must be sensitive because it mirrors access_token")
	}
}
