// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

// offeredProviders answers the Complement attributes' wire.AvailableFunc from
// GET /Libraries/AvailableOptions, asking once per content type.
type offeredProviders struct {
	c      *client.Client
	served map[string]*client.AvailableLibraryOptions
}

func newOfferedProviders(c *client.Client) *offeredProviders {
	return &offeredProviders{c: c, served: map[string]*client.AvailableLibraryOptions{}}
}

func (o *offeredProviders) forContentType(ctx context.Context, contentType string) (*client.AvailableLibraryOptions, error) {
	if served, ok := o.served[contentType]; ok {
		return served, nil
	}
	if o.c == nil {
		return nil, fmt.Errorf("no Jellyfin client to ask which providers the server offers")
	}
	served, err := o.c.GetAvailableLibraryOptions(ctx, contentType)
	if err != nil {
		return nil, err
	}
	o.served[contentType] = served
	return served, nil
}

func optionNames(options []client.AvailableOption) []string {
	out := make([]string, len(options))
	for i, o := range options {
		out[i] = o.Name
	}
	return out
}

// forLibrary answers the lists of a whole library of collectionType.
func (o *offeredProviders) forLibrary(collectionType string) wire.AvailableFunc {
	return func(ctx context.Context, offered, scope string) ([]string, error) {
		if offered != "SubtitleFetchers" || scope != "" {
			return nil, fmt.Errorf("a library lists no %s for %q", offered, scope)
		}
		served, err := o.forContentType(ctx, collectionType)
		if err != nil {
			return nil, err
		}
		return optionNames(served.SubtitleFetchers), nil
	}
}

// contentTypesByItemType are library content types that between them hold
// every item type Jellyfin lists providers of, ordered to find those of a
// fresh server's metadata options in few requests.
var contentTypesByItemType = []string{"movies", "tvshows", "music", "boxsets", "books", "homevideos", "playlists"}

// byItemType answers the lists of the item type scope names, from the first
// content type whose libraries hold that item type.
func (o *offeredProviders) byItemType(ctx context.Context, offered, scope string) ([]string, error) {
	for _, contentType := range contentTypesByItemType {
		served, err := o.forContentType(ctx, contentType)
		if err != nil {
			return nil, err
		}
		for _, t := range served.TypeOptions {
			if !strings.EqualFold(t.Type, scope) {
				continue
			}
			switch offered {
			case "MetadataFetchers":
				return optionNames(t.MetadataFetchers), nil
			case "ImageFetchers":
				return optionNames(t.ImageFetchers), nil
			}
			return nil, fmt.Errorf("an item type lists no %s", offered)
		}
	}
	return nil, fmt.Errorf("item type %q: %w", scope, wire.ErrNotOffered)
}

// combinedStringList is an optional list that replaces the list attributes of
// the same object named replaced, whose Jellyfin keys it writes, so it
// conflicts with them.
func combinedStringList(desc string, replaced ...string) schema.ListAttribute {
	conflicts := make([]path.Expression, len(replaced))
	for i, r := range replaced {
		conflicts[i] = path.MatchRelative().AtParent().AtName(r)
	}
	return schema.ListAttribute{
		ElementType:         types.StringType,
		Description:         desc,
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		Validators:          []validator.List{listvalidator.ConflictsWith(conflicts...)},
		PlanModifiers: []planmodifier.List{
			listplanmodifier.UseNonNullStateForUnknown(),
			unknownWhileSharedKeysChange{siblings: replaced},
		},
	}
}

// replacedBy deprecates a, a list attribute that replacement now writes the
// Jellyfin key of.
func replacedBy(a schema.ListAttribute, deprecation, replacement string) schema.ListAttribute {
	a.Description += " " + deprecation
	a.MarkdownDescription += " " + deprecation
	a.DeprecationMessage = deprecation
	a.PlanModifiers = append(a.PlanModifiers, unknownWhileSharedKeysChange{siblings: []string{replacement}})
	return a
}

// unknownWhileSharedKeysChange plans a list attribute that the configuration
// leaves unset as unknown while one of siblings, attributes of the same object
// that write the same Jellyfin keys, is configured to a value other than its
// prior one. Once unknown, the attribute reads back what the sibling makes of
// the keys, and leaves the keys to it.
type unknownWhileSharedKeysChange struct {
	siblings []string
}

func (m unknownWhileSharedKeysChange) Description(_ context.Context) string {
	return "Unset, the value is unknown while " + strings.Join(m.siblings, " or ") + " changes."
}

func (m unknownWhileSharedKeysChange) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m unknownWhileSharedKeysChange) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if !req.ConfigValue.IsNull() || req.State.Raw.IsNull() {
		return
	}
	for _, name := range m.siblings {
		p := req.Path.ParentPath().AtName(name)
		var configured, prior types.List
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, p, &configured)...)
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, p, &prior)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if changes(configured, prior) {
			resp.PlanValue = types.ListUnknown(req.PlanValue.ElementType(ctx))
			return
		}
	}
}

// changes reports whether configured, an attribute's configured value, sets it
// to other than prior; an unset attribute changes nothing.
func changes(configured, prior types.List) bool {
	return !configured.IsNull() && !configured.Equal(prior)
}
