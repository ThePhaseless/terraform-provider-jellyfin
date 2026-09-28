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
			unknownWhileSharedKeyChanges(replaced...),
		},
	}
}

// replacedBy deprecates a, a list attribute that replacement now writes the
// Jellyfin key of.
func replacedBy(a schema.ListAttribute, deprecation, replacement string) schema.ListAttribute {
	a.Description += " " + deprecation
	a.MarkdownDescription += " " + deprecation
	a.DeprecationMessage = deprecation
	a.PlanModifiers = append(a.PlanModifiers, unknownWhileSharedKeyChanges(replacement))
	return a
}

// unknownWhileSharedKeyChanges plans a list attribute that the configuration
// leaves unset as unknown while one of the named attributes of the same object
// is configured to a value other than its prior one. They write the same
// Jellyfin keys, so the attribute reads back what that change makes of them;
// once unknown, it also leaves the keys to the attribute that changes.
func unknownWhileSharedKeyChanges(siblings ...string) planmodifier.List {
	return unknownWhileSharedKeyChangesModifier{siblings: siblings}
}

type unknownWhileSharedKeyChangesModifier struct {
	siblings []string
}

func (m unknownWhileSharedKeyChangesModifier) Description(_ context.Context) string {
	return "Unset, the value is unknown while " + strings.Join(m.siblings, " or ") + " changes."
}

func (m unknownWhileSharedKeyChangesModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m unknownWhileSharedKeyChangesModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
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
		if !configured.IsNull() && !configured.Equal(prior) {
			resp.PlanValue = types.ListUnknown(req.PlanValue.ElementType(ctx))
			return
		}
	}
}
