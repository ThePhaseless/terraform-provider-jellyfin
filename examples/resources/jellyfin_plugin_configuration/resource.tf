# Configure any plugin using raw JSON.
# This example shows SSO-Auth plugin configuration.
resource "jellyfin_plugin_configuration" "sso_auth" {
  plugin_id = jellyfin_plugin.sso_auth.id

  configuration_json = jsonencode({
    SamlConfigs = []
    OidConfigs = [
      {
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
    ]
  })
}
