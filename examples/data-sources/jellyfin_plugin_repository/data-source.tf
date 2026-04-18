data "jellyfin_plugin_repository" "stable" {
  name = "Jellyfin Stable"
}

output "stable_repo_url" {
  value = data.jellyfin_plugin_repository.stable.url
}
