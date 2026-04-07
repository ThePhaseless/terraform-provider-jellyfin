resource "jellyfin_user" "example" {
  name             = "johndoe"
  password         = "secret123"
  is_administrator = false
}

# User with full policy control via policy_json
resource "jellyfin_user" "restricted" {
  name               = "restricted_user"
  password           = "secret456"
  is_administrator   = false
  enable_all_folders = false

  policy_json = jsonencode({
    EnableMediaPlayback            = true
    EnableAudioPlaybackTranscoding = true
    EnableVideoPlaybackTranscoding = false
    EnableContentDeletion          = false
    EnableRemoteAccess             = true
    EnableLiveTvAccess             = false
    LoginAttemptsBeforeLockout     = 3
    MaxActiveSessions              = 2
    SyncPlayAccess                 = "JoinGroups"
    EnabledFolders                 = []
  })
}
