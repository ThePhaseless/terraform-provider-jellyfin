data "jellyfin_available_packages" "all" {}

output "package_names" {
  value = [for p in data.jellyfin_available_packages.all.packages : p.name]
}
