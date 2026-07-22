resource "jellyfin_networking_configuration" "example" {
  base_url                               = ""
  enable_https                           = false
  require_https                          = false
  certificate_path                       = ""
  certificate_password                   = ""
  internal_http_port                     = 0
  internal_https_port                    = 0
  public_http_port                       = 0
  public_https_port                      = 0
  auto_discovery                         = false
  enable_upnp                            = false
  enable_ipv4                            = false
  enable_ipv6                            = false
  enable_remote_access                   = false
  local_network_subnets                  = []
  local_network_addresses                = []
  known_proxies                          = []
  ignore_virtual_interfaces              = false
  virtual_interface_names                = []
  enable_published_server_uri_by_request = false
  published_server_uri_by_subnet         = []
  remote_ip_filter                       = []
  is_remote_ip_filter_blacklist          = false
}
