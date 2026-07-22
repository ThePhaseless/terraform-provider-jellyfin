// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

var (
	_ resource.Resource                = &UserResource{}
	_ resource.ResourceWithImportState = &UserResource{}
)

// NewUserResource creates a new user resource.
func NewUserResource() resource.Resource {
	return &UserResource{}
}

// UserResource defines the resource implementation.
type UserResource struct {
	client *client.Client
}

// UserResourceModel describes the resource data model.
type UserResourceModel struct {
	ID               types.String     `tfsdk:"id"`
	Name             types.String     `tfsdk:"name"`
	Password         types.String     `tfsdk:"password"`
	IsAdministrator  types.Bool       `tfsdk:"is_administrator"`
	IsDisabled       types.Bool       `tfsdk:"is_disabled"`
	EnableAllFolders types.Bool       `tfsdk:"enable_all_folders"`
	Policy           *UserPolicyModel `tfsdk:"policy"`
}

// UserPolicyModel describes the typed user policy data model.
// Top-level IsAdministrator, IsDisabled, and EnableAllFolders are managed outside
// this nested object. InvalidLoginAttemptCount is server-managed and excluded.
type UserPolicyModel struct {
	IsHidden                         types.Bool   `tfsdk:"is_hidden"`
	EnableCollectionManagement       types.Bool   `tfsdk:"enable_collection_management"`
	EnableSubtitleManagement         types.Bool   `tfsdk:"enable_subtitle_management"`
	EnableLyricManagement            types.Bool   `tfsdk:"enable_lyric_management"`
	MaxParentalRating                types.Int64  `tfsdk:"max_parental_rating"`
	MaxParentalSubRating             types.Int64  `tfsdk:"max_parental_sub_rating"`
	BlockedTags                      types.List   `tfsdk:"blocked_tags"`
	AllowedTags                      types.List   `tfsdk:"allowed_tags"`
	EnableUserPreferenceAccess       types.Bool   `tfsdk:"enable_user_preference_access"`
	AccessSchedules                  types.List   `tfsdk:"access_schedules"`
	BlockUnratedItems                types.List   `tfsdk:"block_unrated_items"`
	EnableRemoteControlOfOtherUsers  types.Bool   `tfsdk:"enable_remote_control_of_other_users"`
	EnableSharedDeviceControl        types.Bool   `tfsdk:"enable_shared_device_control"`
	EnableRemoteAccess               types.Bool   `tfsdk:"enable_remote_access"`
	EnableLiveTvManagement           types.Bool   `tfsdk:"enable_live_tv_management"`
	EnableLiveTvAccess               types.Bool   `tfsdk:"enable_live_tv_access"`
	EnableMediaPlayback              types.Bool   `tfsdk:"enable_media_playback"`
	EnableAudioPlaybackTranscoding   types.Bool   `tfsdk:"enable_audio_playback_transcoding"`
	EnableVideoPlaybackTranscoding   types.Bool   `tfsdk:"enable_video_playback_transcoding"`
	EnablePlaybackRemuxing           types.Bool   `tfsdk:"enable_playback_remuxing"`
	ForceRemoteSourceTranscoding     types.Bool   `tfsdk:"force_remote_source_transcoding"`
	EnableContentDeletion            types.Bool   `tfsdk:"enable_content_deletion"`
	EnableContentDeletionFromFolders types.List   `tfsdk:"enable_content_deletion_from_folders"`
	EnableContentDownloading         types.Bool   `tfsdk:"enable_content_downloading"`
	EnableSyncTranscoding            types.Bool   `tfsdk:"enable_sync_transcoding"`
	EnableMediaConversion            types.Bool   `tfsdk:"enable_media_conversion"`
	EnabledDevices                   types.List   `tfsdk:"enabled_devices"`
	EnableAllDevices                 types.Bool   `tfsdk:"enable_all_devices"`
	EnabledChannels                  types.List   `tfsdk:"enabled_channels"`
	EnableAllChannels                types.Bool   `tfsdk:"enable_all_channels"`
	EnabledFolders                   types.List   `tfsdk:"enabled_folders"`
	LoginAttemptsBeforeLockout       types.Int64  `tfsdk:"login_attempts_before_lockout"`
	MaxActiveSessions                types.Int64  `tfsdk:"max_active_sessions"`
	EnablePublicSharing              types.Bool   `tfsdk:"enable_public_sharing"`
	BlockedMediaFolders              types.List   `tfsdk:"blocked_media_folders"`
	BlockedChannels                  types.List   `tfsdk:"blocked_channels"`
	RemoteClientBitrateLimit         types.Int64  `tfsdk:"remote_client_bitrate_limit"`
	AuthenticationProviderID         types.String `tfsdk:"authentication_provider_id"`
	PasswordResetProviderID          types.String `tfsdk:"password_reset_provider_id"`
	SyncPlayAccess                   types.String `tfsdk:"sync_play_access"`
}

