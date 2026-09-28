// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

// ReadForImport returns the schema of resourceType and the values that
// terraform import with importID gives its attributes once the Read that
// follows has read raw, the JSON document Jellyfin serves for the resource.
// Only resources that read a Jellyfin document through a wire binding have
// one; an attribute the Read sets without the document, such as a computed
// id, holds what ImportState gave it. c answers what the Read asks the server
// besides the document: the providers it offers each item type.
func ReadForImport(ctx context.Context, c *client.Client, resourceType, importID, raw string) (schema.Schema, types.Object, error) {
	var r resource.Resource
	for _, newResource := range New("import")().Resources(ctx) {
		candidate := newResource()
		var meta resource.MetadataResponse
		candidate.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "jellyfin"}, &meta)
		if meta.TypeName == resourceType {
			r = candidate
			break
		}
	}
	if r == nil {
		return schema.Schema{}, types.Object{}, fmt.Errorf("the provider has no resource %s", resourceType)
	}
	bound, isBound := r.(wireBound)
	importer, imports := r.(resource.ResourceWithImportState)
	if !isBound || !imports {
		return schema.Schema{}, types.Object{}, fmt.Errorf("%s does not import a Jellyfin JSON document", resourceType)
	}
	s := schemaOf(r)

	// The framework starts every import from this null state.
	resp := resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}}
	importer.ImportState(ctx, resource.ImportStateRequest{ID: importID}, &resp)
	if resp.Diagnostics.HasError() {
		return s, types.Object{}, fmt.Errorf("importing %s %q: %v", resourceType, importID, resp.Diagnostics)
	}
	imported, err := s.Type().ValueFromTerraform(ctx, resp.State.Raw)
	if err != nil {
		return s, types.Object{}, fmt.Errorf("importing %s %q: %w", resourceType, importID, err)
	}
	prior, ok := imported.(basetypes.ObjectValue)
	if !ok {
		return s, types.Object{}, fmt.Errorf("importing %s %q: the state is a %T", resourceType, importID, imported)
	}

	b, err := bound.Wire()
	if err != nil {
		return s, types.Object{}, err
	}
	doc, err := parseJSONObject(raw)
	if err != nil {
		return s, types.Object{}, fmt.Errorf("reading %s: %w", resourceType, err)
	}
	got, diags := b.Flatten(wire.WithAvailable(ctx, newOfferedProviders(c).byItemType), doc, prior)
	if diags.HasError() {
		return s, types.Object{}, fmt.Errorf("reading %s: %v", resourceType, diags)
	}
	return s, got, nil
}
