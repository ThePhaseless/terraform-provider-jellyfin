resource "jellyfin_initial_setup" "bootstrap" {
  username                    = "admin"
  password                    = "changeMe!"
  ui_culture                  = "en-US"
  metadata_country_code       = "US"
  preferred_metadata_language = "en"
}
