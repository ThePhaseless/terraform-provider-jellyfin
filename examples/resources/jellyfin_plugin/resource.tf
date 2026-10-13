resource "jellyfin_plugin" "example" {
  name           = "Bookshelf"
  version        = "13.0.0.0"
  repository_url = "https://repo.jellyfin.org/files/plugin/manifest.json"

  # The Update Plugins schedule decides whether version is a minimum or exact,
  # so apply it before the plugin.
  depends_on = [jellyfin_scheduled_task.plugin_updates]
}

# Jellyfin's Update Plugins task upgrades every plugin at startup and daily.
# While it has triggers, version is a minimum, so Bookshelf may run a newer
# release. With no triggers, as here, no plugin updates automatically and
# Bookshelf stays at exactly 13.0.0.0.
resource "jellyfin_scheduled_task" "plugin_updates" {
  key      = "PluginUpdates"
  triggers = []
}
