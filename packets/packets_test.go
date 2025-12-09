package packets

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestByteSwap(t *testing.T) {
	v1a := []int16{0x0102, 0x0a0b, 0x004f}
	v1b := []int16{0x0201, 0x0b0a, 0x4f00}
	v2a := []int32{0x01020304, 0x0a0b0c0d, 0x0000ff1f}
	v2b := []int32{0x04030201, 0x0d0c0b0a, 0x1fff0000}
	v3a := []int64{0x0102030405060708, 0x08090a0b0c0d0e0f, 0x0123456789abcd0f}
	v3b := []int64{0x0807060504030201, 0x0f0e0d0c0b0a0908, 0x0fcdab8967452301}
	if err := ByteSwap(v1a); err != nil {
		t.Errorf("byteSwap(%T) error: %v", v1a, err)
	}
	if err := ByteSwap(v2a); err != nil {
		t.Errorf("byteSwap(%T) error: %v", v2a, err)
	}
	if err := ByteSwap(v3a); err != nil {
		t.Errorf("byteSwap(%T) error: %v", v3a, err)
	}
	for i, v := range v1a {
		if v != v1b[i] {
			t.Errorf("byteSwap(%T) v[%d]=0x%x, want 0x%x", v1b, i, v, v1b[i])
		}
	}
	for i, v := range v2a {
		if v != v2b[i] {
			t.Errorf("byteSwap(%T) v[%d]=0x%x, want 0x%x", v2b, i, v, v2b[i])
		}
	}
	for i, v := range v3a {
		if v != v3b[i] {
			t.Errorf("byteSwap(%T) v[%d]=0x%x, want 0x%x", v3b, i, v, v3b[i])
		}
	}
	if err := ByteSwap([]float64{2.5, 3.5}); err == nil {
		t.Errorf("byteSwap([]float64) should error, did not.")
	}
}

func TestHeader(t *testing.T) {
	hdr1 := NewPacket(0x11, 0x44, 0x55, 0)
	data := hdr1.Bytes()

	hdr2, err := ReadPacket(bytes.NewReader(data))
	if err != nil {
		t.Errorf("ReadPacket() returns error %v", err)
	}
	if hdr2.version != hdr1.version {
		t.Errorf("ReadPacket version is 0x%x, want 0x%x", hdr2.version, hdr1.version)
	}
	if hdr2.headerLength != hdr1.headerLength {
		t.Errorf("ReadPacket length is 0x%x, want 0x%x", hdr2.headerLength, hdr1.headerLength)
	}
	if hdr2.payloadLength != hdr1.payloadLength {
		t.Errorf("ReadPacket payload length is 0x%x, want 0x%x", hdr2.payloadLength, hdr1.payloadLength)
	}
	if hdr2.sourceID != hdr1.sourceID {
		t.Errorf("ReadPacket source ID is 0x%x, want 0x%x", hdr2.sourceID, hdr1.sourceID)
	}
	if hdr2.sequenceNumber != hdr1.sequenceNumber {
		t.Errorf("ReadPacket sequence number is 0x%x, want 0x%x", hdr2.sequenceNumber, hdr1.sequenceNumber)
	}

	// Check ReadPacketPlusPad
	dataPadded := append(data, make([]byte, 10000)...)
	// np := len(dataPadded)
	const magicbyte = byte(0xfe)
	dataPadded[5000] = magicbyte
	rdr3 := bytes.NewReader(dataPadded)
	_, err = ReadPacketPlusPad(rdr3, 5000)
	if err != nil {
		t.Errorf("ReadPacketPlusPad() returns error %v", err)
	} else {
		b, err := rdr3.ReadByte()
		if err != nil {
			t.Errorf("ReadPacketPlusPad() did not leave bytes remaining: %s", err)
		} else if b != magicbyte {
			t.Errorf("ReadPacketPlusPad() first remaining byte is 0x%x, want 0x%x", b, magicbyte)
		}
	}
	if _, err = ReadPacketPlusPad(rdr3, 5000); err == nil {
		t.Errorf("ReadPacketPlusPad twice succeeds, should fail")
	}
	rdr3 = bytes.NewReader(dataPadded)
	if _, err = ReadPacketPlusPad(rdr3, 50000); err == nil {
		t.Errorf("ReadPacketPlusPad with large stride succeeds, should fail")
	}

	// Make sure incomplete headers fail
	hdr := make([]byte, 16)
	if _, err2 := ReadPacket(bytes.NewReader(hdr[:5])); err2 == nil {
		t.Errorf("ReadPacket should fail if header length < 16")
	}
	hdr[1] = 9
	if _, err2 := ReadPacket(bytes.NewReader(hdr)); err2 == nil {
		t.Errorf("ReadPacket should fail if header stated length < 16")
	}
	hdr[1] = 16
	hdr[3] = 7
	if _, err2 := ReadPacket(bytes.NewReader(hdr)); err2 == nil {
		t.Errorf("ReadPacket should fail if payload length is not a multiple of 8")
	}
	hdr[3] = 16
	if _, err2 := ReadPacket(bytes.NewReader(hdr)); err2 == nil {
		t.Errorf("ReadPacket should fail if magic is wrong")
	}
	hdr[4] = 0x81
	hdr[5] = 0x0b
	hdr[7] = 0xff
	if _, err2 := ReadPacket(bytes.NewReader(hdr)); err2 != nil {
		t.Errorf("ReadPacket should failed on valid header")
	}

	hdr[1] = 24
	if _, err2 := ReadPacket(bytes.NewReader(hdr)); err2 == nil {
		t.Errorf("ReadPacket should fail if header stated length is longer than actual")
	}

	// Now add a failing TLV
	hdr = append(hdr, make([]byte, 8)...)
	if _, err2 := ReadPacket(bytes.NewReader(hdr)); err2 == nil {
		t.Errorf("ReadPacket should fail if header stated length is longer than actual")
	}
}

