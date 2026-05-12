// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
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
		Description: "The Jellyfin provider allows you to manage a Jellyfin media server instance. " +
			"It supports managing users, libraries, plugins, system configuration, and initial setup.",
		MarkdownDescription: "The Jellyfin provider allows you to manage a Jellyfin media server instance. " +
			"It supports managing users, libraries, plugins, system configuration, and initial setup.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Description: "The URL of the Jellyfin server (e.g., `http://localhost:8096`). " +
					"Can also be set via the `JELLYFIN_ENDPOINT` environment variable.",
				MarkdownDescription: "The URL of the Jellyfin server (e.g., `http://localhost:8096`). " +
					"Can also be set via the `JELLYFIN_ENDPOINT` environment variable.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"api_key": schema.StringAttribute{
				Description: "The API key for authenticating with the Jellyfin server. " +
					"Can also be set via the `JELLYFIN_API_KEY` environment variable. " +
					"Use username and password instead when bootstrapping a new server.",
				MarkdownDescription: "The API key for authenticating with the Jellyfin server. " +
					"Can also be set via the `JELLYFIN_API_KEY` environment variable. " +
					"Use username and password instead when bootstrapping a new server.",
				Optional:  true,
				Sensitive: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"username": schema.StringAttribute{
				Description: "Username for authenticating with the Jellyfin server and creating the initial admin during bootstrap. " +
					"Can also be set via the `JELLYFIN_USERNAME` environment variable.",
				MarkdownDescription: "Username for authenticating with the Jellyfin server and creating the initial admin during bootstrap. " +
					"Can also be set via the `JELLYFIN_USERNAME` environment variable.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"password": schema.StringAttribute{
				Description: "Password for authenticating with the Jellyfin server and creating the initial admin during bootstrap. " +
					"Can also be set via the `JELLYFIN_PASSWORD` environment variable.",
				MarkdownDescription: "Password for authenticating with the Jellyfin server and creating the initial admin during bootstrap. " +
					"Can also be set via the `JELLYFIN_PASSWORD` environment variable.",
				Optional:  true,
				Sensitive: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
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
	if !data.Endpoint.IsNull() && !data.Endpoint.IsUnknown() {
		endpoint = data.Endpoint.ValueString()
	}

	apiKey := os.Getenv("JELLYFIN_API_KEY")
	if !data.APIKey.IsNull() && !data.APIKey.IsUnknown() {
		apiKey = data.APIKey.ValueString()
	}

	username := os.Getenv("JELLYFIN_USERNAME")
	if !data.Username.IsNull() && !data.Username.IsUnknown() {
		username = data.Username.ValueString()
	}

	password := os.Getenv("JELLYFIN_PASSWORD")
	if !data.Password.IsNull() && !data.Password.IsUnknown() {
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

	c, err := configureClient(ctx, endpoint, apiKey, username, password)
	if err != nil {
		resp.Diagnostics.AddError(
			"Jellyfin Configuration Failed",
			err.Error(),
		)
		return
	}

	resp.DataSourceData = c
	resp.ResourceData = c
}

func configureClient(ctx context.Context, endpoint, apiKey, username, password string) (*client.Client, error) {
	c := client.NewClient(endpoint, apiKey)

	info, err := c.GetPublicSystemInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("checking Jellyfin startup status: %w", err)
	}

	if !info.StartupWizardCompleted {
		if username == "" || password == "" {
			return nil, errors.New("Jellyfin has not been bootstrapped yet; set username and password in the provider configuration or via JELLYFIN_USERNAME and JELLYFIN_PASSWORD so the initial admin user can be created")
		}

		if err := c.UpdateStartupConfiguration(ctx, &client.StartupConfiguration{
			UICulture:                 "en-US",
			MetadataCountryCode:       "US",
			PreferredMetadataLanguage: "en",
		}); err != nil {
			return nil, err
		}
		if err := c.SetStartupUser(ctx, username, password); err != nil {
			return nil, err
		}
		if err := c.CompleteStartupWizard(ctx); err != nil {
			return nil, err
		}

		return authenticateClient(ctx, c, username, password)
	}

	if apiKey != "" {
		return c, nil
	}

	if username == "" && password == "" {
		return nil, errors.New("set api_key or username and password in the provider configuration, or via JELLYFIN_API_KEY or JELLYFIN_USERNAME and JELLYFIN_PASSWORD")
	}

	return authenticateClient(ctx, c, username, password)
}

func authenticateClient(ctx context.Context, c *client.Client, username, password string) (*client.Client, error) {
	if username == "" {
		return nil, errors.New("missing Jellyfin username; set it in the provider configuration or via JELLYFIN_USERNAME")
	}
	if password == "" {
		return nil, errors.New("missing Jellyfin password; set it in the provider configuration or via JELLYFIN_PASSWORD")
	}

	authResult, err := c.AuthenticateByName(ctx, username, password)
	if err != nil {
		return nil, fmt.Errorf("authenticating with Jellyfin: %w", err)
	}
	c.APIKey = authResult.AccessToken
	return c, nil
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
		NewNetworkingConfigurationResource,
		NewBrandingConfigurationResource,
		NewScheduledTaskResource,
		NewLiveTVConfigurationResource,
		NewMetadataConfigurationResource,
		NewAPIKeyResource,
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
