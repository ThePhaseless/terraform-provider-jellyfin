// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"strings"
	"testing"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
)

func TestLivetvAttributes(t *testing.T) {
	raw := `{
  "GuideDays": 14,
  "RecordingPath": null,
  "EnableRecordingSubfolders": true,
  "TunerHosts": [{"Id": "t1", "Url": "http://tuner/", "Type": "hdhomerun", "TunerCount": 2}],
  "ListingProviders": [{"Id": "p1", "Type": "SchedulesDirect", "Password": "secret", "EnabledTuners": ["t1"], "ChannelMappings": [{"Name": "1", "Value": "one"}]}],
  "MediaLocationsCreated": ["/recordings"],
  "RecordingPostProcessorArguments": "\"{path}\""
}`

	attrs, err := livetvAttributes(raw)
	if err != nil {
		t.Fatalf("livetvAttributes() error: %v", err)
	}

	want := map[string]string{
		"guide_days":                         "14",
		"enable_recording_subfolders":        "true",
		"recording_post_processor_arguments": `"\"{path}\""`,
		"tuner_hosts": `[
    {
      id = "t1"
      tuner_count = 2
      type = "hdhomerun"
      url = "http://tuner/"
    },
  ]`,
		"listing_providers": `[
    {
      channel_mappings = [
        {
          name = "1"
          value = "one"
        },
      ]
      enabled_tuners = ["t1"]
      id = "p1"
      type = "SchedulesDirect"
    },
  ]`,
	}
	if len(attrs) != len(want) {
		t.Errorf("livetvAttributes() keys = %v, want %d keys", sortedKeys(attrs), len(want))
	}
	for k, v := range want {
		if attrs[k] != v {
			t.Errorf("livetvAttributes()[%q] =\n%s\nwant\n%s", k, attrs[k], v)
		}
	}
}

func TestHCLStringEscapesTemplatesAndControlCharacters(t *testing.T) {
	got := hclString("${a} %{b} $c \"q\" \\ line\nnext\x01")
	want := `"$${a} %%{b} $c \"q\" \\ line\nnext\u0001"`
	if got != want {
		t.Errorf("hclString() = %s, want %s", got, want)
	}
}

func TestGenerateSingletonConfigsRendersTypedLivetvAttributes(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	g := &generator{
		client:    client.NewClient(server.URL, "test-key"),
		outputDir: t.TempDir(),
		usedNames: make(map[string]int),
	}

	_, resources, err := g.generateSingletonConfigs()
	if err != nil {
		t.Fatalf("generateSingletonConfigs() error: %v", err)
	}

	var livetv string
	for _, r := range resources {
		if strings.HasPrefix(r, `resource "jellyfin_livetv_configuration" "this"`) {
			livetv = r
		}
	}
	want := `resource "jellyfin_livetv_configuration" "this" {
  enable_recording_subfolders = false
}
`
	if livetv != want {
		t.Errorf("livetv resource block =\n%s\nwant\n%s", livetv, want)
	}
}
