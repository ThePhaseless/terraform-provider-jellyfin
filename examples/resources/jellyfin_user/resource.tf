resource "jellyfin_user" "example" {
  name             = "johndoe"
  password         = "secret123"
  is_administrator = false
}

# User with full typed policy control. Top-level IsAdministrator, IsDisabled,
# and EnableAllFolders are managed outside the `policy` block; InvalidLoginAttemptCount
# is server-managed and excluded.
resource "jellyfin_user" "restricted" {
  name               = "restricted_user"
  password           = "secret456"
  is_administrator   = false
  enable_all_folders = false

  policy = {
    enable_media_playback             = true
    enable_audio_playback_transcoding = true
    enable_video_playback_transcoding = false
    enable_content_deletion           = false
    enable_remote_access              = true
    enable_live_tv_access             = false
    login_attempts_before_lockout     = 3
    max_active_sessions               = 2
    sync_play_access                  = "JoinGroups"
    enabled_folders                   = []
  }
}
