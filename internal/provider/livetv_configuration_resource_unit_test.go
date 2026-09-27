// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"
)

func TestUnitLiveTVConfigurationRoundTrip(t *testing.T) {
	ctx := context.Background()
	b, err := livetvWire()
	if err != nil {
		t.Fatal(err)
	}
	fixture := `{"GuideDays":14,"RecordingPath":"/recordings","MovieRecordingPath":"/recordings/movies","SeriesRecordingPath":"/recordings/series","EnableRecordingSubfolders":true,"EnableOriginalAudioWithEncodedRecordings":false,"TunerHosts":[{"Id":"host1","Url":"http://tv/","Type":"m3u","DeviceId":"device1","FriendlyName":"Tuner","ImportFavoritesOnly":false,"AllowHWTranscoding":true,"AllowFmp4TranscodingContainer":false,"AllowStreamSharing":true,"FallbackMaxStreamingBitrate":30000000,"EnableStreamLooping":false,"Source":"source","TunerCount":2,"UserAgent":"agent","IgnoreDts":true,"ReadAtNativeFramerate":false}],"ListingProviders":[{"Id":"prov1","Type":"SchedulesDirect","Username":"u","Password":"p","ListingsId":"listings1","ZipCode":"12345","Country":"US","Path":"/guide.xml","EnabledTuners":["host1"],"EnableAllTuners":false,"NewsCategories":["News"],"SportsCategories":["Sports"],"KidsCategories":["Kids"],"MovieCategories":["Movie"],"ChannelMappings":[{"Name":"c1","Value":"d1"}],"MoviePrefix":"M: ","PreferredLanguage":"en","UserAgent":"agent"}],"PrePaddingSeconds":30,"PostPaddingSeconds":30,"MediaLocationsCreated":["/recordings"],"RecordingPostProcessor":"/bin/true","RecordingPostProcessorArguments":"\"{path}\"","SaveRecordingNFO":true,"SaveRecordingImages":true}`

	data := readWire[LiveTVConfigurationResourceModel](t, b, fixture)
	base := map[string]json.RawMessage{}
	if d := b.OverlayModel(ctx, base, &data); d.HasError() {
		t.Fatalf("write: %v", d)
	}
	checkSameJSON(t, base, fixture)
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
