// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

// wireBound is implemented by every resource that reads and writes Jellyfin
// JSON documents, with the binding of its schema to their keys.
type wireBound interface {
	Wire() (*wire.Binding, error)
}

func schemaOf(r resource.Resource) schema.Schema {
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp.Schema
}
