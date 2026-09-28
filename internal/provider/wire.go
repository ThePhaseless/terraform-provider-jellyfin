// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

// wireBound is implemented by every resource that reads and writes Jellyfin
// JSON documents, with the binding of its schema to their keys.
type wireBound interface {
	Wire() (*wire.Binding, error)
}

// configuredClient returns the client the provider hands a resource or data
// source, a kind, in its Configure, or nil before the provider is configured.
func configuredClient(providerData any, kind string, diags *diag.Diagnostics) *client.Client {
	if providerData == nil {
		return nil
	}
	c, ok := providerData.(*client.Client)
	if !ok {
		diags.AddError("Unexpected "+kind+" Configure Type", fmt.Sprintf("Expected *client.Client, got: %T.", providerData))
	}
	return c
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

// document is a Jellyfin JSON document that a resource reads and writes
// whole through its binding: what names it in messages, get reads it, and put
// replaces it. gone, when set, handles a document the server does not have.
type document struct {
	what string
	get  func(context.Context) (string, error)
	put  func(context.Context, string) error
	gone func(context.Context)
}

// write writes model, the plan, over the document the server serves, and
// then reads model from what the server serves after the write. It reports
// whether model holds that read.
func (d document) write(ctx context.Context, b *wire.Binding, model any, diags *diag.Diagnostics) bool {
	current, err := d.get(ctx)
	if err != nil {
		diags.AddError("Failed to read current "+d.what, err.Error())
		return false
	}
	base, err := parseJSONObject(current)
	if err != nil {
		diags.AddError("Failed to parse current "+d.what, err.Error())
		return false
	}
	if o := b.OverlayModel(ctx, base, model); o.HasError() {
		diags.Append(o...)
		return false
	}
	payload, err := json.Marshal(base)
	if err != nil {
		diags.AddError("Failed to serialize "+d.what, err.Error())
		return false
	}
	if err := d.put(ctx, string(payload)); err != nil {
		diags.AddError("Failed to update "+d.what, err.Error())
		return false
	}
	updated, err := d.get(ctx)
	if err != nil {
		diags.AddError("Failed to read "+d.what+" after update", err.Error())
		return false
	}
	diags.Append(b.FlattenAfterApply(ctx, updated, model)...)
	return true
}

// read reads model from the document the server serves, and reports whether
// model holds that read.
func (d document) read(ctx context.Context, b *wire.Binding, model any, diags *diag.Diagnostics) bool {
	current, err := d.get(ctx)
	switch {
	case d.gone != nil && client.IsNotFound(err):
		d.gone(ctx)
		return false
	case err != nil:
		diags.AddError("Failed to read "+d.what, err.Error())
		return false
	}
	diags.Append(b.FlattenInto(ctx, current, model)...)
	return true
}

type singleton[M any] struct {
	id   string
	bind func() (*wire.Binding, error)
	doc  document
}

func (s singleton[M]) write(ctx context.Context, plan tfsdk.Plan, state *tfsdk.State, diags *diag.Diagnostics) {
	var data M
	diags.Append(plan.Get(ctx, &data)...)
	if diags.HasError() {
		return
	}
	if b := wireBinding(diags, s.bind); b != nil && s.doc.write(ctx, b, &data, diags) {
		s.set(ctx, &data, state, diags)
	}
}

func (s singleton[M]) read(ctx context.Context, prior tfsdk.State, state *tfsdk.State, diags *diag.Diagnostics) {
	var data M
	diags.Append(prior.Get(ctx, &data)...)
	if diags.HasError() {
		return
	}
	if b := wireBinding(diags, s.bind); b != nil && s.doc.read(ctx, b, &data, diags) {
		s.set(ctx, &data, state, diags)
	}
}

func (s singleton[M]) set(ctx context.Context, data *M, state *tfsdk.State, diags *diag.Diagnostics) {
	diags.Append(state.Set(ctx, data)...)
	s.setID(ctx, state, diags)
}

func (s singleton[M]) setID(ctx context.Context, state *tfsdk.State, diags *diag.Diagnostics) {
	diags.Append(state.SetAttribute(ctx, path.Root("id"), types.StringValue(s.id))...)
}

// checkServerHasFields rejects, at plan time, configured values whose fields
// the server's Jellyfin version lacks. Such a server accepts the write and
// silently drops the value, which Terraform could only report after apply as
// an inconsistent result. Terraform plans each resource this way again in the
// refresh that precedes a destroy, so such a value left in the configuration
// also fails terraform destroy unless it runs with -refresh=false.
func checkServerHasFields(ctx context.Context, c *client.Client, bind func() (*wire.Binding, error), config tfsdk.Config, diags *diag.Diagnostics) {
	b := wireBinding(diags, bind)
	if b == nil || c == nil {
		return
	}
	diags.Append(b.VersionErrors(ctx, config, func() (string, error) {
		info, err := c.GetPublicSystemInfo(ctx)
		if err != nil {
			return "", err
		}
		return info.Version, nil
	})...)
}