func counterToPacket(c *HeadCounter) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, byte(tlvCOUNTER))
	binary.Write(buf, binary.BigEndian, byte(1))
	binary.Write(buf, binary.BigEndian, c.ID)
	binary.Write(buf, binary.BigEndian, c.Count)
	return buf.Bytes()
}

func tagToPacket(tag *PacketTag) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, byte(tlvTAG))
	binary.Write(buf, binary.BigEndian, byte(1))
	binary.Write(buf, binary.BigEndian, uint16(0))
	binary.Write(buf, binary.BigEndian, tag)
	return buf.Bytes()
}

func timestampToPacket(ts *PacketTimestamp) []byte {
	buf := new(bytes.Buffer)
	if ts.Rate == 0.0 {
		binary.Write(buf, binary.BigEndian, byte(tlvTIMESTAMP))
		binary.Write(buf, binary.BigEndian, byte(1))
		binary.Write(buf, binary.BigEndian, uint16(ts.T>>32))
		binary.Write(buf, binary.BigEndian, uint32(ts.T))
	} else {
		binary.Write(buf, binary.BigEndian, byte(tlvTIMESTAMPUNIT))
		binary.Write(buf, binary.BigEndian, byte(2))
		binary.Write(buf, binary.BigEndian, byte(64))
		e := int8(math.Log10(ts.Rate)) + 1
		const num = uint16(10000)
		r := ts.Rate * float64(num) / math.Pow10(int(e))
		denom := uint16(math.Round(r))
		binary.Write(buf, binary.BigEndian, -e)
		binary.Write(buf, binary.BigEndian, num)
		binary.Write(buf, binary.BigEndian, denom)
		binary.Write(buf, binary.BigEndian, ts.T)
	}

	return buf.Bytes()
}

func payloadshapeToPacket(ps *headPayloadShape) []byte {
	buf := new(bytes.Buffer)
	n8 := 1 + len(ps.Sizes)/4
	binary.Write(buf, binary.BigEndian, byte(tlvSHAPE))
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
	binary.Write(buf, binary.BigEndian, byte(tlvCHANOFFSET))
	binary.Write(buf, binary.BigEndian, byte(1))
	binary.Write(buf, binary.BigEndian, uint16(0))
	binary.Write(buf, binary.BigEndian, uint32(off))
	return buf.Bytes()
}

