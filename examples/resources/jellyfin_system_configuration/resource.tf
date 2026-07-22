resource "jellyfin_system_configuration" "example" {
  server_name = "My Jellyfin Server"
  cache_path  = "/cache"

  metadata_options = [
    {
      item_type = "Movie"
    }
  ]

  trickplay_options = {
    interval         = 10000
    width_resolutions = [320, 480, 640]
    tile_width       = 10
    tile_height      = 10
  }
}
