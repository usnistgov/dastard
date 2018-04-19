package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
	"time"
)

// TestPublishRecord checks packet(DataRecord) makes a reasonable header and message.
func TestPublishRecord(t *testing.T) {
	data := []RawType{1, 2, 3, 4, 5, 4, 3, 2, 1}
	rec := &DataRecord{data: data, trigTime: time.Now()}

	header, message := packet(rec)

	buf := bytes.NewReader(header)
	if buf.Len() != len(header) {
		t.Errorf("bytes.Reader has length %d, want %d", buf.Len(), len(header))
	}
	var b uint8
	for i := 0; i < len(header); i++ {
		if err := binary.Read(buf, binary.LittleEndian, &b); err != nil {
			t.Errorf("binary.Read failed: %v", err)
		}
	}
	if err := binary.Read(buf, binary.LittleEndian, &b); err != io.EOF {
		t.Errorf("binary.Read should have failed, but did not")
	}

	if len(message)/2 != len(data) {
		t.Errorf("packet generated message of %d samples, want %d", len(message)/2, len(data))
	}

}
