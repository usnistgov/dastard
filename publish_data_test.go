package dastard

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gonum.org/v1/gonum/mat"
)

func TestPublishData(t *testing.T) {
	ljh2Testfile := filepath.Join("testData", "TestPublishData.ljh")
	ljh3Testfile := filepath.Join("testData", "TestPublishData.ljh3")
	offTestfile := filepath.Join("testData", "TestPublishData.off")

	dp := DataPublisher{}
	dp.StartWriteDataLoop()
	d := []RawType{10, 10, 10, 10, 15, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10}
	rec := &DataRecord{data: d, presamples: 4, modelCoefs: make([]float64, 3)}
	records := []*DataRecord{rec, rec, rec, nil}
	// Any errors in the triggering calcultion produce nil values for a record; remove them
	records = slices.DeleteFunc(records, func(r *DataRecord) bool { return r == nil })

	if err := dp.PublishData(records); err != nil {
		t.Errorf("could not publish %d data records", len(records))
	}
	startTime := time.Now()
	dp.SetLJH22(1, 4, len(d), 1, 1, startTime, 8, 1, 16, 8, 3, 0, 3,
		ljh2Testfile, "testSource", "chanX", 1, Pixel{})
	if err := dp.PublishData(records); err != nil {
		t.Errorf("could not publish %d data records", len(records))
	}
	if dp.numberWritten != 3 {
		t.Errorf("expected PublishData numberWritten with LJH22 enabled, (want %d, found %d)", 3, dp.numberWritten)
	}
	if !dp.HasLJH22() {
		t.Error("HasLJH22() false, want true")
	}
	dp.RemoveLJH22()
	if dp.HasLJH22() {
		t.Error("HasLJH22() true, want false")
	}
	if dp.numberWritten != 0 {
		t.Errorf("expected RemoveLJH22 to set numberWritten to 0")
	}

	if dp.HasPubRecords() {
		t.Error("HasPubRecords() true, want false")
	}
	dp.SetPubRecords()

	if !dp.HasPubRecords() {
		t.Error("HasPubRecords() false, want true")
	}

	dp.PublishData(records)
	if dp.numberWritten != 0 {
		t.Errorf("expected PublishData to not increment numberWritten with only PubRecords enabled")
	}
	dp.RemovePubRecords()
	if dp.HasPubRecords() {
		t.Error("HasPubRecords() true, want false")
	}

	if dp.HasPubSummaries() {
		t.Error("HasPubSummaries() true, want false")
	}
	dp.SetPubSummaries()

	if !dp.HasPubSummaries() {
		t.Error("HasPubSummaries() false, want true")
	}

	dp.PublishData(records)

	dp.RemovePubSummaries()
	if dp.HasPubSummaries() {
		t.Error("HasPubSummaries() true, want false")
	}

	dp.SetLJH3(0, 0, 0, 0, 0, 0, ljh3Testfile)
	if err := dp.PublishData(records); err != nil {
		t.Error("failed to publish record")
	}
	if dp.numberWritten != 3 {
		t.Errorf("expected PublishedData to increment numberWritten, (want %d, found %d)", 3, dp.numberWritten)
	}
	if !dp.HasLJH3() {
		t.Error("HasLJH3() false, want true")
	}
	dp.RemoveLJH3()
	if dp.numberWritten != 0 {
		t.Errorf("expected RemoveLJH3 to set numberWritten to 0")
	}
	if dp.HasLJH3() {
		t.Error("HasLJH3() true, want false")
	}

	nbases := 3
	nsamples := 4
	projectors := mat.NewDense(nbases, nsamples, make([]float64, nbases*nsamples))
	basis := mat.NewDense(nsamples, nbases, make([]float64, nbases*nsamples))
	dp.SetOFF(0, 0, 0, 1, 1, time.Now(), 1, 1, 1, 1, 1, 1, 1, offTestfile, "sourceName",
		"chanName", 1, projectors, basis, "ModelDescription", Pixel{})
	if err := dp.PublishData(records); err != nil {
		t.Error(err)
	}
	if dp.numberWritten != 3 {
		t.Errorf("expected PublishedData to increment numberWritten (want %d, found %d)", 3, dp.numberWritten)
	}
	if !dp.HasOFF() {
		t.Error("HasOFF() false, want true")
	}
	dp.RemoveOFF()
	if dp.numberWritten != 0 {
		t.Errorf("expected RemoveOFF to set numberWritten to 0")
	}
	if dp.HasOFF() {
		t.Error("HasOFF() true, want false")
	}

	if err := configurePubRecordsSocket(); err == nil {
		t.Error("it should be an error to configurePubRecordsSocket twice")
	}
	if err := configurePubSummariesSocket(); err == nil {
		t.Error("it should be an error to configurePubSummariesSocket twice")
	}

	rec = &DataRecord{data: d, presamples: 4}
	for i, signed := range []bool{false, true} {
		(*rec).signed = signed
		msg := messageRecords(rec)
		header := msg[0]
		dtype := header[3]
		expect := []uint8{3, 2}
		if dtype != expect[i] {
			t.Errorf("messageRecords with signed=%t gives dtype=%d, want %d",
				signed, dtype, expect[i])
		}

	}
}

