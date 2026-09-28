# Configure any plugin using raw JSON. Jellyfin replaces the plugin's whole
# configuration with it, so a key left out takes the plugin's default.
# This example configures the SSO-Auth plugin, whose OIDC and SAML providers
# are maps keyed by provider name.
resource "jellyfin_plugin_repository" "sso" {
  name = "SSO-Auth"
  url  = "https://raw.githubusercontent.com/9p4/jellyfin-plugin-sso/manifest-release/manifest.json"
}

resource "jellyfin_plugin" "sso_auth" {
  name           = "SSO-Auth"
  repository_url = jellyfin_plugin_repository.sso.url
}

# Jellyfin loads a newly installed plugin, and serves its configuration, only
# after a restart.
resource "jellyfin_restart" "sso_auth" {
  triggers = {
    plugin_version = jellyfin_plugin.sso_auth.installed_version
  }
}

resource "jellyfin_plugin_configuration" "sso_auth" {
  plugin_id  = jellyfin_plugin.sso_auth.id
  depends_on = [jellyfin_restart.sso_auth]

  configuration_json = jsonencode({
    SamlConfigs = {}
    OidConfigs = {
      authelia = {
        OidClientId       = "jellyfin"
        OidSecret         = "your-secret"
        OidEndpoint       = "https://auth.example.com"
        Enabled           = true
        EnableAllFolders  = true
        AdminRoles        = ["admin"]
        Roles             = ["user"]
        EnableFolderRoles = false
        FolderRoleMapping = []
      }
    }
  })
}
