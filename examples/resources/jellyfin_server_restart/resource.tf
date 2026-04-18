# Restart the server whenever the system configuration changes.
resource "jellyfin_system_configuration" "main" {
  configuration_json = jsonencode({
    ServerName = "media-server"
  })
}

resource "jellyfin_server_restart" "after_config_change" {
  triggers = {
    config = jellyfin_system_configuration.main.configuration_json
  }
}
