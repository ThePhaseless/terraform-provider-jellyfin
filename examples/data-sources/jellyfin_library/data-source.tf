data "jellyfin_library" "movies" {
  name = "Movies"
}

output "movie_paths" {
  value = data.jellyfin_library.movies.paths
}
