// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

var (
	_ resource.Resource                = &UserResource{}
	_ resource.ResourceWithImportState = &UserResource{}
	_ resource.ResourceWithModifyPlan  = &UserResource{}
	_ wireBound                        = &UserResource{}
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

var userWire = sync.OnceValues(func() (*wire.Binding, error) {
	return wire.Bind(schemaOf(&UserResource{}), "UserDto",
		wire.Identity("id"),
		wire.Elsewhere("password", "written by POST /Users/Password"),
		wire.Document("Policy"),
		wire.Key("is_administrator", "Policy.IsAdministrator"),
		wire.Key("is_disabled", "Policy.IsDisabled"),
		wire.Key("enable_all_folders", "Policy.EnableAllFolders"),
		// Each has a default, so the plan always holds a value: a missing or
		// null flag reads as false, as reading it as null would plan a change.
		wire.ReadMissingAs("is_administrator", types.BoolValue(false)),
		wire.ReadMissingAs("is_disabled", types.BoolValue(false)),
		wire.ReadMissingAs("enable_all_folders", types.BoolValue(false)),
		wire.Unmanaged("AccessSchedule", "Id", "Jellyfin numbers access schedules itself"),
		wire.Unmanaged("AccessSchedule", "UserId", "Jellyfin fills in the user the policy belongs to"))
})

func (r *UserResource) Wire() (*wire.Binding, error) { return userWire() }

// userPolicyWire writes POST /Users/{id}/Policy, which takes the policy with
// the three top-level flags in it.
var userPolicyWire = sync.OnceValues(func() (*wire.Binding, error) {
	b, err := userWire()
	if err != nil {
		return nil, err
	}
	return b.Document("Policy")
})

// userNameWire writes a rename, which posts the whole user: selecting the
// name keeps the policy and flags out of that request.
var userNameWire = sync.OnceValues(func() (*wire.Binding, error) {
	b, err := userWire()
	if err != nil {
		return nil, err
	}
	return b.Select("name")
})

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

func userPolicyAttributes() map[string]schema.Attribute {
	defaultedString := func(desc, def string) schema.StringAttribute {
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

	// Null is the server's "no limit", so these are not computed: a computed
	// attribute would keep the prior limit when unset and could never be
	// cleared.
	nullableInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
		}
	}

	return map[string]schema.Attribute{
		"is_hidden":                     optionalBool("Whether the user is hidden from login screens."),
		"enable_collection_management":  optionalBool("Whether the user can manage collections."),
		"enable_subtitle_management":    optionalBool("Whether the user can manage subtitles."),
		"enable_lyric_management":       optionalBool("Whether the user can manage lyrics."),
		"max_parental_rating":           nullableInt("Maximum parental rating allowed for the user. When `policy` is set, leaving this unset or null means no limit."),
		"max_parental_sub_rating":       nullableInt("Maximum parental sub-rating allowed for the user. When `policy` is set, leaving this unset or null means no limit."),
		"blocked_tags":                  optionalStringList("Tags that are blocked for the user."),
		"allowed_tags":                  optionalStringList("Tags that are explicitly allowed for the user."),
		"enable_user_preference_access": optionalBool("Whether the user can access their own preferences."),
		// A schedule's attributes take no UseStateForUnknown, which pairs
		// list elements by index: removing a schedule would plan the next one
		// with its hours. The list fills them from the prior schedule with the
		// same day and hours instead.
		"access_schedules": schema.ListNestedAttribute{
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"day_of_week": elementString("Day of week for the schedule."),
					"start_hour":  elementFloat("Start hour of the schedule (0-24)."),
					"end_hour":    elementFloat("End hour of the schedule (0-24)."),
				},
			},
			Description:         "Access schedules restricting when the user can use the server.",
			MarkdownDescription: "Access schedules restricting when the user can use the server.",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
				useStateForUnknownByKey(
					[]string{"day_of_week", "start_hour", "end_hour"},
					[]string{"day_of_week", "start_hour"},
					[]string{"day_of_week", "end_hour"},
					[]string{"day_of_week"},
				),
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
		"enabled_channels":                     guidList(optionalStringList("Channels explicitly enabled for the user, by ID as Jellyfin lists it.")),
		"enable_all_channels":                  optionalBool("Whether all channels are enabled."),
		"enabled_folders":                      guidList(optionalStringList("Folders explicitly enabled for the user, by ID as Jellyfin lists it, such as a library's `item_id`.")),
		"login_attempts_before_lockout":        optionalInt("Number of failed login attempts before the account is locked."),
		"max_active_sessions":                  optionalInt("Maximum number of simultaneous sessions."),
		"enable_public_sharing":                optionalBool("Whether public sharing is enabled."),
		"blocked_media_folders":                guidList(optionalStringList("Media folders that are blocked, by ID as Jellyfin lists it, such as a library's `item_id`.")),
		"blocked_channels":                     guidList(optionalStringList("Channels that are blocked, by ID as Jellyfin lists it.")),
		"remote_client_bitrate_limit":          optionalInt("Remote client bitrate limit."),
		"authentication_provider_id":           defaultedString("Authentication provider ID.", ""),
		"password_reset_provider_id":           defaultedString("Password reset provider ID.", ""),
		"sync_play_access":                     defaultedString("SyncPlay access level.", ""),
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

// Jellyfin reads an ID in any of .NET's Guid spellings but lists it as 32
// lowercase hex digits, so any other spelling would read back otherwise.
var guidPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// guidList takes only IDs spelled as Jellyfin lists them.
func guidList(a schema.ListAttribute) schema.ListAttribute {
	a.Validators = append(a.Validators, listvalidator.ValueStringsAre(stringvalidator.RegexMatches(guidPattern,
		"must be an ID as Jellyfin lists it: 32 lowercase hex digits without dashes")))
	return a
}

func (r *UserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	b := wireBinding(&resp.Diagnostics, userWire)
	if b == nil {
		return
	}

	// policy is computed, so a plan without it carries an unknown object,
	// which the pointer field of the model cannot hold. Read the attributes
	// one by one and convert the policy only when it is known.
	var data UserResourceModel
	var policy types.Object
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("name"), &data.Name)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("password"), &data.Password)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("is_administrator"), &data.IsAdministrator)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("is_disabled"), &data.IsDisabled)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("enable_all_folders"), &data.EnableAllFolders)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("policy"), &policy)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !policy.IsNull() && !policy.IsUnknown() {
		var p UserPolicyModel
		resp.Diagnostics.Append(policy.As(ctx, &p, basetypes.ObjectAsOptions{})...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Policy = &p
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
	// Saved now so that a failure below leaves the user in state as tainted
	// rather than orphaned on the server, where its name would make every
	// later create fail.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), data.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A failed policy update still reads the user back, so the tainted state
	// holds the user as the server has it rather than only its id. From the
	// id alone, an untaint followed by an apply without refresh plans a null
	// policy, and Update fails with an inconsistent result.
	flatten := b.FlattenAfterApply
	if err := r.applyPolicy(ctx, &data, user.ID, &resp.Diagnostics); err != nil {
		resp.Diagnostics.AddError("Failed to update user policy", err.Error())
		// Nothing was written, so a planned value the server lacks is not one
		// it dropped.
		flatten = b.FlattenInto
	}

	raw, err := r.client.GetUserRaw(ctx, user.ID)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read user after creation", err.Error())
		return
	}
	resp.Diagnostics.Append(flatten(ctx, raw, &data)...)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	b := wireBinding(&resp.Diagnostics, userWire)
	if b == nil {
		return
	}

	var data UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	raw, err := r.client.GetUserRaw(ctx, data.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read user", err.Error())
		return
	}
	resp.Diagnostics.Append(b.FlattenInto(ctx, raw, &data)...)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	b := wireBinding(&resp.Diagnostics, userWire)
	if b == nil {
		return
	}

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

	if !data.Name.Equal(state.Name) {
		if err := r.renameUser(ctx, state.ID.ValueString(), data.Name.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to update user", err.Error())
			return
		}
	}

	if err := r.applyPolicy(ctx, &data, state.ID.ValueString(), &resp.Diagnostics); err != nil {
		resp.Diagnostics.AddError("Failed to update user policy", err.Error())
		return
	}

	if !data.Password.IsNull() && !data.Password.Equal(state.Password) {
		if err := r.client.UpdateUserPassword(ctx, state.ID.ValueString(), "", data.Password.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to update user password", err.Error())
			return
		}
	}

	raw, err := r.client.GetUserRaw(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read user after update", err.Error())
		return
	}
	data.ID = state.ID
	resp.Diagnostics.Append(b.FlattenAfterApply(ctx, raw, &data)...)

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

// ModifyPlan gates each configured field on the Jellyfin version it needs, so
// a field a later pin adds is checked without a change here.
func (r *UserResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	if b := wireBinding(&resp.Diagnostics, userWire); b != nil {
		resp.Diagnostics.Append(checkServerHasFields(ctx, r.client, b, req.Config)...)
	}
}

// renameUser posts the user read from the server back with only Name changed.
// The server also replaces the user's Configuration with the one in the body,
// so sending just the name would reset per-user settings such as language
// preferences.
func (r *UserResource) renameUser(ctx context.Context, id, name string) error {
	b, err := userNameWire()
	if err != nil {
		return err
	}

	raw, err := r.client.GetUserRaw(ctx, id)
	if err != nil {
		return err
	}

	user, err := parseJSONObject(raw)
	if err != nil {
		return fmt.Errorf("parsing user: %w", err)
	}
	if d := b.OverlayModel(ctx, user, &UserResourceModel{Name: types.StringValue(name)}); d.HasError() {
		return fmt.Errorf("writing the name: %s", d.Errors()[0].Detail())
	}

	payloadBytes, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("marshaling user: %w", err)
	}

	return r.client.UpdateUserRaw(ctx, id, string(payloadBytes))
}

