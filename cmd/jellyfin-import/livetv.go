// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

// media_locations_created is left out because Jellyfin maintains it itself.
var livetvFields = []hclField{
	{json: "GuideDays", attr: "guide_days"},
	{json: "RecordingPath", attr: "recording_path"},
	{json: "MovieRecordingPath", attr: "movie_recording_path"},
	{json: "SeriesRecordingPath", attr: "series_recording_path"},
	{json: "EnableRecordingSubfolders", attr: "enable_recording_subfolders"},
	{json: "EnableOriginalAudioWithEncodedRecordings", attr: "enable_original_audio_with_encoded_recordings"},
	{json: "TunerHosts", attr: "tuner_hosts", nested: []hclField{
		{json: "Id", attr: "id"},
		{json: "Url", attr: "url"},
		{json: "Type", attr: "type"},
		{json: "DeviceId", attr: "device_id"},
		{json: "FriendlyName", attr: "friendly_name"},
		{json: "ImportFavoritesOnly", attr: "import_favorites_only"},
		{json: "AllowHWTranscoding", attr: "allow_hw_transcoding"},
		{json: "AllowFmp4TranscodingContainer", attr: "allow_fmp4_transcoding_container"},
		{json: "AllowStreamSharing", attr: "allow_stream_sharing"},
		{json: "FallbackMaxStreamingBitrate", attr: "fallback_max_streaming_bitrate"},
		{json: "EnableStreamLooping", attr: "enable_stream_looping"},
		{json: "Source", attr: "source"},
		{json: "TunerCount", attr: "tuner_count"},
		{json: "UserAgent", attr: "user_agent"},
		{json: "IgnoreDts", attr: "ignore_dts"},
		{json: "ReadAtNativeFramerate", attr: "read_at_native_framerate"},
	}},
	// password is left out to keep it out of resources.tf; the imported
	// state keeps it because the attribute is optional and computed.
	{json: "ListingProviders", attr: "listing_providers", nested: []hclField{
		{json: "Id", attr: "id"},
		{json: "Type", attr: "type"},
		{json: "Username", attr: "username"},
		{json: "ListingsId", attr: "listings_id"},
		{json: "ZipCode", attr: "zip_code"},
		{json: "Country", attr: "country"},
		{json: "Path", attr: "path"},
		{json: "EnabledTuners", attr: "enabled_tuners"},
		{json: "EnableAllTuners", attr: "enable_all_tuners"},
		{json: "NewsCategories", attr: "news_categories"},
		{json: "SportsCategories", attr: "sports_categories"},
		{json: "KidsCategories", attr: "kids_categories"},
		{json: "MovieCategories", attr: "movie_categories"},
		{json: "ChannelMappings", attr: "channel_mappings", nested: []hclField{
			{json: "Name", attr: "name"},
			{json: "Value", attr: "value"},
		}},
		{json: "MoviePrefix", attr: "movie_prefix"},
		{json: "PreferredLanguage", attr: "preferred_language"},
		{json: "UserAgent", attr: "user_agent"},
	}},
	{json: "PrePaddingSeconds", attr: "pre_padding_seconds"},
	{json: "PostPaddingSeconds", attr: "post_padding_seconds"},
	{json: "RecordingPostProcessor", attr: "recording_post_processor"},
	{json: "RecordingPostProcessorArguments", attr: "recording_post_processor_arguments"},
	{json: "SaveRecordingNFO", attr: "save_recording_nfo"},
	{json: "SaveRecordingImages", attr: "save_recording_images"},
}
