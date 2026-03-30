resource "jellyfin_api_key" "example" {
  app_name = "my-application"
}

output "api_key" {
  value     = jellyfin_api_key.example.access_token
  sensitive = true
}
