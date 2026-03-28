data "jellyfin_system_info" "example" {}

output "server_version" {
  value = data.jellyfin_system_info.example.version
}
