// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// hclField maps a Jellyfin JSON property to a Terraform attribute. nested
// describes the attributes of each element when the property is a list of
// objects.
type hclField struct {
	json   string
	attr   string
	nested []hclField
}

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
	// password is left out so the generated file holds no secret; the
	// imported state keeps it because the attribute is optional and computed.
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

// livetvAttributes renders the jellyfin_livetv_configuration attributes for a
// Live TV configuration as returned by the server.
func livetvAttributes(raw string) (map[string]string, error) {
	return hclAttributes(raw, livetvFields, 1)
}

// hclAttributes renders the fields present in a JSON object as HCL attribute
// values, skipping nulls. depth is the block nesting level the attributes are
// written at, used to indent multi-line values.
func hclAttributes(raw string, fields []hclField, depth int) (map[string]string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, fmt.Errorf("parsing JSON object: %w", err)
	}

	attrs := make(map[string]string, len(fields))
	for _, f := range fields {
		v, ok := obj[f.json]
		if !ok || string(v) == "null" {
			continue
		}
		rendered, err := hclValue(v, f.nested, depth)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.json, err)
		}
		attrs[f.attr] = rendered
	}
	return attrs, nil
}

func hclValue(raw json.RawMessage, nested []hclField, depth int) (string, error) {
	switch {
	case nested != nil:
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return "", err
		}
		if len(elements) == 0 {
			return "[]", nil
		}
		indent := strings.Repeat("  ", depth)
		var b strings.Builder
		b.WriteString("[\n")
		for _, e := range elements {
			attrs, err := hclAttributes(string(e), nested, depth+2)
			if err != nil {
				return "", err
			}
			b.WriteString(indent + "  {\n")
			keys := make([]string, 0, len(attrs))
			for k := range attrs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "%s    %s = %s\n", indent, k, attrs[k])
			}
			b.WriteString(indent + "  },\n")
		}
		b.WriteString(indent + "]")
		return b.String(), nil
	case bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")):
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return "", err
		}
		rendered := make([]string, len(elements))
		for i, e := range elements {
			s, err := hclValue(e, nil, depth)
			if err != nil {
				return "", err
			}
			rendered[i] = s
		}
		return "[" + strings.Join(rendered, ", ") + "]", nil
	case bytes.HasPrefix(bytes.TrimSpace(raw), []byte(`"`)):
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return hclString(s), nil
	default:
		// Numbers and booleans read the same in HCL as in JSON.
		return string(bytes.TrimSpace(raw)), nil
	}
}

// hclString quotes s as an HCL string literal. Unlike quote, it also escapes
// control characters and the ${ and %{ template introducers, which HCL would
// otherwise interpolate.
func hclString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		case (r == '$' || r == '%') && strings.HasPrefix(s[i+1:], "{"):
			b.WriteRune(r)
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
