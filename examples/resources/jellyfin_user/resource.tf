resource "jellyfin_user" "example" {
  name             = "johndoe"
  password         = "secret123"
  is_administrator = false
}
