data "jellyfin_plugins" "all" {}

output "plugin_names" {
  value = [for p in data.jellyfin_plugins.all.plugins : p.name]
}
