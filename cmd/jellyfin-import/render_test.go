// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"maps"
	"slices"
	"testing"
)

func TestImportedAttributesForLiveTV(t *testing.T) {
	raw := `{
  "GuideDays": 14,
  "RecordingPath": null,
  "EnableRecordingSubfolders": true,
  "TunerHosts": [{"Id": "t1", "Url": "http://tuner/", "Type": "hdhomerun", "TunerCount": 2}],
  "ListingProviders": [{"Id": "p1", "Type": "SchedulesDirect", "Password": "secret", "EnabledTuners": ["t1"], "ChannelMappings": [{"Name": "1", "Value": "one"}]}],
  "MediaLocationsCreated": ["/recordings"],
  "RecordingPostProcessorArguments": "\"{path}\""
}`

	attrs, err := importedAttributes(t.Context(), nil, "jellyfin_livetv_configuration", "livetv", raw)
	if err != nil {
		t.Fatalf("importedAttributes() error: %v", err)
	}

	want := map[string]string{
		"guide_days":                         "14",
		"enable_recording_subfolders":        "true",
		"recording_post_processor_arguments": `"\"{path}\""`,
		"tuner_hosts": `[
    {
      id          = "t1"
      tuner_count = 2
      type        = "hdhomerun"
      url         = "http://tuner/"
    },
  ]`,
		"listing_providers": `[
    {
      channel_mappings = [
        {
          name  = "1"
          value = "one"
        },
      ]
      enabled_tuners = ["t1"]
      id             = "p1"
      type           = "SchedulesDirect"
    },
  ]`,
	}
	if len(attrs) != len(want) {
		t.Errorf("importedAttributes() keys = %v, want %d keys", slices.Sorted(maps.Keys(attrs)), len(want))
	}
	for k, v := range want {
		if attrs[k] != v {
			t.Errorf("importedAttributes()[%q] =\n%s\nwant\n%s", k, attrs[k], v)
		}
	}
}
