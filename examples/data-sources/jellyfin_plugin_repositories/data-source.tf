data "jellyfin_plugin_repositories" "all" {}

output "repository_names" {
  value = [for r in data.jellyfin_plugin_repositories.all.repositories : r.name]
}
