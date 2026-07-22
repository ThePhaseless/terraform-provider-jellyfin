resource "jellyfin_sso_plugin_configuration" "example" {
  plugin_id = jellyfin_plugin.sso_auth.id

  oid_configs = {
    authentik = {
      oid_endpoint                = "https://auth.example.com/application/o/jellyfin"
      oid_client_id               = "client-id"
      oid_secret                  = "client-secret"
      enabled                     = true
      enable_authorization        = true
      enable_all_folders          = true
      enabled_folders             = []
      admin_roles                 = ["admins"]
      roles                       = ["watchers", "admins"]
      enable_folder_roles         = false
      enable_live_tv_roles        = false
      enable_live_tv              = false
      enable_live_tv_management   = false
      live_tv_roles               = []
      live_tv_management_roles    = []
      folder_role_mapping         = []
      role_claim                  = "groups"
      oid_scopes                  = ["groups"]
      default_provider            = ""
      scheme_override             = "https"
      port_override               = 0
      new_path                    = true
      default_username_claim      = "preferred_username"
      avatar_url_format           = ""
      disable_https               = false
      disable_pushed_authorization = false
      do_not_validate_endpoints   = false
      do_not_validate_issuer_name = false
      do_not_load_profile         = false
    }
  }

  saml_configs = {}
}
