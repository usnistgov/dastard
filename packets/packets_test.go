package packets

import (
	"bytes"
	"encoding/binary"
	"reflect"
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
	if _, err2 := Header(bytes.NewReader([]byte{0, 16, 0, 0, 0x81, 0xb, 0, 0xff})); err2 == nil {
		t.Errorf("Header should fail if source ID cannot be read")
	}
	if _, err2 := Header(bytes.NewReader([]byte{0, 16, 0, 0, 0x81, 0xb, 0, 0xff, 0, 0, 0, 0})); err2 == nil {
		t.Errorf("Header should fail if sequence number cannot be read")
	}
	if _, err2 := Header(bytes.NewReader([]byte{0, 16, 0, 9, 0x81, 0xb, 0, 0xff, 0, 0, 0, 0, 0, 0, 0, 0})); err2 == nil {
		t.Errorf("Header should fail if payload length is not a multiple of 8")
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

func timestampToPacket(t headTimestamp) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, byte(0x11))
	binary.Write(buf, binary.BigEndian, byte(1))
	binary.Write(buf, binary.BigEndian, uint16(t>>32))
	binary.Write(buf, binary.BigEndian, uint32(t))
	return buf.Bytes()
}

func payloadshapeToPacket(ps *headPayloadShape) []byte {
	buf := new(bytes.Buffer)
	n8 := 1 + len(ps.Sizes)/4
	binary.Write(buf, binary.BigEndian, byte(0x22))
	binary.Write(buf, binary.BigEndian, byte(n8))
	for _, s := range ps.Sizes {
		binary.Write(buf, binary.BigEndian, s)
	}
	for i := len(ps.Sizes) % 4; i < 3; i++ {
		binary.Write(buf, binary.BigEndian, int16(0))
	}
	return buf.Bytes()
}

func chanOffsetToPacket(off headChannelOffset) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, byte(0x23))
	binary.Write(buf, binary.BigEndian, byte(1))
	binary.Write(buf, binary.BigEndian, uint16(0))
	binary.Write(buf, binary.BigEndian, uint32(off))
	return buf.Bytes()
}

