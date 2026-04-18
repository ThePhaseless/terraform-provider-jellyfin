data "jellyfin_system_configuration" "current" {}

output "server_name" {
  value = data.jellyfin_system_configuration.current.server_name
}
