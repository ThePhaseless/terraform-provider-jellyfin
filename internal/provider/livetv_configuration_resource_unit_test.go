// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnitLiveTVConfigurationRoundTrip(t *testing.T) {
	fixture := `{"GuideDays":14,"RecordingPath":"/recordings","MovieRecordingPath":"/recordings/movies","SeriesRecordingPath":"/recordings/series","EnableRecordingSubfolders":true,"EnableOriginalAudioWithEncodedRecordings":false,"TunerHosts":[{"Id":"host1","Url":"http://tv/","Type":"m3u","DeviceId":"device1","FriendlyName":"Tuner","ImportFavoritesOnly":false,"AllowHWTranscoding":true,"AllowFmp4TranscodingContainer":false,"AllowStreamSharing":true,"FallbackMaxStreamingBitrate":30000000,"EnableStreamLooping":false,"Source":"source","TunerCount":2,"UserAgent":"agent","IgnoreDts":true,"ReadAtNativeFramerate":false}],"ListingProviders":[{"Id":"prov1","Type":"SchedulesDirect","Username":"u","Password":"p","ListingsId":"listings1","ZipCode":"12345","Country":"US","Path":"/guide.xml","EnabledTuners":["host1"],"EnableAllTuners":false,"NewsCategories":["News"],"SportsCategories":["Sports"],"KidsCategories":["Kids"],"MovieCategories":["Movie"],"ChannelMappings":[{"Name":"c1","Value":"d1"}],"MoviePrefix":"M: ","PreferredLanguage":"en","UserAgent":"agent"}],"PrePaddingSeconds":30,"PostPaddingSeconds":30,"MediaLocationsCreated":["/recordings"],"RecordingPostProcessor":"/bin/true","RecordingPostProcessorArguments":"\"{path}\"","SaveRecordingNFO":true,"SaveRecordingImages":true}`
	checkRoundTrip[LiveTVConfigurationResourceModel](t, mustWire(t, livetvWire), fixture)
}

// A first apply plans the settings an entry leaves unset as unknown; the
// write keeps those of the served entry with the same id.
func TestUnitLiveTVTunerHostKeepsTheServedEntrysSettings(t *testing.T) {
	ctx := t.Context()
	b := mustWire(t, livetvWire)
	const served = `{"TunerHosts": [{"Id": "abc", "Url": "http://old", "Type": "hdhomerun", "FriendlyName": "Living room", "TunerCount": 2}]}`
	doc, err := parseJSONObject(served)
	if err != nil {
		t.Fatal(err)
	}
	read := flattenFromNull(ctx, t, b, served)
	hosts, ok := read.Attributes()["tuner_hosts"].(types.List)
	if !ok {
		t.Fatal("tuner_hosts is not a list")
	}
	elemType, ok := hosts.ElementType(ctx).(types.ObjectType)
	if !ok {
		t.Fatal("tuner_hosts holds no objects")
	}
	planned := testUnitUnknownAttributes(t, elemType.AttrTypes)
	planned["id"] = types.StringValue("abc")
	planned["url"] = types.StringValue("http://new")
	attrs := read.Attributes()
	attrs["tuner_hosts"] = types.ListValueMust(elemType, []attr.Value{types.ObjectValueMust(elemType.AttrTypes, planned)})

	if d := b.Overlay(ctx, doc, types.ObjectValueMust(read.AttributeTypes(ctx), attrs)); d.HasError() {
		t.Fatal(d)
	}
	checkSameJSON(t, doc["TunerHosts"], `[{"Id": "abc", "Url": "http://new", "Type": "hdhomerun", "FriendlyName": "Living room", "TunerCount": 2}]`)
}
