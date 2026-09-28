# Every attribute is optional: set only those to manage. These values are
# Jellyfin's defaults for a server reached on its LAN and remotely.
resource "jellyfin_networking_configuration" "example" {
  base_url             = ""
  enable_https         = false
  require_https        = false
  internal_http_port   = 8096
  internal_https_port  = 8920
  public_http_port     = 8096
  public_https_port    = 8920
  auto_discovery       = true
  enable_ipv4          = true
  enable_ipv6          = false
  enable_remote_access = true
  known_proxies        = []
}
