resource "jellyfin_networking_configuration" "example" {
  configuration_json = jsonencode({
    BaseUrl                   = ""
    EnableHttps               = false
    RequireHttps              = false
    InternalHttpPort          = 8096
    InternalHttpsPort         = 8920
    PublicHttpPort            = 8096
    PublicHttpsPort           = 8920
    AutoDiscovery             = true
    EnableIPv4                = true
    EnableIPv6                = false
    EnableRemoteAccess        = true
    KnownProxies              = []
    RemoteIPFilter            = []
    IsRemoteIPFilterBlacklist = false
  })
}
