data "jellyfin_plugin" "bookshelf" {
  name = "Bookshelf"
}

output "bookshelf_version" {
  value = data.jellyfin_plugin.bookshelf.version
}
