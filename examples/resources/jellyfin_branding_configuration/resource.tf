resource "jellyfin_branding_configuration" "example" {
  configuration_json = jsonencode({
    SplashscreenEnabled = false
  })
}
