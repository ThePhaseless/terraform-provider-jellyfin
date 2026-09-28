// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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

// A first apply plans the settings an entry leaves unset as unknown; the
// write keeps those of the served entry with the same id.
func TestUnitLiveTVTunerHostKeepsTheServedEntrysSettings(t *testing.T) {
	ctx := context.Background()
	b, err := livetvWire()
	if err != nil {
		t.Fatal(err)
	}
	const served = `{"TunerHosts": [{"Id": "abc", "Url": "http://old", "Type": "hdhomerun", "FriendlyName": "Living room", "TunerCount": 2}]}`
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(served), &doc); err != nil {
		t.Fatal(err)
	}
	read, d := b.Flatten(ctx, doc, types.ObjectNull(b.AttrTypes))
	if d.HasError() {
		t.Fatal(d)
	}
	hosts, ok := read.Attributes()["tuner_hosts"].(types.List)
	if !ok {
		t.Fatal("tuner_hosts is not a list")
	}
	elemType, ok := hosts.ElementType(ctx).(types.ObjectType)
	if !ok {
		t.Fatal("tuner_hosts holds no objects")
	}
	planned := map[string]attr.Value{}
	for name, typ := range elemType.AttrTypes {
		v, err := typ.ValueFromTerraform(ctx, tftypes.NewValue(typ.TerraformType(ctx), tftypes.UnknownValue))
		if err != nil {
			t.Fatal(err)
		}
		planned[name] = v
	}
	planned["id"] = types.StringValue("abc")
	planned["url"] = types.StringValue("http://new")
	attrs := read.Attributes()
	attrs["tuner_hosts"] = types.ListValueMust(elemType, []attr.Value{types.ObjectValueMust(elemType.AttrTypes, planned)})

	if d := b.Overlay(ctx, doc, types.ObjectValueMust(read.AttributeTypes(ctx), attrs)); d.HasError() {
		t.Fatal(d)
	}
	checkSameJSON(t, doc["TunerHosts"], `[{"Id": "abc", "Url": "http://new", "Type": "hdhomerun", "FriendlyName": "Living room", "TunerCount": 2}]`)
}
