// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure JellyfinProvider satisfies various provider interfaces.
var _ provider.Provider = &JellyfinProvider{}

// JellyfinProvider defines the provider implementation.
type JellyfinProvider struct {
	version string
}

// JellyfinProviderModel describes the provider data model.
type JellyfinProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	APIKey   types.String `tfsdk:"api_key"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

func (p *JellyfinProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "jellyfin"
	resp.Version = p.version
}

func (p *JellyfinProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Jellyfin provider allows you to manage a Jellyfin media server instance. " +
			"It supports managing users, libraries, plugins, system configuration, and initial setup.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "The URL of the Jellyfin server (e.g., `http://localhost:8096`). " +
					"Can also be set via the `JELLYFIN_ENDPOINT` environment variable.",
				Optional: true,
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "The API key for authenticating with the Jellyfin server. " +
					"Can also be set via the `JELLYFIN_API_KEY` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Username for authenticating with the Jellyfin server (used during initial setup). " +
					"Can also be set via the `JELLYFIN_USERNAME` environment variable.",
				Optional: true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password for authenticating with the Jellyfin server (used during initial setup). " +
					"Can also be set via the `JELLYFIN_PASSWORD` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
		},
	}
}

func (p *JellyfinProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data JellyfinProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := os.Getenv("JELLYFIN_ENDPOINT")
	if !data.Endpoint.IsNull() {
		endpoint = data.Endpoint.ValueString()
	}

	apiKey := os.Getenv("JELLYFIN_API_KEY")
	if !data.APIKey.IsNull() {
		apiKey = data.APIKey.ValueString()
	}

	username := os.Getenv("JELLYFIN_USERNAME")
	if !data.Username.IsNull() {
		username = data.Username.ValueString()
	}

	password := os.Getenv("JELLYFIN_PASSWORD")
	if !data.Password.IsNull() {
		password = data.Password.ValueString()
	}

	if endpoint == "" {
		resp.Diagnostics.AddError(
			"Missing Jellyfin Endpoint",
			"The provider cannot create the Jellyfin API client because the endpoint is missing. "+
				"Set the endpoint in the provider configuration or via the JELLYFIN_ENDPOINT environment variable.",
		)
		return
	}

	c := client.NewClient(endpoint, apiKey)

	// If no API key is set but we have credentials, authenticate to get one.
	if apiKey == "" && username != "" {
		authResult, err := c.AuthenticateByName(username, password)
		if err != nil {
			resp.Diagnostics.AddError(
				"Authentication Failed",
				"Failed to authenticate with Jellyfin: "+err.Error(),
			)
			return
		}
		c.APIKey = authResult.AccessToken
	}

	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *JellyfinProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewUserResource,
		NewLibraryResource,
		NewPluginRepositoryResource,
		NewPluginResource,
		NewPluginConfigurationResource,
		NewSystemConfigurationResource,
		NewEncodingConfigurationResource,
	}
}

func (p *JellyfinProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewSystemInfoDataSource,
	}
}

// New creates a new provider factory function.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &JellyfinProvider{
			version: version,
		}
	}
}
