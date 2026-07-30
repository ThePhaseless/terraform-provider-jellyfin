resource "jellyfin_plugin" "example" {
  name           = "Bookshelf"
  repository_url = "https://repo.jellyfin.org/files/plugin/manifest.json"
}

# Restart Jellyfin so the newly installed plugin loads. Any resource that
# configures the plugin should depend on this restart so it runs against the
# loaded plugin in the same apply:
#
#   resource "jellyfin_plugin_configuration" "example" {
#     plugin_id          = jellyfin_plugin.example.id
#     configuration_json = jsonencode({ /* plugin-specific */ })
#     depends_on         = [jellyfin_restart.example]
#   }
resource "jellyfin_restart" "example" {
  triggers = {
    plugin_version = jellyfin_plugin.example.version
  }
}
