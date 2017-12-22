package dastard

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// TestPublishRecord checks packet(DataRecord) makes a reasonable header and message.
func TestPublishRecord(t *testing.T) {
	data := []RawType{1, 2, 3, 4, 5, 4, 3, 2, 1}
	rec := &DataRecord{data: data, trigTime: time.Now()}

	header, message := packet(rec)

	buf := bytes.NewReader(header)
	var channum int16
	err := binary.Read(buf, binary.LittleEndian, &channum)
	if err != nil {
		t.Errorf("binary.Read failed: %v", err)
	}
	var hver, dtype uint8
	err = binary.Read(buf, binary.LittleEndian, &hver)
	if err != nil {
		t.Errorf("binary.Read failed: %v", err)
	}
	err = binary.Read(buf, binary.LittleEndian, &dtype)
	if err != nil {
		t.Errorf("binary.Read failed: %v", err)
	}
	var npre int32
	err = binary.Read(buf, binary.LittleEndian, &npre)
	if err != nil {
		t.Errorf("binary.Read failed: %v", err)
	}
	var period, vperarb float32
	err = binary.Read(buf, binary.LittleEndian, &period)
	if err != nil {
		t.Errorf("binary.Read failed: %v", err)
	}
	err = binary.Read(buf, binary.LittleEndian, &vperarb)
	if err != nil {
		t.Errorf("binary.Read failed: %v", err)
	}

	if hver != 0 {
		t.Errorf("packet generated with version number %d, want %d", hver, 0)
	}

	if len(message)/2 != len(data) {
		t.Errorf("packet generated message of %d samples, want %d", len(message)/2, len(data))
	}
}
