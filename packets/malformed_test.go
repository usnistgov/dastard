package packets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// buildPacket assembles a wire-format packet from the given TLV blocks and payload.
func buildPacket(tlv []byte, payload []byte) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint8(0x11))          // version
	binary.Write(buf, binary.BigEndian, uint8(16+len(tlv)))   // headerLength
	binary.Write(buf, binary.BigEndian, uint16(len(payload))) // payloadLength
	binary.Write(buf, binary.BigEndian, packetMAGIC)
	binary.Write(buf, binary.BigEndian, uint32(0x20)) // sourceID
	binary.Write(buf, binary.BigEndian, uint32(0x30)) // sequenceNumber
	buf.Write(tlv)
	buf.Write(payload)
	return buf.Bytes()
}

var tlvFormatInt16 = []byte{tlvFORMAT, 1, '<', 'h', 0, 0, 0, 0}
var tlvFormatEndianOnly = []byte{tlvFORMAT, 1, '<', 0, 0, 0, 0, 0}
var tlvShape2 = []byte{tlvSHAPE, 1, 0, 2, 0, 0, 0, 0}

// A header that carries a format TLV but no shape TLV must not make ChannelInfo fail.
func TestChannelInfoWithoutShape(t *testing.T) {
	raw := buildPacket(tlvFormatInt16, []byte{1, 0, 2, 0})
	p, err := ReadPacket(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadPacket() returns error %v, want nil", err)
	}
	if p.shape != nil {
		t.Fatalf("test packet carries a shape TLV, want none")
	}
	nchan, offset := p.ChannelInfo()
	if nchan != 1 {
		t.Errorf("ChannelInfo() nchan=%d, want 1", nchan)
	}
	if offset != 0 {
		t.Errorf("ChannelInfo() offset=%d, want 0", offset)
	}
}

// A header that carries a shape TLV but no format TLV must not make Frames fail.
func TestFramesWithoutFormat(t *testing.T) {
	raw := buildPacket(tlvShape2, []byte{1, 0, 2, 0, 3, 0, 4, 0})
	p, err := ReadPacket(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadPacket() returns error %v, want nil", err)
	}
	if p.format != nil {
		t.Fatalf("test packet carries a format TLV, want none")
	}
	if frames := p.Frames(); frames != 0 {
		t.Errorf("Frames()=%d, want 0", frames)
	}
}

// A format TLV that names no data type leaves wordlen at 0, which must not make Frames fail.
func TestFramesWithZeroWordLength(t *testing.T) {
	tlv := append(append([]byte{}, tlvFormatEndianOnly...), tlvShape2...)
	raw := buildPacket(tlv, []byte{1, 0, 2, 0, 3, 0, 4, 0})
	p, err := ReadPacket(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadPacket() returns error %v, want nil", err)
	}
	if p.format == nil || p.format.wordlen != 0 {
		t.Fatalf("test packet format is %v, want a format of word length 0", p.format)
	}
	if frames := p.Frames(); frames != 0 {
		t.Errorf("Frames()=%d, want 0", frames)
	}
}
