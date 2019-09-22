package dastard

import (
	"encoding/binary"
	"fmt"
	"io"
)

// PacketTLV represents a Type-Length-Value item from the packet header.
// type PacketTLV struct {
// 	TLVType byte // code for this TLV's type
// 	Length  int  // length in bytes
// 	Value   interface{}
// }

// ByteToPacketTLV converts a []byte slice to a new *PacketTLV.
// func ByteToPacketTLV(data []byte) *PacketTLV {
// 	p := new(PacketTLV)
// 	p.TLVType = data[0]
// 	p.Length = 8 * int(data[1])
// 	p.Value = data[2:p.Length]
// 	return p
// }

// PacketHeader represents the header of an Abaco data packet
type PacketHeader struct {
	version        uint8
	headerLength   uint8
	payloadLength  uint16
	sourceID       uint32
	sequenceNumber uint32
	otherTLV       interface{}
}

// PACKETMAGIC is the packet header's magic number.
const PACKETMAGIC uint32 = 0x08ff00ee

// Header returns a PacketHeader read from an io.reader
func Header(data io.Reader) (h *PacketHeader, err error) {
	h = new(PacketHeader)
	if err = binary.Read(data, binary.BigEndian, &h.version); err != nil {
		return nil, err
	}
	if err = binary.Read(data, binary.BigEndian, &h.headerLength); err != nil {
		return nil, err
	}
	const MINLENGTH uint8 = 16
	if h.headerLength < MINLENGTH {
		return nil, fmt.Errorf("Header length is %d, expect at least %d", h.headerLength, MINLENGTH)
	}
	if err = binary.Read(data, binary.BigEndian, &h.payloadLength); err != nil {
		return nil, err
	}
	var magic uint32
	if err = binary.Read(data, binary.BigEndian, &magic); err != nil {
		return nil, err
	}
	if magic != PACKETMAGIC {
		return nil, fmt.Errorf("Magic was 0x%x, want 0x%x", magic, PACKETMAGIC)
	}
	if err = binary.Read(data, binary.BigEndian, &h.sourceID); err != nil {
		return nil, err
	}
	if err = binary.Read(data, binary.BigEndian, &h.sequenceNumber); err != nil {
		return nil, err
	}
	return h, nil
}

// type HeadNull struct{}
//
// type HeadTimestamp struct{}

// HeadCounter represents a counter found in a packet header
type HeadCounter struct {
	ID    int16
	Count int64
}