// UserAccessScheduleModel describes one access schedule entry.
type UserAccessScheduleModel struct {
	DayOfWeek types.String  `tfsdk:"day_of_week"`
	StartHour types.Float64 `tfsdk:"start_hour"`
	EndHour   types.Float64 `tfsdk:"end_hour"`
}

func userPolicyAttributes() map[string]schema.Attribute {
	optionalBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.UseStateForUnknown(),
			},
		}
	}

	optionalString := func(desc, def string) schema.StringAttribute {
		a := schema.StringAttribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		}
		if def != "" {
			a.Default = stringdefault.StaticString(def)
		}
		return a
	}

	optionalInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		}
	}

	optionalFloat := func(desc string) schema.Float64Attribute {
		return schema.Float64Attribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.Float64{
				float64planmodifier.UseStateForUnknown(),
			},
		}
	}

	optionalStringList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{
			ElementType:         types.StringType,
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		}
	}

	return map[string]schema.Attribute{
		"is_hidden":                     optionalBool("Whether the user is hidden from login screens."),
		"enable_collection_management":  optionalBool("Whether the user can manage collections."),
		"enable_subtitle_management":    optionalBool("Whether the user can manage subtitles."),
		"enable_lyric_management":       optionalBool("Whether the user can manage lyrics."),
		"max_parental_rating":           optionalInt("Maximum parental rating allowed for the user."),
		"max_parental_sub_rating":       optionalInt("Maximum parental sub-rating allowed for the user."),
		"blocked_tags":                  optionalStringList("Tags that are blocked for the user."),
		"allowed_tags":                  optionalStringList("Tags that are explicitly allowed for the user."),
		"enable_user_preference_access": optionalBool("Whether the user can access their own preferences."),
		"access_schedules": schema.ListNestedAttribute{
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"day_of_week": schema.StringAttribute{
						Description:         "Day of week for the schedule.",
						MarkdownDescription: "Day of week for the schedule.",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"start_hour": optionalFloat("Start hour of the schedule (0-24)."),
					"end_hour":   optionalFloat("End hour of the schedule (0-24)."),
				},
			},
			Description:         "Access schedules restricting when the user can use the server.",
			MarkdownDescription: "Access schedules restricting when the user can use the server.",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		},
		"block_unrated_items":                  optionalStringList("Item types that are blocked when unrated."),
		"enable_remote_control_of_other_users": optionalBool("Whether the user can remote-control other users' sessions."),
		"enable_shared_device_control":         optionalBool("Whether shared device control is enabled for the user."),
		"enable_remote_access":                 optionalBool("Whether remote access is enabled for the user."),
		"enable_live_tv_management":            optionalBool("Whether the user can manage live TV."),
		"enable_live_tv_access":                optionalBool("Whether the user can access live TV."),
		"enable_media_playback":                optionalBool("Whether media playback is enabled for the user."),
		"enable_audio_playback_transcoding":    optionalBool("Whether audio playback transcoding is enabled."),
		"enable_video_playback_transcoding":    optionalBool("Whether video playback transcoding is enabled."),
		"enable_playback_remuxing":             optionalBool("Whether playback remuxing is enabled."),
		"force_remote_source_transcoding":      optionalBool("Whether remote source transcoding is forced."),
		"enable_content_deletion":              optionalBool("Whether content deletion is enabled."),
		"enable_content_deletion_from_folders": optionalStringList("Folders from which the user may delete content."),
		"enable_content_downloading":           optionalBool("Whether content downloading is enabled."),
		"enable_sync_transcoding":              optionalBool("Whether sync transcoding is enabled."),
		"enable_media_conversion":              optionalBool("Whether media conversion is enabled."),
		"enabled_devices":                      optionalStringList("Devices explicitly enabled for the user."),
		"enable_all_devices":                   optionalBool("Whether all devices are enabled."),
		"enabled_channels":                     optionalStringList("Channels explicitly enabled for the user."),
		"enable_all_channels":                  optionalBool("Whether all channels are enabled."),
		"enabled_folders":                      optionalStringList("Folders explicitly enabled for the user."),
		"login_attempts_before_lockout":        optionalInt("Number of failed login attempts before the account is locked."),
		"max_active_sessions":                  optionalInt("Maximum number of simultaneous sessions."),
		"enable_public_sharing":                optionalBool("Whether public sharing is enabled."),
		"blocked_media_folders":                optionalStringList("Media folders that are blocked."),
		"blocked_channels":                     optionalStringList("Channels that are blocked."),
		"remote_client_bitrate_limit":          optionalInt("Remote client bitrate limit."),
		"authentication_provider_id":           optionalString("Authentication provider ID.", ""),
		"password_reset_provider_id":           optionalString("Password reset provider ID.", ""),
		"sync_play_access":                     optionalString("SyncPlay access level.", ""),
	}
}

