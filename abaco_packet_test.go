package dastard

import (
	"bytes"
	"encoding/binary"
	"fmt"
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
	switch x := tlvs[0].(type) {
	case HeadCounter:

	default:
		fmt.Printf("c:  %v\n", c)
		fmt.Printf("cp: %v\n", cp)
		fmt.Printf("tlv0: %v\n", tlvs[0])
		t.Errorf("expected type HeadCounter, got %v", x)
	}
}
