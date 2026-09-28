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

// offersProviders is a wireBound resource whose Read asks the server which
// providers it offers, besides reading the document.
type offersProviders interface {
	offered(c *client.Client) wire.AvailableFunc
}

func resourceNamed(ctx context.Context, resourceType string) (resource.Resource, error) {
	for _, newResource := range New("import")().Resources(ctx) {
		r := newResource()
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "jellyfin"}, &meta)
		if meta.TypeName == resourceType {
			return r, nil
		}
	}
	return nil, fmt.Errorf("the provider has no resource %s", resourceType)
}

// ReadForImport returns the schema of resourceType and the values that
// terraform import with importID gives its attributes once the Read that
// follows has read raw, the JSON document Jellyfin serves for the resource.
// Only resources that read a Jellyfin document through a wire binding have
// one; an attribute the Read sets without the document, such as a computed
// id, holds what ImportState gave it. c answers what the Read asks the server
// besides the document, such as the providers it offers each item type.
func ReadForImport(ctx context.Context, c *client.Client, resourceType, importID, raw string) (schema.Schema, types.Object, error) {
	r, err := resourceNamed(ctx, resourceType)
	if err != nil {
		return schema.Schema{}, types.Object{}, err
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
	o, offers := r.(offersProviders)
	if !offers && asksOfferedNames(b) {
		return s, types.Object{}, fmt.Errorf("%s reads the providers Jellyfin offers each of its objects, which ReadForImport cannot ask for", resourceType)
	}
	doc, err := parseJSONObject(raw)
	if err != nil {
		return s, types.Object{}, fmt.Errorf("reading %s: %w", resourceType, err)
	}
	if offers {
		ctx = wire.WithAvailable(ctx, o.offered(c))
	}
	got, diags := b.Flatten(ctx, doc, prior)
	if diags.HasError() {
		return s, types.Object{}, fmt.Errorf("reading %s: %v", resourceType, diags)
	}
	return s, got, nil
}

// asksOfferedNames reports whether reading b asks for the names the server
// offers, as a Complement does.
func asksOfferedNames(b *wire.Binding) bool {
	for _, f := range b.Fields {
		if f.Offered != "" || f.Elem != nil && asksOfferedNames(f.Elem) {
			return true
		}
	}
	return false
}

// SharedKeys maps, by resource type, the dotted path of each attribute whose
// Jellyfin keys another attribute of the same object also writes, such as a
// deprecated attribute whose keys its replacement writes, to the name of that
// attribute.
func SharedKeys(ctx context.Context) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	var walk func(shared map[string]string, b *wire.Binding)
	walk = func(shared map[string]string, b *wire.Binding) {
		for _, f := range b.Fields {
			for _, s := range f.Shares {
				shared[s.Path] = f.Name
			}
			if f.Elem != nil {
				walk(shared, f.Elem)
			}
		}
	}
	for _, newResource := range New("import")().Resources(ctx) {
		r := newResource()
		bound, ok := r.(wireBound)
		if !ok {
			continue
		}
		b, err := bound.Wire()
		if err != nil {
			return nil, err
		}
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "jellyfin"}, &meta)
		shared := map[string]string{}
		walk(shared, b)
		if len(shared) > 0 {
			out[meta.TypeName] = shared
		}
	}
	return out, nil
}
