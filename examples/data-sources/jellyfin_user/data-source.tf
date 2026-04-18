data "jellyfin_user" "admin" {
  name = "admin"
}

output "admin_id" {
  value = data.jellyfin_user.admin.id
}