func nonsensePacket() []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, byte(tlvINVALID))
	binary.Write(buf, binary.BigEndian, byte(1))
	binary.Write(buf, binary.BigEndian, uint16(0))
	binary.Write(buf, binary.BigEndian, uint32(0))
	return buf.Bytes()
}

func TestTLVs(t *testing.T) {
	// Try a counter
	c := HeadCounter{1234, 987654321}
	cp := counterToPacket(&c)
	tlvs, err := parseTLV(cp)
	if err != nil {
		t.Errorf("parseTLV() for HeadCounter returns: %v", err)
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

	// Try a timestamp, with and without units
	ts := PacketTimestamp{217304205466533888, 1e+09}
	tp := timestampToPacket(&ts)
	tlvs, err = parseTLV(tp)
	if err != nil {
		t.Errorf("parseTLV() for PacketTimestamp returns %v", err)
	}
	switch hts := tlvs[0].(type) {
	case *PacketTimestamp:
		if hts.T != ts.T {
			t.Errorf("PacketTimestamp = %v, want %v", *hts, ts)
		}
	default:
		t.Errorf("expected type PacketTimestamp, got %v", hts)
	}
	// Now add a rate
	ts = PacketTimestamp{0x0102030405060708, 0}
	ts.Rate = 256e6
	tp = timestampToPacket(&ts)
	tlvs, err = parseTLV(tp)
	if err != nil {
		t.Errorf("parseTLV() for PacketTimestamp returns %v", err)
	}
	switch hts := tlvs[0].(type) {
	case *PacketTimestamp:
		if hts.T != ts.T || hts.Rate != ts.Rate {
			t.Errorf("PacketTimestamp = %v, want %v", *hts, ts)
		}
	default:
		t.Errorf("expected type PacketTimestamp, got %v", hts)
	}
	tryTSNbits := []uint8{64, 48, 32, 16, 8}
	for _, nbits := range tryTSNbits {
		tp[2] = nbits
		tlvs, err = parseTLV(tp)
		if err != nil {
			t.Errorf("parseTLV() for PacketTimestamp returns %v", err)
		}
		switch hts := tlvs[0].(type) {
		case *PacketTimestamp:
			mask := ^(uint64(math.MaxUint64) << nbits)
			if hts.T != (ts.T & mask) {
				t.Errorf("nbits %d: PacketTimestamp = 0x%x, want 0x%x", nbits, hts.T, ts.T&mask)
			}
		default:
			t.Errorf("expected type PacketTimestamp, got %v", hts)
		}
	}
	tp[2] = 66
	if _, err = parseTLV(tp); err == nil {
		t.Errorf("parseTLV() for PacketTimestamp succeeds with %d bits (>64), should fail", tp[2])
	}

	// Try a nonsensical TLV type. Should not be an error
	nonsense := nonsensePacket()
	_, err = parseTLV(nonsense)
	if err != nil {
		t.Errorf("parseTLV() for invalid TLV should be ignored, but returns %v", err)
	}

	// Check packet tags.
	tagval := PacketTag(0xda37a9d)
	tagpacket := tagToPacket(&tagval)
	tlvs, err = parseTLV(tagpacket)
	if err != nil {
		t.Errorf("parseTLV() for TAG TLV returns %v", err)
	}
	switch pt := tlvs[0].(type) {
	case PacketTag:
		if pt != tagval {
			t.Errorf("PacketTag = 0x%x, want 0x%x", pt, tagval)
		}
	default:
		t.Errorf("expected type PacketTag, got %v", pt)
	}

	// Try a channel offset
	offset := headChannelOffset(13579)
	op := chanOffsetToPacket(offset)
	tlvs, err = parseTLV(op)
	if err != nil {
		t.Errorf("parseTLV() for headChannelOffset returns %v", err)
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
	x := []byte{tlvFORMAT, 1, '>', 'i', 'i', 0, 0, 0}
	tlvs, err = parseTLV(x)
	if err != nil {
		t.Errorf("parseTLV() for headPayloadFormat returns %v", err)
	}
	if len(tlvs) != 1 {
		t.Errorf("parseTLV() for headPayloadFormat returns array length %d, want 1", len(tlvs))
	}
	switch hpf := tlvs[0].(type) {
	case *headPayloadFormat:
		if hpf.endian != binary.BigEndian {
			t.Errorf("headPayloadFormat.endian is %v, want BigEndian", hpf.endian)
		}

	default:
		t.Errorf("expected type headPayloadFormat, got %v", hpf)
	}
	x[4] = 'Q'
	if _, err = parseTLV(x); err != nil {
		t.Errorf("Error on parseTLV with mixed format characters: %v", err)
	}
	x[3] = 'a'
	if _, err = parseTLV(x); err == nil {
		t.Errorf("Expect error on parseTLV with unknown format character")
	}

	// Check all types
	x[2] = '<'
	x[3] = 'a'
	x[4] = 0
	var tests = []struct {
		tag   byte
		dtype reflect.Kind
	}{
		{'b', reflect.Int8},
		{'B', reflect.Uint8},
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
		tlvs, err = parseTLV(x)
		if err != nil {
			t.Errorf("parseTLV failed on format string '%s'", string(x[2:]))
		}
		if len(tlvs) != 1 {
			t.Errorf("parseTLV() for headPayloadFormat returns array length %d, want 1", len(tlvs))
		}
		h, ok := tlvs[0].(*headPayloadFormat)
		if !ok {
			t.Errorf("parseTLV()[0] is %v, fails type assertion to &headPayloadFormat", tlvs[0])
		}
		if h.dtype[0] != test.dtype {
			t.Errorf("parseTLV for headPayloadFormat is dtype %v for tag '%c', want %v", h.dtype, test.tag, test.dtype)
		}
		if h.endian == binary.BigEndian {
			t.Errorf("parseTLV for headPayloadFormat is BigEndian, want LittleEndian")
		}
		if h.nvals != 1 {
			t.Errorf("parseTLV for headPayloadFormat has nvals %d, want 1", h.nvals)
		}
	}

	// Try payload shape
	ps := new(headPayloadShape)
	ps.Sizes = append(ps.Sizes, int16(5))
	ps.Sizes = append(ps.Sizes, int16(6))
	pp := payloadshapeToPacket(ps)
	tlvs, err = parseTLV(pp)
	if err != nil {
		t.Errorf("parseTLV() for headPayloadShape returns %v", err)
	}
	if len(tlvs) != 1 {
		t.Errorf("parseTLV() for headPayloadShape returns array length %d, want 1", len(tlvs))
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
	s := make([]byte, 8)
	if _, err := parseTLV(s[:7]); err == nil {
		t.Errorf("parseTLV on fewer than 8 bytes should error")
	}
	s[0] = tlvFORMAT
	s[1] = 0
	if _, err := parseTLV(s); err == nil {
		t.Errorf("parseTLV on L=0 should error")
	}
	s[1] = 2
	if _, err := parseTLV(s); err == nil {
		t.Errorf("parseTLV with L=2 but size-8 string should error")
	}
	s[0] = tlvTIMESTAMPUNIT
	s[1] = 1
	if _, err := parseTLV(s); err == nil {
		t.Errorf("parseTLV without timestamp upper bytes should error")
	}
	s2 := make([]byte, 16)
	s2[0] = tlvCOUNTER
	s2[1] = 2
	if _, err := parseTLV(s2); err == nil {
		t.Errorf("parseTLV with L>1 counters should error")
	}
	s[0] = tlvSHAPE
	s[1] = 1
	s[2] = 0
	if _, err := parseTLV(s); err == nil {
		t.Errorf("parseTLV on shape without any dimensions should error")
	}
	expectZeroPads := []uint8{tlvCHANOFFSET, tlvTAG}
	for _, v := range expectZeroPads {
		s[0] = v
		s[2] = 9
		if _, err := parseTLV(s); err == nil {
			t.Errorf("parseTLV on type 0x%x should require 0x0000 padding", v)
		}
	}
	tryTheseTypes := []uint8{0x40, 0xf0, 0xff}
	for _, v := range tryTheseTypes {
		s[0] = v
		if _, err := parseTLV(s); err != nil {
			t.Errorf("parseTLV on type 0x%x should not error, but error %v", v, err)
		}
	}
}

func TestFullPackets(t *testing.T) {
	p := NewPacket(12, 99, 100, 0)
	nd := 100
	dims := []int16{8}
	d16 := make([]int16, nd)
	d32 := make([]int32, nd)
	d64 := make([]int64, nd)
	for i := 0; i < nd; i++ {
		d16[i] = 20 * int16(i)
		d32[i] = 20 * int32(i)
		d64[i] = 20 * int64(i)
	}
	alldata := []any{d16, d32, d64}
	for _, d := range alldata {
		if err := p.NewData(d, dims); err != nil {
			t.Errorf("Packet.NewData failed: %v", err)
		}
		b := p.Bytes()
		pread, err := ReadPacket(bytes.NewReader(b))
		if err != nil {
			t.Errorf("Could not ReadPacket: %s", err)
			continue
		} else if pread == nil {
			t.Errorf("Could not ReadPacket: returned nil")
			continue
		}
		if pread.Data == nil {
			t.Errorf("ReadPacket yielded no data, expect %v\n", reflect.TypeOf(d))
		}
		if ts := pread.Timestamp(); ts != nil {
			t.Errorf("ReadPacket.Timestamp() yielded %v, expected nil", ts)
		}

		// Test PretendPacket
		arbSeqNum := uint32(34567)
		pfake := p.MakePretendPacket(arbSeqNum, int(dims[0]))
		if pfake == p {
			t.Errorf("MakePretendPacket() result points to original packet")
		}
		if arbSeqNum != pfake.SequenceNumber() {
			t.Errorf("MakePretendPacket(%d) result has seq num %d, want %d", arbSeqNum,
				pfake.SequenceNumber(), arbSeqNum)
		}
		nsamp := pfake.Frames()
		for i := 0; i < nsamp; i++ {
			v := pfake.ReadValue(i)
			want := 20 * (i % 8)
			if v != want {
				t.Errorf("Packet.ReadValue(%d)=%d, want %d", i, v, want)
			}
		}
		v := pfake.ReadValue(-1)
		if v != 0 {
			t.Errorf("Packet.ReadValue(-1)=%d, want 0", v)
		}
		// Make sure we didn't clobber the original packet data
		nsamp = p.Frames()
		for i := 0; i < nsamp; i++ {
			v := p.ReadValue(i)
			want := 20 * i
			if v != want {
				t.Errorf("Packet.ReadValue(%d)=%d, want %d", i, v, want)
			}
		}
	}

	p.ClearData()

	nd = 1024
	d64 = make([]int64, nd)
	for i := 0; i < nd; i++ {
		d64[i] = int64(i)
	}
	if err := p.NewData(d64, dims); err == nil {
		t.Errorf("Packet.NewData expected to fail with packet bigger than %d", maxPACKETLENGTH)
	}

	if err := p.NewData(64, dims); err == nil {
		t.Errorf("Packet.NewData should error on arbitrary non-array data")
	}

	const firstSeq = uint32(950)
	p = NewPacket(12, 99, firstSeq-1, 0)
	for i := firstSeq; i < firstSeq+10; i++ {
		if err := p.NewData(d16, dims); err != nil {
			t.Errorf("Packet.NewData failed: %s", err)
		}
		b := p.Bytes()
		pread, err := ReadPacket(bytes.NewReader(b))
		if err != nil {
			t.Errorf("Could not ReadPacket: %s", err)
			continue
		} else if pread == nil {
			t.Errorf("Could not ReadPacket: returned nil")
			continue
		}
		if pread.sequenceNumber != i {
			t.Errorf("Packet has sequence number %d, want %d", pread.sequenceNumber, i)
		}
		if pread.SequenceNumber() != i {
			t.Errorf("Packet returns SequenceNumber()=%d, want %d", pread.sequenceNumber, i)
		}
	}
	ts := new(PacketTimestamp)
	ts.Rate = 250e8
	ts.T = 0x123456
	p.SetTimestamp(ts)
	tsCopy := p.Timestamp()
	if tsCopy == nil {
		t.Errorf("Packet.Timestamp() returns nil, want %v", ts)
	} else if tsCopy.Rate != ts.Rate || tsCopy.T != ts.T {
		t.Errorf("Packet.Timestamp() returns %v, want %v", tsCopy, ts)
	}

}

func TestExamplePackets(t *testing.T) {
	datasource := filepath.Join("..", "testData", "test1.bin")
	f, err := os.Open(datasource)
	if err != nil {
		t.Errorf("could not open %s", datasource)
	}
	defer f.Close()

	for i := 0; ; i++ {
		h, err := ReadPacket(f)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		} else if err != nil {
			t.Errorf("could not read header from %s: %v", datasource, err)
		}
		if h.offset != 0 {
			t.Errorf("header channel offset %d, want 0", h.offset)
		}
		// loc, _ := f.Seek(0, 1)
		// fmt.Printf("Read packet %4d  of size %4d  fmt %s now at byte %8d\n", i,
		// 	h.packetLength, h.format.rawfmt, loc)
	}
}

func TestExtTriggerPackets(t *testing.T) {
	datasource := filepath.Join("..", "testData", "timer_packets.bin")
	f, err := os.Open(datasource)
	if err != nil {
		t.Errorf("could not open %s", datasource)
	}
	defer f.Close()
	expect_offset := []headChannelOffset{0, 0, 0, 0x3000}
	isExtTrig := []bool{true, true, true, false}

	for i := 0; ; i++ {
		h, err := ReadPacket(f)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		} else if err != nil {
			t.Errorf("could not read header from %s: %v", datasource, err)
		}
		if h.offset != expect_offset[i] {
			t.Errorf("header channel offset %d, want 0", h.offset)
		}
		if h.IsExternalTrigger() != isExtTrig[i] {
			t.Errorf("IsExternalTrigger() %t, want %t", h.IsExternalTrigger(), isExtTrig[i])
		}
		// loc, _ := f.Seek(0, 1)
		// fmt.Printf("Read packet %4d  of size %4d  fmt %s now at byte %8d\n", i,
		// 	h.packetLength, h.format.rawfmt, loc)
		// fmt.Println(h.Frames(), h.Length(), h.SequenceNumber())
		// spew.Dump(h)
		// spew.Dump(h.offset)
		// spew.Dump(h.payloadLabel)
		// spew.Dump(h.shape)
		// spew.Dump(h.offset)
		// spew.Dump(h.timestamp)
		// spew.Dump(h.format.wordlen, h.format.dtype)
		// fmt.Println("figfig", h.ReadValue(0))
	}
}

func BenchmarkPacketEncoding(b *testing.B) {
	Npackets := 2000
	Nsamples := 2500
	packets := make([]*Packet, Npackets)
	payload := make([]int16, Nsamples)
	for i := range Nsamples {
		payload[i] = int16(i)
	}
	dims := make([]int16, 1)
	dims[0] = int16(Nsamples)
	// ctrpk := counterToPacket(&HeadCounter{5, 99})
	// fmt.Printf("ctrpk: %v\n", ctrpk)

	for i := range Npackets {
		p := NewPacket(1, 20, uint32(1000+i), 1)
		p.NewData(payload, dims)
		// p.otherTLV = append(p.otherTLV, ctrpk)
		// p.otherTLV = append(p.otherTLV, ctrpk)
		packets[i] = p
	}

	for i := 0; b.Loop(); i++ {
		for range Npackets {
			p := packets[i]
			p.Bytes()
		}
	}
}

func BenchmarkPacketDecoding(b *testing.B) {
	Npackets := 2000
	Nsamples := 2500
	payload := make([]int16, Nsamples)
	for i := range Nsamples {
		payload[i] = int16(i)
	}
	dims := make([]int16, 1)
	dims[0] = int16(Nsamples)

	var buf bytes.Buffer // A Buffer needs no initialization.

	for i := range Npackets {
		p := NewPacket(1, 20, uint32(1000+i), 1)
		p.NewData(payload, dims)
		buf.Write(p.Bytes())
	}
	fulltext := buf.Bytes()

	for b.Loop() {
		b2 := bytes.NewBuffer(fulltext)
		for range Npackets {
			if _, err := ReadPacket(b2); err != nil {
				b.Errorf("Could not read packet with error %v", err)
			}
		}
	}
}
