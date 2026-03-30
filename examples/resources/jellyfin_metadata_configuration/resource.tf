resource "jellyfin_metadata_configuration" "example" {
  configuration_json = jsonencode({
    UseFileCreationTimeForDateAdded = true
  })
}
