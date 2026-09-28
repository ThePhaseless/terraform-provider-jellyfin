// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"regexp"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/wire"
)

var (
	_ resource.Resource                = &NetworkingConfigurationResource{}
	_ resource.ResourceWithImportState = &NetworkingConfigurationResource{}
	_ resource.ResourceWithModifyPlan  = &NetworkingConfigurationResource{}
	_ wireBound                        = &NetworkingConfigurationResource{}
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

var networkingWire = sync.OnceValues(func() (*wire.Binding, error) {
	return wire.Bind(schemaOf(&NetworkingConfigurationResource{}), "NetworkConfiguration", wire.Identity("id"))
})

func (r *NetworkingConfigurationResource) Wire() (*wire.Binding, error) { return networkingWire() }

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
			"base_url":                               schema.StringAttribute{Description: "The base URL, such as /jellyfin: empty, or a path that starts with / and does not end with one, as Jellyfin stores it.", MarkdownDescription: "The base URL, such as `/jellyfin`: empty, or a path that starts with `/` and does not end with one, as Jellyfin stores it.", Optional: true, Computed: true, Validators: []validator.String{baseURLValidator}, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"enable_https":                           optionalBool("Whether HTTPS is enabled."),
			"require_https":                          optionalBool("Whether HTTPS is required."),
			"certificate_path":                       optionalString("Path to the TLS certificate."),
			"certificate_password":                   schema.StringAttribute{Description: "Password for the TLS certificate.", MarkdownDescription: "Password for the TLS certificate.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, Sensitive: true},
			"internal_http_port":                     optionalInt("Internal HTTP port."),
			"internal_https_port":                    optionalInt("Internal HTTPS port."),
			"public_http_port":                       optionalInt("Public HTTP port."),
			"public_https_port":                      optionalInt("Public HTTPS port."),
			"auto_discovery":                         optionalBool("Whether auto discovery is enabled."),
			"enable_upnp":                            optionalBool("Whether UPnP is enabled."),
			"enable_ipv4":                            optionalBool("Whether IPv4 is enabled."),
			"enable_ipv6":                            optionalBool("Whether IPv6 is enabled."),
			"enable_remote_access":                   optionalBool("Whether remote access is enabled."),
			"local_network_subnets":                  optionalStringList("Local network subnets."),
			"local_network_addresses":                optionalStringList("Local network addresses."),
			"known_proxies":                          optionalStringList("Known proxy addresses."),
			"ignore_virtual_interfaces":              optionalBool("Whether virtual interfaces are ignored."),
			"virtual_interface_names":                optionalStringList("Virtual interface names."),
			"enable_published_server_uri_by_request": optionalBool("Whether published server URI by request is enabled."),
			"published_server_uri_by_subnet":         optionalStringList("Published server URIs by subnet."),
			"remote_ip_filter":                       optionalStringList("Remote IP filter list."),
			"is_remote_ip_filter_blacklist":          optionalBool("Whether the remote IP filter is a blacklist."),
		},
	}
}

var baseURLValidator = stringvalidator.RegexMatches(regexp.MustCompile(`^(/.*[^/])?$`),
	"must be empty, or start with / and not end with /, such as /jellyfin")

func (r *NetworkingConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, "Resource", &resp.Diagnostics)
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
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue("networking"))...)
}

// ModifyPlan gates each configured field on the Jellyfin version it needs, so
// a field a later pin adds is checked without a change here.
func (r *NetworkingConfigurationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	if b := wireBinding(&resp.Diagnostics, networkingWire); b != nil {
		resp.Diagnostics.Append(checkServerHasFields(ctx, r.client, b, req.Config)...)
	}
}

func (r *NetworkingConfigurationResource) apply(ctx context.Context, data *NetworkingConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	b := wireBinding(diags, networkingWire)
	if b == nil || !r.document().write(ctx, b, data, diags) {
		return
	}
	data.ID = types.StringValue("networking")
	diags.Append(state.Set(ctx, data)...)
}

func (r *NetworkingConfigurationResource) read(ctx context.Context, data *NetworkingConfigurationResourceModel, diags *diag.Diagnostics, state *tfsdk.State) {
	b := wireBinding(diags, networkingWire)
	if b == nil || !r.document().read(ctx, b, data, diags) {
		return
	}
	data.ID = types.StringValue("networking")
	diags.Append(state.Set(ctx, data)...)
}

func (r *NetworkingConfigurationResource) document() document {
	return document{what: "networking configuration", get: r.client.GetNetworkConfiguration, put: r.client.UpdateNetworkConfiguration}
}
