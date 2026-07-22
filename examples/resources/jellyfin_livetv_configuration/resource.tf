resource "jellyfin_livetv_configuration" "example" {
  guide_days           = 14
  recording_path       = "/recordings"
  pre_padding_seconds  = 30
  post_padding_seconds = 30
  save_recording_nfo   = true
}
