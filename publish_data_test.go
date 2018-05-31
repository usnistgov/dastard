package dastard

import (
	"testing"
)

func TestPublishData(t *testing.T) {
	dp := DataPublisher{}
	d := []RawType{10, 10, 10, 10, 15, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10}
	rec := &DataRecord{data: d, presamples: 4}
	records := []*DataRecord{rec}
	err := dp.PublishData(records)
	if err != nil {
		t.Fail()
	}
	dp.SetLJH22(1, 4, len(d), 1, 1, 8, 1, "TestPublishData.ljh")
	err = dp.PublishData(records)
	if err != nil {
		t.Fail()
	}
	if dp.LJH22.RecordsWritten != 1 {
		t.Fail()
	}
	if !dp.HasLJH22() {
		t.Error("HasLJH22 want true, have", dp.HasLJH22())
	}
	dp.RemoveLJH22()
	if dp.HasLJH22() {
		t.Error("HasLJH22 want false, have", dp.HasLJH22())
	}
	if dp.HasPubFeederChan() {
		t.Error("HasPubFeederChan want false, have", dp.HasPubFeederChan())
	}
	dp.SetPubFeederChan()
	if !dp.HasPubFeederChan() {
		t.Error("HasPubFeederChan want true, have", dp.HasPubFeederChan())
	}
	dp.PublishData(records)
	dp.RemovePubFeederChan()
	if dp.HasPubFeederChan() {
		t.Error("HasPubFeederChan want false, have", dp.HasPubFeederChan())
	}

}
