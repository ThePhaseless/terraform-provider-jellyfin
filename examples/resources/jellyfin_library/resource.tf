resource "jellyfin_library" "movies" {
  name            = "Movies"
  collection_type = "movies"
  paths           = ["/media/movies"]

  library_options = {
    enable_photos                                 = true
    enable_realtime_monitor                       = true
    extract_chapters_during_library_scan          = false
    enable_chapter_image_extraction               = false
    extract_media_information_during_library_scan = true
    download_images_in_advance                    = false
    cache_images_in_library                       = true
    preferred_metadata_language                   = "en"
    metadata_country_code                         = "US"
    save_local_metadata                           = true
    save_local_thumbnail_sets                     = true
    import_missing_episodes                       = true
    season_zero_display_name                      = "Specials"
    metadata_refresh_mode                         = "Default"
    disabled                                      = false

    type_options = [
      {
        type                = "Movie"
        metadata_fetchers   = ["TheMovieDb"]
        image_fetchers      = ["TheMovieDb"]
        image_fetcher_order = ["TheMovieDb"]
        image_options = [
          {
            type  = "Backdrop"
            limit = 3
          }
        ]
      }
    ]
  }
}