func (r *UserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages a Jellyfin user.",
		MarkdownDescription: "Manages a Jellyfin user.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "The unique user identifier.",
				MarkdownDescription: "The unique user identifier.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description:         "The username.",
				MarkdownDescription: "The username.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"password": schema.StringAttribute{
				Description:         "The user password.",
				MarkdownDescription: "The user password.",
				Optional:            true,
				Sensitive:           true,
			},
			"is_administrator": schema.BoolAttribute{
				Description:         "Whether the user is an administrator.",
				MarkdownDescription: "Whether the user is an administrator.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"is_disabled": schema.BoolAttribute{
				Description:         "Whether the user is disabled.",
				MarkdownDescription: "Whether the user is disabled.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"enable_all_folders": schema.BoolAttribute{
				Description:         "Whether the user has access to all folders.",
				MarkdownDescription: "Whether the user has access to all folders.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"policy": schema.SingleNestedAttribute{
				Description: "Typed user policy settings. Excludes IsAdministrator, IsDisabled, " +
					"EnableAllFolders (managed at the top level) and InvalidLoginAttemptCount (server-managed).",
				MarkdownDescription: "Typed user policy settings. Excludes `IsAdministrator`, `IsDisabled`, " +
					"`EnableAllFolders` (managed at the top level) and `InvalidLoginAttemptCount` (server-managed).",
				Optional:   true,
				Computed:   true,
				Attributes: userPolicyAttributes(),
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *UserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	password := ""
	if !data.Password.IsNull() {
		password = data.Password.ValueString()
	}

	user, err := r.client.CreateUser(ctx, data.Name.ValueString(), password)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create user", err.Error())
		return
	}

	data.ID = types.StringValue(user.ID)

	if err := r.applyPolicy(ctx, &data, user.ID, &resp.Diagnostics); err != nil {
		resp.Diagnostics.AddError("Failed to update user policy", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := r.client.GetUserByID(ctx, data.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read user", err.Error())
		return
	}

	data.Name = types.StringValue(user.Name)
	data.IsAdministrator = types.BoolValue(user.Policy.IsAdministrator)
	data.IsDisabled = types.BoolValue(user.Policy.IsDisabled)
	data.EnableAllFolders = types.BoolValue(user.Policy.EnableAllFolders)

	policyRaw, err := r.client.GetUserPolicyRaw(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read user policy", err.Error())
		return
	}

	data.Policy = policyFromRaw(ctx, policyRaw, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	currentUser, err := r.client.GetUserByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read current user", err.Error())
		return
	}

	currentUser.Name = data.Name.ValueString()
	if err := r.client.UpdateUser(ctx, currentUser); err != nil {
		resp.Diagnostics.AddError("Failed to update user", err.Error())
		return
	}

	if err := r.applyPolicy(ctx, &data, state.ID.ValueString(), &resp.Diagnostics); err != nil {
		resp.Diagnostics.AddError("Failed to update user policy", err.Error())
		return
	}

	// Update password if changed.
	if !data.Password.IsNull() && !data.Password.Equal(state.Password) {
		if err := r.client.UpdateUserPassword(ctx, state.ID.ValueString(), "", data.Password.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to update user password", err.Error())
			return
		}
	}

	updatedUser, err := r.client.GetUserByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read user after update", err.Error())
		return
	}

	data.ID = state.ID
	data.Name = types.StringValue(updatedUser.Name)
	data.IsAdministrator = types.BoolValue(updatedUser.Policy.IsAdministrator)
	data.IsDisabled = types.BoolValue(updatedUser.Policy.IsDisabled)
	data.EnableAllFolders = types.BoolValue(updatedUser.Policy.EnableAllFolders)

	policyRaw, err := r.client.GetUserPolicyRaw(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read user policy after update", err.Error())
		return
	}
	data.Policy = policyFromRaw(ctx, policyRaw, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteUser(ctx, data.ID.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete user", err.Error())
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyPolicy overlays the planned top-level booleans and typed policy onto the
// existing server policy, then POSTs the result to /Users/{id}/Policy.
func (r *UserResource) applyPolicy(ctx context.Context, data *UserResourceModel, id string, diags *diag.Diagnostics) error {
	base, err := r.client.GetUserPolicyRaw(ctx, id)
	if err != nil {
		return err
	}

	baseMap, err := parseJSONObject(base)
	if err != nil {
		return fmt.Errorf("parsing existing policy: %w", err)
	}

	// Write top-level managed booleans.
	putJSONBool(baseMap, "IsAdministrator", data.IsAdministrator)
	putJSONBool(baseMap, "IsDisabled", data.IsDisabled)
	putJSONBool(baseMap, "EnableAllFolders", data.EnableAllFolders)

	// Overlay typed policy fields, if the nested block is configured.
	if data.Policy != nil {
		d := overlayPolicyIntoJSON(ctx, baseMap, data.Policy)
		if d.HasError() {
			diags.Append(d...)
			return fmt.Errorf("overlaying policy")
		}
	}

	payloadBytes, err := json.Marshal(baseMap)
	if err != nil {
		return fmt.Errorf("marshaling policy: %w", err)
	}

	return r.client.UpdateUserPolicyRaw(ctx, id, string(payloadBytes))
}

func overlayPolicyIntoJSON(ctx context.Context, m map[string]json.RawMessage, policy *UserPolicyModel) diag.Diagnostics {
	var diags diag.Diagnostics

	putJSONBool(m, "IsHidden", policy.IsHidden)
	putJSONBool(m, "EnableCollectionManagement", policy.EnableCollectionManagement)
	putJSONBool(m, "EnableSubtitleManagement", policy.EnableSubtitleManagement)
	putJSONBool(m, "EnableLyricManagement", policy.EnableLyricManagement)
	putJSONInt64(m, "MaxParentalRating", policy.MaxParentalRating)
	putJSONInt64(m, "MaxParentalSubRating", policy.MaxParentalSubRating)
	if d := putJSONStringList(ctx, m, "BlockedTags", policy.BlockedTags); d.HasError() {
		return append(diags, d...)
	}
	if d := putJSONStringList(ctx, m, "AllowedTags", policy.AllowedTags); d.HasError() {
		return append(diags, d...)
	}
	putJSONBool(m, "EnableUserPreferenceAccess", policy.EnableUserPreferenceAccess)
	if d := putAccessSchedules(ctx, m, policy.AccessSchedules); d.HasError() {
		return append(diags, d...)
	}
	if d := putJSONStringList(ctx, m, "BlockUnratedItems", policy.BlockUnratedItems); d.HasError() {
		return append(diags, d...)
	}
	putJSONBool(m, "EnableRemoteControlOfOtherUsers", policy.EnableRemoteControlOfOtherUsers)
	putJSONBool(m, "EnableSharedDeviceControl", policy.EnableSharedDeviceControl)
	putJSONBool(m, "EnableRemoteAccess", policy.EnableRemoteAccess)
	putJSONBool(m, "EnableLiveTvManagement", policy.EnableLiveTvManagement)
	putJSONBool(m, "EnableLiveTvAccess", policy.EnableLiveTvAccess)
	putJSONBool(m, "EnableMediaPlayback", policy.EnableMediaPlayback)
	putJSONBool(m, "EnableAudioPlaybackTranscoding", policy.EnableAudioPlaybackTranscoding)
	putJSONBool(m, "EnableVideoPlaybackTranscoding", policy.EnableVideoPlaybackTranscoding)
	putJSONBool(m, "EnablePlaybackRemuxing", policy.EnablePlaybackRemuxing)
	putJSONBool(m, "ForceRemoteSourceTranscoding", policy.ForceRemoteSourceTranscoding)
	putJSONBool(m, "EnableContentDeletion", policy.EnableContentDeletion)
	if d := putJSONStringList(ctx, m, "EnableContentDeletionFromFolders", policy.EnableContentDeletionFromFolders); d.HasError() {
		return append(diags, d...)
	}
	putJSONBool(m, "EnableContentDownloading", policy.EnableContentDownloading)
	putJSONBool(m, "EnableSyncTranscoding", policy.EnableSyncTranscoding)
	putJSONBool(m, "EnableMediaConversion", policy.EnableMediaConversion)
	if d := putJSONStringList(ctx, m, "EnabledDevices", policy.EnabledDevices); d.HasError() {
		return append(diags, d...)
	}
	putJSONBool(m, "EnableAllDevices", policy.EnableAllDevices)
	if d := putJSONStringList(ctx, m, "EnabledChannels", policy.EnabledChannels); d.HasError() {
		return append(diags, d...)
	}
	putJSONBool(m, "EnableAllChannels", policy.EnableAllChannels)
	if d := putJSONStringList(ctx, m, "EnabledFolders", policy.EnabledFolders); d.HasError() {
		return append(diags, d...)
	}
	putJSONInt64(m, "LoginAttemptsBeforeLockout", policy.LoginAttemptsBeforeLockout)
	putJSONInt64(m, "MaxActiveSessions", policy.MaxActiveSessions)
	putJSONBool(m, "EnablePublicSharing", policy.EnablePublicSharing)
	if d := putJSONStringList(ctx, m, "BlockedMediaFolders", policy.BlockedMediaFolders); d.HasError() {
		return append(diags, d...)
	}
	if d := putJSONStringList(ctx, m, "BlockedChannels", policy.BlockedChannels); d.HasError() {
		return append(diags, d...)
	}
	putJSONInt64(m, "RemoteClientBitrateLimit", policy.RemoteClientBitrateLimit)
	putJSONString(m, "AuthenticationProviderId", policy.AuthenticationProviderID)
	putJSONString(m, "PasswordResetProviderId", policy.PasswordResetProviderID)
	putJSONString(m, "SyncPlayAccess", policy.SyncPlayAccess)

	return diags
}

func putAccessSchedules(ctx context.Context, m map[string]json.RawMessage, v types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if v.IsNull() || v.IsUnknown() {
		return diags
	}

	var elements []UserAccessScheduleModel
	if d := v.ElementsAs(ctx, &elements, false); d.HasError() {
		return append(diags, d...)
	}

	rawEntries := make([]map[string]json.RawMessage, len(elements))
	for i, e := range elements {
		entry := map[string]json.RawMessage{}
		putJSONString(entry, "DayOfWeek", e.DayOfWeek)
		putJSONFloat64(entry, "StartHour", e.StartHour)
		putJSONFloat64(entry, "EndHour", e.EndHour)
		rawEntries[i] = entry
	}

	b, err := json.Marshal(rawEntries)
	if err != nil {
		return append(diags, diag.NewErrorDiagnostic("Failed to marshal access schedules", err.Error()))
	}
	m["AccessSchedules"] = b
	return diags
}

func policyFromRaw(ctx context.Context, raw string, diags *diag.Diagnostics) *UserPolicyModel {
	m, err := parseJSONObject(raw)
	if err != nil {
		return nil
	}

	policy := &UserPolicyModel{}
	policy.IsHidden = getJSONBool(m, "IsHidden")
	policy.EnableCollectionManagement = getJSONBool(m, "EnableCollectionManagement")
	policy.EnableSubtitleManagement = getJSONBool(m, "EnableSubtitleManagement")
	policy.EnableLyricManagement = getJSONBool(m, "EnableLyricManagement")
	policy.MaxParentalRating = getJSONInt64(m, "MaxParentalRating")
	policy.MaxParentalSubRating = getJSONInt64(m, "MaxParentalSubRating")
	policy.BlockedTags, _ = getJSONStringList(ctx, m, "BlockedTags")
	policy.AllowedTags, _ = getJSONStringList(ctx, m, "AllowedTags")
	policy.EnableUserPreferenceAccess = getJSONBool(m, "EnableUserPreferenceAccess")
	policy.AccessSchedules = getAccessSchedules(ctx, m, diags)
	policy.BlockUnratedItems, _ = getJSONStringList(ctx, m, "BlockUnratedItems")
	policy.EnableRemoteControlOfOtherUsers = getJSONBool(m, "EnableRemoteControlOfOtherUsers")
	policy.EnableSharedDeviceControl = getJSONBool(m, "EnableSharedDeviceControl")
	policy.EnableRemoteAccess = getJSONBool(m, "EnableRemoteAccess")
	policy.EnableLiveTvManagement = getJSONBool(m, "EnableLiveTvManagement")
	policy.EnableLiveTvAccess = getJSONBool(m, "EnableLiveTvAccess")
	policy.EnableMediaPlayback = getJSONBool(m, "EnableMediaPlayback")
	policy.EnableAudioPlaybackTranscoding = getJSONBool(m, "EnableAudioPlaybackTranscoding")
	policy.EnableVideoPlaybackTranscoding = getJSONBool(m, "EnableVideoPlaybackTranscoding")
	policy.EnablePlaybackRemuxing = getJSONBool(m, "EnablePlaybackRemuxing")
	policy.ForceRemoteSourceTranscoding = getJSONBool(m, "ForceRemoteSourceTranscoding")
	policy.EnableContentDeletion = getJSONBool(m, "EnableContentDeletion")
	policy.EnableContentDeletionFromFolders, _ = getJSONStringList(ctx, m, "EnableContentDeletionFromFolders")
	policy.EnableContentDownloading = getJSONBool(m, "EnableContentDownloading")
	policy.EnableSyncTranscoding = getJSONBool(m, "EnableSyncTranscoding")
	policy.EnableMediaConversion = getJSONBool(m, "EnableMediaConversion")
	policy.EnabledDevices, _ = getJSONStringList(ctx, m, "EnabledDevices")
	policy.EnableAllDevices = getJSONBool(m, "EnableAllDevices")
	policy.EnabledChannels, _ = getJSONStringList(ctx, m, "EnabledChannels")
	policy.EnableAllChannels = getJSONBool(m, "EnableAllChannels")
	policy.EnabledFolders, _ = getJSONStringList(ctx, m, "EnabledFolders")
	policy.LoginAttemptsBeforeLockout = getJSONInt64(m, "LoginAttemptsBeforeLockout")
	policy.MaxActiveSessions = getJSONInt64(m, "MaxActiveSessions")
	policy.EnablePublicSharing = getJSONBool(m, "EnablePublicSharing")
	policy.BlockedMediaFolders, _ = getJSONStringList(ctx, m, "BlockedMediaFolders")
	policy.BlockedChannels, _ = getJSONStringList(ctx, m, "BlockedChannels")
	policy.RemoteClientBitrateLimit = getJSONInt64(m, "RemoteClientBitrateLimit")
	policy.AuthenticationProviderID = getJSONString(m, "AuthenticationProviderId")
	policy.PasswordResetProviderID = getJSONString(m, "PasswordResetProviderId")
	policy.SyncPlayAccess = getJSONString(m, "SyncPlayAccess")

	return policy
}

func getAccessSchedules(_ context.Context, m map[string]json.RawMessage, diags *diag.Diagnostics) types.List {
	raw, ok := m["AccessSchedules"]
	if !ok {
		return types.ListNull(types.ObjectType{AttrTypes: map[string]attr.Type{
			"day_of_week": types.StringType,
			"start_hour":  types.Float64Type,
			"end_hour":    types.Float64Type,
		}})
	}

	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags.AddError("Failed to parse access schedules", err.Error())
		return types.ListNull(types.ObjectType{AttrTypes: map[string]attr.Type{
			"day_of_week": types.StringType,
			"start_hour":  types.Float64Type,
			"end_hour":    types.Float64Type,
		}})
	}

	objects := make([]types.Object, len(entries))
	for i, entry := range entries {
		attrs := map[string]attr.Value{
			"day_of_week": getJSONString(entry, "DayOfWeek"),
			"start_hour":  getJSONFloat64(entry, "StartHour"),
			"end_hour":    getJSONFloat64(entry, "EndHour"),
		}
		obj, d := types.ObjectValue(map[string]attr.Type{
			"day_of_week": types.StringType,
			"start_hour":  types.Float64Type,
			"end_hour":    types.Float64Type,
		}, attrs)
		if d.HasError() {
			diags.Append(d...)
			return types.ListNull(types.ObjectType{AttrTypes: map[string]attr.Type{
				"day_of_week": types.StringType,
				"start_hour":  types.Float64Type,
				"end_hour":    types.Float64Type,
			}})
		}
		objects[i] = obj
	}

	values := make([]attr.Value, len(objects))
	for i, obj := range objects {
		values[i] = obj
	}
	list, d := types.ListValue(types.ObjectType{AttrTypes: map[string]attr.Type{
		"day_of_week": types.StringType,
		"start_hour":  types.Float64Type,
		"end_hour":    types.Float64Type,
	}}, values)
	if d.HasError() {
		diags.Append(d...)
		return types.ListNull(types.ObjectType{AttrTypes: map[string]attr.Type{
			"day_of_week": types.StringType,
			"start_hour":  types.Float64Type,
			"end_hour":    types.Float64Type,
		}})
	}
	return list
}