// applyPolicy overlays the planned top-level booleans and typed policy onto the
// existing server policy, then POSTs the result to /Users/{id}/Policy.
func (r *UserResource) applyPolicy(ctx context.Context, data *UserResourceModel, id string, diags *diag.Diagnostics) error {
	b, err := userPolicyWire()
	if err != nil {
		return err
	}

	base, err := r.client.GetUserPolicyRaw(ctx, id)
	if err != nil {
		return err
	}

	baseMap, err := parseJSONObject(base)
	if err != nil {
		return fmt.Errorf("parsing existing policy: %w", err)
	}
	wasAdministrator := jsonTrue(baseMap[policyIsAdministrator])

	if d := b.OverlayModel(ctx, baseMap, data); d.HasError() {
		diags.Append(d...)
		return fmt.Errorf("overlaying policy")
	}

	// Jellyfin refuses to disable an administrator, going by the flag the
	// user holds before the write, so a policy that demotes and disables the
	// user at once is posted in two steps: the demotion, then the rest.
	if wasAdministrator && !jsonTrue(baseMap[policyIsAdministrator]) && jsonTrue(baseMap[policyIsDisabled]) {
		demoted := maps.Clone(baseMap)
		demoted[policyIsDisabled] = json.RawMessage("false")
		if err := r.postPolicy(ctx, id, demoted); err != nil {
			return err
		}
	}
	return r.postPolicy(ctx, id, baseMap)
}

// The policy keys applyPolicy reads to order the writes Jellyfin accepts.
const (
	policyIsAdministrator = "IsAdministrator"
	policyIsDisabled      = "IsDisabled"
)

func (r *UserResource) postPolicy(ctx context.Context, id string, policy map[string]json.RawMessage) error {
	payloadBytes, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("marshaling policy: %w", err)
	}
	return r.client.UpdateUserPolicyRaw(ctx, id, string(payloadBytes))
}

func jsonTrue(raw json.RawMessage) bool {
	var b bool
	return json.Unmarshal(raw, &b) == nil && b
}
