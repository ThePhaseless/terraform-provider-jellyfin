// Copyright Sofie Finnes 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

const configurationOwnershipKey = "configuration_object_ownership_v1"

type configurationPrivate interface {
	SetKey(context.Context, string, []byte) diag.Diagnostics
}

func configurationObject(raw string) (map[string]any, error) {
	normalized, err := normalizeJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("configuration must be a valid JSON object")
	}
	value, err := decodeJSON(normalized)
	if err != nil {
		return nil, fmt.Errorf("configuration must be a valid JSON object")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("configuration must be a JSON object")
	}
	return object, nil
}

// The mask contains keys only. Remote values, especially secrets, must never
// enter private state merely because they share a document with owned fields.
func configurationOwnership(raw string) ([]byte, error) {
	object, err := configurationObject(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(ownershipShape(object))
}

func ownershipShape(value any) any {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	shape := make(map[string]any, len(object))
	for key, child := range object {
		shape[key] = ownershipShape(child)
	}
	return shape
}

func saveConfigurationOwnership(ctx context.Context, plan tfsdk.Plan, private configurationPrivate) diag.Diagnostics {
	var data PluginConfigurationResourceModel
	diagnostics := plan.Get(ctx, &data)
	if diagnostics.HasError() {
		return diagnostics
	}
	mask, err := configurationOwnership(data.Configuration.ValueString())
	if err != nil {
		diagnostics.AddError("Invalid configuration ownership", err.Error())
		return diagnostics
	}
	diagnostics.Append(private.SetKey(ctx, configurationOwnershipKey, mask)...)
	return diagnostics
}

func mergeConfiguration(current, desired string) (string, error) {
	object, err := configurationObject(current)
	if err != nil {
		return "", fmt.Errorf("server configuration must be a JSON object")
	}
	overlay, err := configurationObject(desired)
	if err != nil {
		return "", err
	}
	merged, err := json.Marshal(overlayConfiguration(object, overlay))
	if err != nil {
		return "", err
	}
	return normalizeJSON(string(merged))
}

func overlayConfiguration(current, desired map[string]any) map[string]any {
	for key, value := range desired {
		if object, ok := value.(map[string]any); ok {
			existing, ok := current[key].(map[string]any)
			if !ok {
				existing = make(map[string]any)
			}
			current[key] = overlayConfiguration(existing, object)
		} else {
			current[key] = value
		}
	}
	return current
}

type configurationObjectValidator struct{}

func (configurationObjectValidator) Description(context.Context) string {
	return "Configuration must be a JSON object."
}

func (v configurationObjectValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (configurationObjectValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := configurationObject(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid plugin configuration", err.Error())
	}
}

// A plugin error response can echo credentials from the submitted document.
func configurationError(err error) string {
	var response *client.HTTPError
	if errors.As(err, &response) {
		return fmt.Sprintf("%s %s returned status %d", response.Method, response.Path, response.StatusCode)
	}
	return err.Error()
}
