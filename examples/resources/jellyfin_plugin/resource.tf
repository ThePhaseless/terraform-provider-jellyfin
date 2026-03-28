resource "jellyfin_plugin" "example" {
  name           = "Bookshelf"
  version        = "14.0.0.0"
  repository_url = "https://repo.jellyfin.org/files/plugin/manifest.json"
}