func TestTLVs(t *testing.T) {
	// Try a counter
	c := HeadCounter{1234, 987654321}
	cp := counterToPacket(&c)
	tlvs, err := readTLV(bytes.NewReader(cp), 8)
	if err != nil {
		t.Errorf("readTLV() for HeadCounter returns %v", err)
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

	// Try a timestamp
	ts := headTimestamp(1234567890123)
	tp := timestampToPacket(ts)
	tlvs, err = readTLV(bytes.NewReader(tp), 8)
	if err != nil {
		t.Errorf("readTLV() for headTimestamp returns %v", err)
	}
	switch hts := tlvs[0].(type) {
	case headTimestamp:
		if hts != ts {
			t.Errorf("headTimestamp = %d, want %d", hts, ts)
		}
	default:
		t.Errorf("expected type headTimestamp, got %v", hts)
	}

	// Try a channel offset
	offset := headChannelOffset(13579)
	op := chanOffsetToPacket(offset)
	tlvs, err = readTLV(bytes.NewReader(op), 8)
	if err != nil {
		t.Errorf("readTLV() for headChannelOffset returns %v", err)
	}
	switch hoff := tlvs[0].(type) {
	case headChannelOffset:
		if hoff != offset {
			t.Errorf("headChannelOffset = %d, want %d", hoff, offset)
		}
	default:
		t.Errorf("expected type headChannelOffset, got %v", hoff)
	}

	// Try a payload format TLV
	x := []byte{0x21, 1, '>', 'i', 'i', 0, 0, 0}
	tlvs, err = readTLV(bytes.NewReader(x), 8)
	if err != nil {
		t.Errorf("readTLV() for headPayloadFormat returns %v", err)
	}
	if len(tlvs) != 1 {
		t.Errorf("readTLV() for headPayloadFormat returns array length %d, want 1", len(tlvs))
	}
	switch hpf := tlvs[0].(type) {
	case *headPayloadFormat:
		if !hpf.bigendian {
			t.Errorf("headPayloadFormat.bigendian is false, want true")
		}

	default:
		t.Errorf("expected type headPayloadFormat, got %v", hpf)
	}
	x[4] = 'Q'
	if _, err = readTLV(bytes.NewReader(x), 8); err == nil {
		t.Errorf("Expect error on readTLV with conflicting format characters")
	}
	x[3] = 'a'
	if _, err = readTLV(bytes.NewReader(x), 8); err == nil {
		t.Errorf("Expect error on readTLV with unknown format character")
	}

	// Check all types
	x[2] = '<'
	x[3] = 'a'
	x[4] = 0
	var tests = []struct {
		tag   byte
		dtype reflect.Kind
	}{
		{'h', reflect.Int16},
		{'H', reflect.Uint16},
		{'i', reflect.Int32},
		{'l', reflect.Int32},
		{'I', reflect.Uint32},
		{'L', reflect.Uint32},
		{'q', reflect.Int64},
		{'Q', reflect.Uint64},
	}
	for _, test := range tests {
		x[3] = test.tag
		tlvs, err = readTLV(bytes.NewReader(x), 8)
		if err != nil {
			t.Errorf("readTLV failed on format string '%s'", string(x[2:]))
		}
		if len(tlvs) != 1 {
			t.Errorf("readTLV() for headPayloadFormat returns array length %d, want 1", len(tlvs))
		}
		h, ok := tlvs[0].(*headPayloadFormat)
		if !ok {
			t.Errorf("readTLV()[0] is %v, fails type assertion to &headPayloadFormat", tlvs[0])
		}
		if h.dtype != test.dtype {
			t.Errorf("readTLV for headPayloadFormat is dtype %v for tag '%c', want %v", h.dtype, test.tag, test.dtype)
		}
		if h.bigendian {
			t.Errorf("readTLV for headPayloadFormat is big endian, want little endian")
		}
		if h.nvals != 1 {
			t.Errorf("readTLV for headPayloadFormat has nvals %d, want 1", h.nvals)
		}
	}

	// Try payload shape
	ps := new(headPayloadShape)
	ps.Sizes = append(ps.Sizes, int16(5))
	ps.Sizes = append(ps.Sizes, int16(6))
	pp := payloadshapeToPacket(ps)
	tlvs, err = readTLV(bytes.NewReader(pp), 8)
	if err != nil {
		t.Errorf("readTLV() for headPayloadShape returns %v", err)
	}
	if len(tlvs) != 1 {
		t.Errorf("readTLV() for headPayloadShape returns array length %d, want 1", len(tlvs))
	}
	switch hps := tlvs[0].(type) {
	case *headPayloadShape:
		if len(hps.Sizes) != len(ps.Sizes) {
			t.Errorf("headPayloadShape # dimensions = %d, want %d", len(hps.Sizes), len(ps.Sizes))
		}
	default:
		t.Errorf("expected type headPayloadShape, got %v", hps)
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
	if _, err := readTLV(bytes.NewReader([]byte{0x11, 0x1}), 8); err == nil {
		t.Errorf("readTLV without timestamp upper bytes should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{0x11, 0x1, 0, 0}), 8); err == nil {
		t.Errorf("readTLV without timestamp lower bytes should error")
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
	if _, err := readTLV(bytes.NewReader([]byte{0x21, 1}), 8); err == nil {
		t.Errorf("readTLV on payload format with short string should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{0x22, 1, 0}), 8); err == nil {
		t.Errorf("readTLV on channel offset without any dimensions should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{0x23, 1, 0}), 8); err == nil {
		t.Errorf("readTLV on channel offset without padding should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{0x23, 1, 0, 9}), 8); err == nil {
		t.Errorf("readTLV on channel offset with nonzero padding should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{0x23, 1, 0, 0, 0}), 8); err == nil {
		t.Errorf("readTLV on channel offset without offset should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{0xff, 1}), 8); err == nil {
		t.Errorf("readTLV on unknown type should error")
	}
}
