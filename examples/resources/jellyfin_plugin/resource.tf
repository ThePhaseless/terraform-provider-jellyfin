resource "jellyfin_plugin" "example" {
  name           = "Bookshelf"
  version        = "13.0.0.0"
  repository_url = "https://repo.jellyfin.org/files/plugin/manifest.json"
}

# Jellyfin's Update Plugins task upgrades every plugin at startup and daily, so
# Bookshelf would leave the pinned version once a newer one is released.
# Without triggers the task never runs, which stops automatic updates for every
# plugin.
resource "jellyfin_scheduled_task" "plugin_updates" {
  task_id  = "f9b057c054e9e6daee4a88ffd146a403"
  triggers = []
}
