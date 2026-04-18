// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &InitialSetupResource{}

// NewInitialSetupResource creates a new initial setup resource.
func NewInitialSetupResource() resource.Resource {
	return &InitialSetupResource{}
}

// InitialSetupResource defines the resource implementation.
type InitialSetupResource struct {
	client *client.Client
}

// InitialSetupResourceModel describes the resource data model.
type InitialSetupResourceModel struct {
	Username                  types.String `tfsdk:"username"`
	Password                  types.String `tfsdk:"password"`
	UICulture                 types.String `tfsdk:"ui_culture"`
	MetadataCountryCode       types.String `tfsdk:"metadata_country_code"`
	PreferredMetadataLanguage types.String `tfsdk:"preferred_metadata_language"`
}

func (r *InitialSetupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_initial_setup"
}

func (r *InitialSetupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplaceString := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Completes the Jellyfin initial setup wizard. This resource creates the first " +
			"administrator user and finalizes the startup configuration. The startup wizard can only be run once " +
			"per server, so all attributes force replacement and `Delete` is a no-op.",
		Attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{
				MarkdownDescription: "The username for the initial administrator account.",
				Required:            true,
				PlanModifiers:       requiresReplaceString,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "The password for the initial administrator account.",
				Required:            true,
				Sensitive:           true,
				PlanModifiers:       requiresReplaceString,
			},
			"ui_culture": schema.StringAttribute{
				MarkdownDescription: "The UI culture / locale to configure for the server.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("en-US"),
				PlanModifiers:       requiresReplaceString,
			},
			"metadata_country_code": schema.StringAttribute{
				MarkdownDescription: "The metadata country code to configure for the server.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("US"),
				PlanModifiers:       requiresReplaceString,
			},
			"preferred_metadata_language": schema.StringAttribute{
				MarkdownDescription: "The preferred metadata language to configure for the server.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("en"),
				PlanModifiers:       requiresReplaceString,
			},
		},
	}
}

func (r *InitialSetupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *InitialSetupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data InitialSetupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	info, err := r.client.GetPublicSystemInfo()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get public system info", err.Error())
		return
	}

	if info.StartupWizardCompleted {
		resp.Diagnostics.AddError(
			"Startup wizard already completed",
			"The Jellyfin startup wizard has already been completed for this server and cannot be run again.",
		)
		return
	}

	config := &client.StartupConfiguration{
		UICulture:                 data.UICulture.ValueString(),
		MetadataCountryCode:       data.MetadataCountryCode.ValueString(),
		PreferredMetadataLanguage: data.PreferredMetadataLanguage.ValueString(),
	}
	if err := r.client.UpdateStartupConfiguration(config); err != nil {
		resp.Diagnostics.AddError("Failed to update startup configuration", err.Error())
		return
	}

	if err := r.client.SetStartupUser(data.Username.ValueString(), data.Password.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to set initial admin user", err.Error())
		return
	}

	if err := r.client.CompleteStartupWizard(); err != nil {
		resp.Diagnostics.AddError("Failed to complete startup wizard", err.Error())
		return
	}

	// Authenticate so subsequent provider operations have a valid token.
	authResult, err := r.client.AuthenticateByName(data.Username.ValueString(), data.Password.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to authenticate after initial setup",
			"The startup wizard was completed, but authentication for the new admin user failed: "+err.Error(),
		)
		return
	}
	r.client.APIKey = authResult.AccessToken

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InitialSetupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data InitialSetupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	info, err := r.client.GetPublicSystemInfo()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get public system info", err.Error())
		return
	}

	if !info.StartupWizardCompleted {
		// The setup was somehow undone or never persisted. Remove from state.
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InitialSetupResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All attributes require replacement, so Update should never be called.
	resp.Diagnostics.AddError("Update not supported", "Initial setup attributes require resource replacement.")
}

func (r *InitialSetupResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// The Jellyfin startup wizard cannot be undone. We just remove from state.
}
