// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
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

// wireBinding returns nil after reporting the error when the binding cannot be
// built, which TestUnitWireBindings rules out for a released provider.
func wireBinding(diags *diag.Diagnostics, bind func() (*wire.Binding, error)) *wire.Binding {
	b, err := bind()
	if err != nil {
		diags.AddError("Failed to map the resource to Jellyfin's keys", err.Error()+"\n\nThis is a bug in the provider.")
		return nil
	}
	return b
}

// checkServerHasFields rejects, at plan time, configured values whose fields
// the server's Jellyfin version lacks. Such a server accepts the write and
// silently drops the value, which Terraform could only report after apply as
// an inconsistent result.
func checkServerHasFields(ctx context.Context, c *client.Client, b *wire.Binding, config tfsdk.Config) diag.Diagnostics {
	if c == nil {
		return nil
	}
	return b.VersionErrors(ctx, config, func() (string, error) {
		info, err := c.GetPublicSystemInfo(ctx)
		if err != nil {
			return "", err
		}
		return info.Version, nil
	})
}
