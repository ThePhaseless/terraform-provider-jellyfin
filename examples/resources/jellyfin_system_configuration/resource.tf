resource "jellyfin_system_configuration" "example" {
  server_name = "My Jellyfin Server"
  cache_path  = "/cache"

  metadata_options = [
    {
      item_type = "Movie"
      # The enabled fetchers, in priority order; the others are disabled.
      metadata_fetchers = ["TheMovieDb", "The Open Movie Database"]
      image_fetchers    = ["TheMovieDb", "Embedded Image Extractor", "Screen Grabber"]
    }
  ]

  trickplay_options = {
    interval          = 10000
    width_resolutions = [320, 480, 640]
    tile_width        = 10
    tile_height       = 10
  }
}
