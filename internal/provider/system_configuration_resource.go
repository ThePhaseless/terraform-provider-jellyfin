// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SystemConfigurationResource{}

// NewSystemConfigurationResource creates a new system configuration resource.
func NewSystemConfigurationResource() resource.Resource {
	return &SystemConfigurationResource{}
}

// SystemConfigurationResource defines the resource implementation.
type SystemConfigurationResource struct {
	client *client.Client
}

// SystemConfigurationResourceModel describes the resource data model.
type SystemConfigurationResourceModel struct {
	ServerName        types.String         `tfsdk:"server_name"`
	ConfigurationJSON jsontypes.Normalized `tfsdk:"configuration_json"`
}

func (r *SystemConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_configuration"
}

func (r *SystemConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jellyfin system configuration. Can be used for initial setup and ongoing configuration. " +
			"The `configuration_json` attribute accepts the full system configuration as JSON, allowing " +
			"complete control over all settings.",
		Attributes: map[string]schema.Attribute{
			"server_name": schema.StringAttribute{
				MarkdownDescription: "The server display name.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"configuration_json": schema.StringAttribute{
				MarkdownDescription: "The full system configuration as a JSON string. " +
					"When provided, it will be merged with the existing configuration.",
				Optional:   true,
				Computed:   true,
				CustomType: jsontypes.NormalizedType{},
			},
		},
	}
}

func (r *SystemConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}

	r.client = c
}

func (r *SystemConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SystemConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.applyConfiguration(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *SystemConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SystemConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, err := r.client.GetSystemConfiguration()
	if err != nil {
		resp.Diagnostics.AddError("Failed to read system configuration", err.Error())
		return
	}

	data.ServerName = types.StringValue(config.ServerName)
	normalized, err := normalizeJSON(config.RawJSON)
	if err != nil {
		resp.Diagnostics.AddError("Failed to normalize system configuration", err.Error())
		return
	}
	data.ConfigurationJSON = jsontypes.NewNormalizedValue(normalized)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SystemConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SystemConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.applyConfiguration(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *SystemConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// System configuration cannot be deleted. We just remove from state.
}

func (r *SystemConfigurationResource) applyConfiguration(ctx context.Context, data *SystemConfigurationResourceModel, diagnostics *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetSystemConfiguration()
	if err != nil {
		diagnostics.AddError("Failed to read current system configuration", err.Error())
		return
	}

	rawJSON := current.RawJSON

	if !data.ConfigurationJSON.IsNull() && data.ConfigurationJSON.ValueString() != "" {
		merged, mergeErr := mergeJSON(rawJSON, data.ConfigurationJSON.ValueString())
		if mergeErr != nil {
			diagnostics.AddError("Failed to merge configuration JSON", mergeErr.Error())
			return
		}
		rawJSON = merged
	}

	if !data.ServerName.IsNull() && !data.ServerName.IsUnknown() {
		var parsed map[string]json.RawMessage
		if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil {
			diagnostics.AddError("Failed to parse configuration JSON", err.Error())
			return
		}
		nameJSON, _ := json.Marshal(data.ServerName.ValueString())
		parsed["ServerName"] = json.RawMessage(nameJSON)
		result, err := json.Marshal(parsed)
		if err != nil {
			diagnostics.AddError("Failed to serialize configuration JSON", err.Error())
			return
		}
		rawJSON = string(result)
	}

	config := &client.SystemConfiguration{RawJSON: rawJSON}
	if err := r.client.UpdateSystemConfiguration(config); err != nil {
		diagnostics.AddError("Failed to update system configuration", err.Error())
		return
	}

	updated, err := r.client.GetSystemConfiguration()
	if err != nil {
		diagnostics.AddError("Failed to read updated system configuration", err.Error())
		return
	}

	data.ServerName = types.StringValue(updated.ServerName)
	normalizedUpdated, normErr := normalizeJSON(updated.RawJSON)
	if normErr != nil {
		diagnostics.AddError("Failed to normalize system configuration", normErr.Error())
		return
	}
	data.ConfigurationJSON = jsontypes.NewNormalizedValue(normalizedUpdated)

	diagnostics.Append(state.Set(ctx, data)...)
}

// mergeJSON merges the override JSON into the base JSON. Override values take precedence.
func mergeJSON(base, override string) (string, error) {
	var baseMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(base), &baseMap); err != nil {
		return "", fmt.Errorf("parsing base JSON: %w", err)
	}

	var overrideMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(override), &overrideMap); err != nil {
		return "", fmt.Errorf("parsing override JSON: %w", err)
	}

	for k, v := range overrideMap {
		baseMap[k] = v
	}

	result, err := json.Marshal(baseMap)
	if err != nil {
		return "", fmt.Errorf("serializing merged JSON: %w", err)
	}

	return string(result), nil
}

// normalizeJSON re-encodes JSON through interface{} to produce canonical key ordering.
func normalizeJSON(raw string) (string, error) {
	var generic interface{}
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		return "", fmt.Errorf("parsing JSON for normalization: %w", err)
	}
	result, err := json.Marshal(generic)
	if err != nil {
		return "", fmt.Errorf("serializing normalized JSON: %w", err)
	}
	return string(result), nil
}
