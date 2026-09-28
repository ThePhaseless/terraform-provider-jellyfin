resource "jellyfin_library" "movies" {
  name            = "Movies"
  collection_type = "movies"
  # Paths are looked up by the Jellyfin server. When it runs in a container,
  # use the path where the media is mounted inside the container.
  paths = ["/media/movies"]

  library_options = {
    enable_photos                        = true
    enable_realtime_monitor              = true
    enable_chapter_image_extraction      = false
    extract_chapters_during_library_scan = false
    preferred_metadata_language          = "en"
    metadata_country_code                = "US"
    save_local_metadata                  = true
    season_zero_display_name             = "Specials"
    disabled                             = false
    # Subtitle fetchers come from plugins, such as Open Subtitles. List the
    # ones to enable, in priority order; the others are disabled.
    subtitle_fetchers = []

    type_options = [
      {
        type = "Movie"
        # The enabled fetchers, in priority order.
        metadata_fetchers = ["TheMovieDb", "The Open Movie Database"]
        image_fetchers    = ["TheMovieDb"]
        image_options = [
          {
            type      = "Backdrop"
            limit     = 3
            min_width = 1280
          }
        ]
      }
    ]
  }
}
