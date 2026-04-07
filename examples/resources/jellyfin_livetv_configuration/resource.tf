resource "jellyfin_livetv_configuration" "example" {
  configuration_json = jsonencode({
    EnableRecordingSubfolders                = false
    EnableOriginalAudioWithEncodedRecordings = false
    PrePaddingSeconds                        = 120
    PostPaddingSeconds                       = 120
    SaveRecordingNFO                         = true
    SaveRecordingImages                      = true
    RecordingPostProcessorArguments          = "\"{path}\""
    TunerHosts                               = []
    ListingProviders                         = []
  })
}
