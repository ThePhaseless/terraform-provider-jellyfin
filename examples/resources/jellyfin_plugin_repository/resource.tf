resource "jellyfin_plugin_repository" "stable" {
  name    = "Jellyfin Stable"
  url     = "https://repo.jellyfin.org/files/plugin/manifest.json"
  enabled = true
}
