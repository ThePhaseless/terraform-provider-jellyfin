// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

var (
	_ resource.Resource                = &NetworkingConfigurationResource{}
	_ resource.ResourceWithImportState = &NetworkingConfigurationResource{}
)

// NewNetworkingConfigurationResource creates a new networking configuration resource.
func NewNetworkingConfigurationResource() resource.Resource {
	return &NetworkingConfigurationResource{}
}

// NetworkingConfigurationResource defines the resource implementation.
type NetworkingConfigurationResource struct {
	client *client.Client
}

// NetworkingConfigurationResourceModel describes the resource data model.
type NetworkingConfigurationResourceModel struct {
	ID                                types.String `tfsdk:"id"`
	BaseURL                           types.String `tfsdk:"base_url"`
	EnableHTTPS                       types.Bool   `tfsdk:"enable_https"`
	RequireHTTPS                      types.Bool   `tfsdk:"require_https"`
	CertificatePath                   types.String `tfsdk:"certificate_path"`
	CertificatePassword               types.String `tfsdk:"certificate_password"`
	InternalHTTPPort                  types.Int64  `tfsdk:"internal_http_port"`
	InternalHTTPSPort                 types.Int64  `tfsdk:"internal_https_port"`
	PublicHTTPPort                    types.Int64  `tfsdk:"public_http_port"`
	PublicHTTPSPort                   types.Int64  `tfsdk:"public_https_port"`
	AutoDiscovery                     types.Bool   `tfsdk:"auto_discovery"`
	EnableUpnp                        types.Bool   `tfsdk:"enable_upnp"`
	EnableIpv4                        types.Bool   `tfsdk:"enable_ipv4"`
	EnableIpv6                        types.Bool   `tfsdk:"enable_ipv6"`
	EnableRemoteAccess                types.Bool   `tfsdk:"enable_remote_access"`
	LocalNetworkSubnets               types.List   `tfsdk:"local_network_subnets"`
	LocalNetworkAddresses             types.List   `tfsdk:"local_network_addresses"`
	KnownProxies                      types.List   `tfsdk:"known_proxies"`
	IgnoreVirtualInterfaces           types.Bool   `tfsdk:"ignore_virtual_interfaces"`
	VirtualInterfaceNames             types.List   `tfsdk:"virtual_interface_names"`
	EnablePublishedServerURIByRequest types.Bool   `tfsdk:"enable_published_server_uri_by_request"`
	PublishedServerURIBySubnet        types.List   `tfsdk:"published_server_uri_by_subnet"`
	RemoteIPFilter                    types.List   `tfsdk:"remote_ip_filter"`
	IsRemoteIPFilterBlacklist         types.Bool   `tfsdk:"is_remote_ip_filter_blacklist"`
}

func (r *NetworkingConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_networking_configuration"
}

