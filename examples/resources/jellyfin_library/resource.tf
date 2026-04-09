resource "jellyfin_library" "movies" {
  name            = "Movies"
  collection_type = "movies"
  paths           = ["/media/movies"]
}
