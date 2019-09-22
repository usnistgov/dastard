package dastard

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
}