func (r *NetworkingConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages the Jellyfin networking configuration.",
		MarkdownDescription: "Manages the Jellyfin networking configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "Resource identifier. Always set to `networking` for this singleton resource.",
				MarkdownDescription: "Resource identifier. Always set to `networking` for this singleton resource.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"base_url":                               schema.StringAttribute{Description: "The base URL.", MarkdownDescription: "The base URL.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"enable_https":                           schema.BoolAttribute{Description: "Whether HTTPS is enabled.", MarkdownDescription: "Whether HTTPS is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"require_https":                          schema.BoolAttribute{Description: "Whether HTTPS is required.", MarkdownDescription: "Whether HTTPS is required.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"certificate_path":                       schema.StringAttribute{Description: "Path to the TLS certificate.", MarkdownDescription: "Path to the TLS certificate.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"certificate_password":                   schema.StringAttribute{Description: "Password for the TLS certificate.", MarkdownDescription: "Password for the TLS certificate.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, Sensitive: true},
			"internal_http_port":                     schema.Int64Attribute{Description: "Internal HTTP port.", MarkdownDescription: "Internal HTTP port.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"internal_https_port":                    schema.Int64Attribute{Description: "Internal HTTPS port.", MarkdownDescription: "Internal HTTPS port.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"public_http_port":                       schema.Int64Attribute{Description: "Public HTTP port.", MarkdownDescription: "Public HTTP port.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"public_https_port":                      schema.Int64Attribute{Description: "Public HTTPS port.", MarkdownDescription: "Public HTTPS port.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"auto_discovery":                         schema.BoolAttribute{Description: "Whether auto discovery is enabled.", MarkdownDescription: "Whether auto discovery is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_upnp":                            schema.BoolAttribute{Description: "Whether UPnP is enabled.", MarkdownDescription: "Whether UPnP is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_ipv4":                            schema.BoolAttribute{Description: "Whether IPv4 is enabled.", MarkdownDescription: "Whether IPv4 is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_ipv6":                            schema.BoolAttribute{Description: "Whether IPv6 is enabled.", MarkdownDescription: "Whether IPv6 is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"enable_remote_access":                   schema.BoolAttribute{Description: "Whether remote access is enabled.", MarkdownDescription: "Whether remote access is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"local_network_subnets":                  schema.ListAttribute{ElementType: types.StringType, Description: "Local network subnets.", MarkdownDescription: "Local network subnets.", Optional: true, Computed: true, PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}},
			"local_network_addresses":                schema.ListAttribute{ElementType: types.StringType, Description: "Local network addresses.", MarkdownDescription: "Local network addresses.", Optional: true, Computed: true, PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}},
			"known_proxies":                          schema.ListAttribute{ElementType: types.StringType, Description: "Known proxy addresses.", MarkdownDescription: "Known proxy addresses.", Optional: true, Computed: true, PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}},
			"ignore_virtual_interfaces":              schema.BoolAttribute{Description: "Whether virtual interfaces are ignored.", MarkdownDescription: "Whether virtual interfaces are ignored.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"virtual_interface_names":                schema.ListAttribute{ElementType: types.StringType, Description: "Virtual interface names.", MarkdownDescription: "Virtual interface names.", Optional: true, Computed: true, PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}},
			"enable_published_server_uri_by_request": schema.BoolAttribute{Description: "Whether published server URI by request is enabled.", MarkdownDescription: "Whether published server URI by request is enabled.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"published_server_uri_by_subnet":         schema.ListAttribute{ElementType: types.StringType, Description: "Published server URIs by subnet.", MarkdownDescription: "Published server URIs by subnet.", Optional: true, Computed: true, PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}},
			"remote_ip_filter":                       schema.ListAttribute{ElementType: types.StringType, Description: "Remote IP filter list.", MarkdownDescription: "Remote IP filter list.", Optional: true, Computed: true, PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}},
			"is_remote_ip_filter_blacklist":          schema.BoolAttribute{Description: "Whether the remote IP filter is a blacklist.", MarkdownDescription: "Whether the remote IP filter is a blacklist.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *NetworkingConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NetworkingConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NetworkingConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *NetworkingConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NetworkingConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *NetworkingConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NetworkingConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, &resp.State)
}

func (r *NetworkingConfigurationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Networking configuration cannot be deleted. We just remove from state.
}

func (r *NetworkingConfigurationResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Singleton resource — the import ID is not used. Read will populate all fields.
	data := NetworkingConfigurationResourceModel{ID: types.StringValue("networking")}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkingConfigurationResource) apply(ctx context.Context, data *NetworkingConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetNetworkConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read current networking configuration", err.Error())
		return
	}

	base, err := parseJSONObject(current.RawJSON)
	if err != nil {
		diags.AddError("Failed to parse current networking configuration", err.Error())
		return
	}

	d := overlayNetworkingConfiguration(ctx, base, data)
	if d.HasError() {
		diags.Append(d...)
		return
	}

	payload, err := json.Marshal(base)
	if err != nil {
		diags.AddError("Failed to serialize networking configuration", err.Error())
		return
	}

	if err := r.client.UpdateNetworkConfiguration(ctx, &client.NetworkConfiguration{RawJSON: string(payload)}); err != nil {
		diags.AddError("Failed to update networking configuration", err.Error())
		return
	}

	updated, err := r.client.GetNetworkConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read networking configuration after update", err.Error())
		return
	}

	flattenNetworkingConfiguration(ctx, updated.RawJSON, data, diags)
	data.ID = types.StringValue("networking")
	diags.Append(state.Set(ctx, data)...)
}

func (r *NetworkingConfigurationResource) read(ctx context.Context, data *NetworkingConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	current, err := r.client.GetNetworkConfiguration(ctx)
	if err != nil {
		diags.AddError("Failed to read networking configuration", err.Error())
		return
	}

	flattenNetworkingConfiguration(ctx, current.RawJSON, data, diags)
	data.ID = types.StringValue("networking")
	diags.Append(state.Set(ctx, data)...)
}

func overlayNetworkingConfiguration(ctx context.Context, m map[string]json.RawMessage, data *NetworkingConfigurationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	putJSONString(m, "BaseURL", data.BaseURL)
	putJSONBool(m, "EnableHTTPS", data.EnableHTTPS)
	putJSONBool(m, "RequireHTTPS", data.RequireHTTPS)
	putJSONString(m, "CertificatePath", data.CertificatePath)
	putJSONString(m, "CertificatePassword", data.CertificatePassword)
	putJSONInt64(m, "InternalHTTPPort", data.InternalHTTPPort)
	putJSONInt64(m, "InternalHTTPSPort", data.InternalHTTPSPort)
	putJSONInt64(m, "PublicHTTPPort", data.PublicHTTPPort)
	putJSONInt64(m, "PublicHTTPSPort", data.PublicHTTPSPort)
	putJSONBool(m, "AutoDiscovery", data.AutoDiscovery)
	putJSONBool(m, "EnableUPnP", data.EnableUpnp)
	putJSONBool(m, "EnableIPv4", data.EnableIpv4)
	putJSONBool(m, "EnableIPv6", data.EnableIpv6)
	putJSONBool(m, "EnableRemoteAccess", data.EnableRemoteAccess)
	if d := putJSONStringList(ctx, m, "LocalNetworkSubnets", data.LocalNetworkSubnets); d.HasError() {
		return d
	}
	if d := putJSONStringList(ctx, m, "LocalNetworkAddresses", data.LocalNetworkAddresses); d.HasError() {
		return d
	}
	if d := putJSONStringList(ctx, m, "KnownProxies", data.KnownProxies); d.HasError() {
		return d
	}
	putJSONBool(m, "IgnoreVirtualInterfaces", data.IgnoreVirtualInterfaces)
	if d := putJSONStringList(ctx, m, "VirtualInterfaceNames", data.VirtualInterfaceNames); d.HasError() {
		return d
	}
	putJSONBool(m, "EnablePublishedServerURIByRequest", data.EnablePublishedServerURIByRequest)
	if d := putJSONStringList(ctx, m, "PublishedServerURIBySubnet", data.PublishedServerURIBySubnet); d.HasError() {
		return d
	}
	if d := putJSONStringList(ctx, m, "RemoteIPFilter", data.RemoteIPFilter); d.HasError() {
		return d
	}
	putJSONBool(m, "IsRemoteIPFilterBlacklist", data.IsRemoteIPFilterBlacklist)
	return diags
}

func flattenNetworkingConfiguration(ctx context.Context, raw string, data *NetworkingConfigurationResourceModel, diags *diag.Diagnostics) {
	m, err := parseJSONObject(raw)
	if err != nil {
		diags.AddError("Failed to parse networking configuration", err.Error())
		return
	}
	data.BaseURL = getJSONString(m, "BaseURL")
	data.EnableHTTPS = getJSONBool(m, "EnableHTTPS")
	data.RequireHTTPS = getJSONBool(m, "RequireHTTPS")
	data.CertificatePath = getJSONString(m, "CertificatePath")
	data.CertificatePassword = getJSONString(m, "CertificatePassword")
	data.InternalHTTPPort = getJSONInt64(m, "InternalHTTPPort")
	data.InternalHTTPSPort = getJSONInt64(m, "InternalHTTPSPort")
	data.PublicHTTPPort = getJSONInt64(m, "PublicHTTPPort")
	data.PublicHTTPSPort = getJSONInt64(m, "PublicHTTPSPort")
	data.AutoDiscovery = getJSONBool(m, "AutoDiscovery")
	data.EnableUpnp = getJSONBool(m, "EnableUPnP")
	data.EnableIpv4 = getJSONBool(m, "EnableIPv4")
	data.EnableIpv6 = getJSONBool(m, "EnableIPv6")
	data.EnableRemoteAccess = getJSONBool(m, "EnableRemoteAccess")
	data.LocalNetworkSubnets, _ = getJSONStringList(ctx, m, "LocalNetworkSubnets")
	data.LocalNetworkAddresses, _ = getJSONStringList(ctx, m, "LocalNetworkAddresses")
	data.KnownProxies, _ = getJSONStringList(ctx, m, "KnownProxies")
	data.IgnoreVirtualInterfaces = getJSONBool(m, "IgnoreVirtualInterfaces")
	data.VirtualInterfaceNames, _ = getJSONStringList(ctx, m, "VirtualInterfaceNames")
	data.EnablePublishedServerURIByRequest = getJSONBool(m, "EnablePublishedServerURIByRequest")
	data.PublishedServerURIBySubnet, _ = getJSONStringList(ctx, m, "PublishedServerURIBySubnet")
	data.RemoteIPFilter, _ = getJSONStringList(ctx, m, "RemoteIPFilter")
	data.IsRemoteIPFilterBlacklist = getJSONBool(m, "IsRemoteIPFilterBlacklist")
}
