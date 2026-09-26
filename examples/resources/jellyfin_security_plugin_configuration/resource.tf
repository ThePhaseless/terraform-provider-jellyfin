resource "jellyfin_plugin_repository" "jellyfin_security" {
  name    = "JellyfinSecurity"
  url     = "https://raw.githubusercontent.com/ZL154/JellyfinSecurity/main/manifest.json"
  enabled = true
}

# Leaving version unset installs the release this provider was tested against,
# in the build the server accepts.
resource "jellyfin_plugin" "jellyfin_security" {
  name           = "Jellyfin Security"
  repository_url = jellyfin_plugin_repository.jellyfin_security.url
}

resource "jellyfin_restart" "jellyfin_security" {
  triggers = {
    plugin_version = jellyfin_plugin.jellyfin_security.version
  }
}

resource "jellyfin_security_plugin_configuration" "example" {
  plugin_id = jellyfin_plugin.jellyfin_security.id

  # Requiring 2FA for administrators also stops this provider signing in with
  # username and password; configure it with api_key before setting this.
  enforcement_scope = "Admins"

  public_base_url     = "https://jellyfin.example.com"
  trust_forwarded_for = true
  trusted_proxy_cidrs = ["10.0.0.0/8"]
  enrollment_deadline = "2030-01-01T00:00:00Z"

  oidc_providers = [{
    id                              = "authentik"
    display_name                    = "Authentik"
    preset                          = "authentik"
    discovery_url                   = "https://auth.example.com/application/o/jellyfin/.well-known/openid-configuration"
    client_id                       = "jellyfin"
    client_secret                   = var.oidc_client_secret
    scopes                          = ["openid", "profile", "email", "groups"]
    admin_groups                    = ["jellyfin-admins"]
    link_existing_users_by_username = true
  }]

  depends_on = [jellyfin_restart.jellyfin_security]
}

variable "oidc_client_secret" {
  type      = string
  sensitive = true
}