func TestRawTypeToX(t *testing.T) {
	d := []RawType{0xFFFF, 0x0101, 0xABCD, 0xEF01, 0x2345, 0x6789}
	b := rawTypeToBytes(d)
	encodedStr := hex.EncodeToString(b)
	expectStr := "ffff0101cdab01ef45238967"
	if encodedStr != expectStr {
		t.Errorf("hex.EncodeToString(rawTypeToBytes(d)) have %v, want %v", encodedStr, expectStr)
	}
	if len(b) != 2*len(d) {
		t.Errorf("rawTypeToBytes giveswrong length, have %v, want %v", len(b), len(d))
	}
	c := rawTypeToUint16(d)
	expect := []uint16{0xFFFF, 0x0101, 0xABCD, 0xEF01, 0x2345, 0x6789}
	if len(c) != len(expect) {
		t.Errorf("rawTypeToUint16 length %d, want %d", len(c), len(expect))
	}
	for i, v := range expect {
		if c[i] != v {
			t.Errorf("rawTypeToUint16(b)[%d] = %v, want %v", i, c[i], v)
		}
	}

	d2 := bytesToRawType(b)
	if len(d) != len(d2) {
		t.Errorf("bytesToRawType length %d, want %d", len(d2), len(d))
	}
	for i, val := range d {
		if d2[i] != val {
			t.Errorf("bytesToRawType(b)[%d] = 0x%x, want 0x%x", i, d2[i], val)
		}
	}
}

func BenchmarkPublish(b *testing.B) {
	ljh2Testfile := filepath.Join("testData", "TestPublishData.ljh")
	ljh3Testfile := filepath.Join("testData", "TestPublishData.ljh3")

	d := make([]RawType, 1000)
	rec := &DataRecord{data: d, presamples: 4}
	records := make([]*DataRecord, 1)
	for i := range records {
		records[i] = rec
	}
	slowPart := func(b *testing.B, dp *DataPublisher, records []*DataRecord) {
		for b.Loop() {
			dp.PublishData(records)
			b.SetBytes(int64(len(d) * 2 * len(records)))
		}
	}
	startTime := time.Now()

	b.Run("PubRecords", func(b *testing.B) {
		dp := DataPublisher{}
		dp.SetPubRecords()
		defer dp.RemovePubRecords()
		slowPart(b, &dp, records)
	})
	b.Run("PubSummaries", func(b *testing.B) {
		dp := DataPublisher{}
		dp.SetPubSummaries()
		defer dp.RemovePubSummaries()
		slowPart(b, &dp, records)
	})
	b.Run("PubLJH22", func(b *testing.B) {
		dp := DataPublisher{}
		go dp.WriteDataLoop()
		dp.SetLJH22(0, 0, len(d), 1, 0, startTime, 0, 0, 0, 0, 0, 0, 0,
			"TestPublishData.ljh", "testSource", "chanX", 1, Pixel{})
		defer dp.RemoveLJH22()
		slowPart(b, &dp, records)
	})
	b.Run("PubLJH3", func(b *testing.B) {
		dp := DataPublisher{}
		go dp.WriteDataLoop()
		dp.SetLJH3(0, 0, 0, 0, 0, 0, ljh3Testfile)
		defer dp.RemoveLJH3()
		slowPart(b, &dp, records)
	})
	b.Run("PubAll", func(b *testing.B) {
		dp := DataPublisher{}
		go dp.WriteDataLoop()
		dp.SetPubRecords()
		defer dp.RemovePubRecords()
		dp.SetPubSummaries()
		defer dp.RemovePubSummaries()
		dp.SetLJH22(0, 0, len(d), 1, 0, startTime, 0, 0, 0, 0, 0, 0, 0,
			ljh2Testfile, "testSource", "chanX", 1, Pixel{})
		defer dp.RemoveLJH22()
		dp.SetLJH3(0, 0, 0, 0, 0, 0, ljh3Testfile)
		defer dp.RemoveLJH3()
		slowPart(b, &dp, records)
	})
	b.Run("PubNone", func(b *testing.B) {
		dp := DataPublisher{}
		slowPart(b, &dp, records)
	})
	b.Run("RawTypeToUint16", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			data := make([]uint16, len(rec.data))
			for i, v := range rec.data {
				data[i] = uint16(v)
			}
			b.SetBytes(int64(2 * len(rec.data)))
		}
	})
	b.Run("binary.Write", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var buf bytes.Buffer
			binary.Write(&buf, binary.LittleEndian, rec.data)
			b.SetBytes(int64(2 * len(rec.data)))
		}
	})
	b.Run("rawTypeToBytes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			data := rawTypeToBytes(rec.data)
			b.SetBytes(int64(2 * len(data)))
		}
	})
}
