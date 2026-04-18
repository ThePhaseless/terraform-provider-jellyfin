data "jellyfin_api_keys" "all" {}

output "api_key_app_names" {
  value = [for k in data.jellyfin_api_keys.all.keys : k.app_name]
}
