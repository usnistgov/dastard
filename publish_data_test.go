package dastard

import (
	"testing"
)

func TestPublishData(t *testing.T) {

	dp := DataPublisher{}
	d := []RawType{10, 10, 10, 10, 15, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10}
	rec := &DataRecord{data: d, presamples: 4}
	records := []*DataRecord{rec, rec, rec}

	if err := dp.PublishData(records); err != nil {
		t.Fail()
	}
	dp.SetLJH22(1, 4, len(d), 1, 1, 8, 1, "TestPublishData.ljh")
	if err := dp.PublishData(records); err != nil {
		t.Fail()
	}
	if dp.LJH22.RecordsWritten != 3 {
		t.Fail()
	}
	if !dp.HasLJH22() {
		t.Error("HasLJH22 want true, have", dp.HasLJH22())
	}
	dp.RemoveLJH22()
	if dp.HasLJH22() {
		t.Error("HasLJH22 want false, have", dp.HasLJH22())
	}

	if dp.HasPubRecords() {
		t.Error("HasPubRecords want false, have", dp.HasPubRecords())
	}
	dp.SetPubRecords()

	if !dp.HasPubRecords() {
		t.Error("HasPubRecords want true, have", dp.HasPubRecords())
	}

	dp.PublishData(records)

	dp.RemovePubRecords()
	if dp.HasPubRecords() {
		t.Error("HasPubRecords want false, have", dp.HasPubRecords())
	}

	if dp.HasPubSummaries() {
		t.Error("HasPubSummaries want false, have", dp.HasPubSummaries())
	}
	dp.SetPubSummaries()

	if !dp.HasPubSummaries() {
		t.Error("HasPubSummaries want true, have", dp.HasPubSummaries())
	}

	dp.PublishData(records)

	dp.RemovePubSummaries()
	if dp.HasPubSummaries() {
		t.Error("HasPubSummaries want false, have", dp.HasPubSummaries())
	}

	dp.SetLJH3(0, 0, 0, 0, "TestPublishData.ljh3")
	if err := dp.PublishData(records); err != nil {
		t.Error("failed to publish record")
	}
	if dp.LJH3.RecordsWritten != 3 {
		t.Error("wrong number of RecordsWritten, want 1, have", dp.LJH3.RecordsWritten)
	}
	if !dp.HasLJH3() {
		t.Error("HasLJH3 want true, have", dp.HasLJH3())
	}
	dp.RemoveLJH3()
	if dp.HasLJH3() {
		t.Error("HasLJH3 want false, have", dp.HasLJH3())
	}
}