data "jellyfin_libraries" "all" {}

output "library_names" {
  value = [for l in data.jellyfin_libraries.all.libraries : l.name]
}
