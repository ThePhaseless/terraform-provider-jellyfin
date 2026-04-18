data "jellyfin_users" "all" {}

output "user_names" {
  value = [for u in data.jellyfin_users.all.users : u.name]
}
