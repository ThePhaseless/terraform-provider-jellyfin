data "jellyfin_api_key" "ci" {
  app_name = "ci-pipeline"
}

output "ci_token" {
  value     = data.jellyfin_api_key.ci.access_token
  sensitive = true
}
