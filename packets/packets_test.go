package packets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func headerToPacket(h *PacketHeader) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, h.version)
	binary.Write(buf, binary.BigEndian, h.headerLength)
	binary.Write(buf, binary.BigEndian, h.payloadLength)
	binary.Write(buf, binary.BigEndian, PACKETMAGIC)
	binary.Write(buf, binary.BigEndian, h.sourceID)
	binary.Write(buf, binary.BigEndian, h.sequenceNumber)
	return buf.Bytes()
}

func TestHeader(t *testing.T) {
	hdr1 := PacketHeader{0x11, 16, 0, 0x44, 0x55, nil}
	data := headerToPacket(&hdr1)

	hdr2, err := Header(bytes.NewReader(data))
	if err != nil {
		t.Errorf("Header() returns error %v", err)
	}
	if hdr2.version != hdr1.version {
		t.Errorf("Header version is 0x%x, want 0x%x", hdr2.version, hdr1.version)
	}
	if hdr2.headerLength != hdr1.headerLength {
		t.Errorf("Header length is 0x%x, want 0x%x", hdr2.headerLength, hdr1.headerLength)
	}
	if hdr2.payloadLength != hdr1.payloadLength {
		t.Errorf("Header payload length is 0x%x, want 0x%x", hdr2.payloadLength, hdr1.payloadLength)
	}
	if hdr2.sourceID != hdr1.sourceID {
		t.Errorf("Header source ID is 0x%x, want 0x%x", hdr2.sourceID, hdr1.sourceID)
	}
	if hdr2.sequenceNumber != hdr1.sequenceNumber {
		t.Errorf("Header sequence number is 0x%x, want 0x%x", hdr2.sequenceNumber, hdr1.sequenceNumber)
	}

	// Make sure incomplete headers fail
	if _, err2 := Header(bytes.NewReader([]byte{})); err2 == nil {
		t.Errorf("Header should fail if version cannot be read")
	}
	if _, err2 := Header(bytes.NewReader([]byte{0})); err2 == nil {
		t.Errorf("Header should fail if header length cannot be read")
	}
	if _, err2 := Header(bytes.NewReader([]byte{0, 15})); err2 == nil {
		t.Errorf("Header should fail if header length < 16")
	}
	if _, err2 := Header(bytes.NewReader([]byte{0, 16})); err2 == nil {
		t.Errorf("Header should fail if payload length cannot be read")
	}
	if _, err2 := Header(bytes.NewReader([]byte{0, 16, 0, 0})); err2 == nil {
		t.Errorf("Header should fail if magic cannot be read")
	}
	if _, err2 := Header(bytes.NewReader([]byte{0, 16, 0, 0, 1, 2, 3, 4})); err2 == nil {
		t.Errorf("Header should fail if magic is wrong")
	}
	if _, err2 := Header(bytes.NewReader([]byte{0, 16, 0, 0, 8, 0xff, 0, 0xee})); err2 == nil {
		t.Errorf("Header should fail if source ID cannot be read")
	}
	if _, err2 := Header(bytes.NewReader([]byte{0, 16, 0, 0, 8, 0xff, 0, 0xee, 0, 0, 0, 0})); err2 == nil {
		t.Errorf("Header should fail if sequence number cannot be read")
	}
}

func counterToPacket(c *HeadCounter) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, byte(0x12))
	binary.Write(buf, binary.BigEndian, byte(1))
	binary.Write(buf, binary.BigEndian, c.ID)
	binary.Write(buf, binary.BigEndian, c.Count)
	return buf.Bytes()
}

func TestTLVs(t *testing.T) {
	c := HeadCounter{1234, 987654321}
	cp := counterToPacket(&c)
	tlvs, err := readTLV(bytes.NewReader(cp), 8)
	if err != nil {
		t.Errorf("readTLV() returns %v", err)
	}
	switch hc := tlvs[0].(type) {
	case *HeadCounter:
		if hc.ID != c.ID {
			t.Errorf("HeadCounter.ID = %d, want %d", hc.ID, c.ID)
		}
		if hc.Count != c.Count {
			t.Errorf("HeadCounter.Count = %d, want %d", hc.Count, c.Count)
		}

	default:
		t.Errorf("expected type HeadCounter, got %v", hc)
	}

	// Make sure errors happen when packet is incomplete
	if _, err := readTLV(bytes.NewReader([]byte{}), 7); err == nil {
		t.Errorf("readTLV on fewer than 8 bytes should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{}), 8); err == nil {
		t.Errorf("readTLV on empty string should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{0x21}), 8); err == nil {
		t.Errorf("readTLV on short string should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{0x21, 0x2}), 8); err == nil {
		t.Errorf("readTLV with L=2 but size-8 string should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{0x12, 0x2}), 16); err == nil {
		t.Errorf("readTLV with L>1 counters should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{0x12, 0x1, 0}), 8); err == nil {
		t.Errorf("readTLV without counter ID should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{0x12, 0x1, 0, 0}), 8); err == nil {
		t.Errorf("readTLV without counter value should error")
	}
}
