package packets

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"os"
	"reflect"
	"testing"
	// "github.com/davecgh/go-spew/spew"
)

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
	if _, err2 := ReadPacket(bytes.NewReader([]byte{})); err2 == nil {
		t.Errorf("ReadPacket should fail if version cannot be read")
	}
	if _, err2 := ReadPacket(bytes.NewReader([]byte{0})); err2 == nil {
		t.Errorf("ReadPacket should fail if header length cannot be read")
	}
	if _, err2 := ReadPacket(bytes.NewReader([]byte{0, 15})); err2 == nil {
		t.Errorf("ReadPacket should fail if header length < 16")
	}
	if _, err2 := ReadPacket(bytes.NewReader([]byte{0, 16})); err2 == nil {
		t.Errorf("ReadPacket should fail if payload length cannot be read")
	}
	if _, err2 := ReadPacket(bytes.NewReader([]byte{0, 16, 0, 0})); err2 == nil {
		t.Errorf("ReadPacket should fail if magic cannot be read")
	}
	if _, err2 := ReadPacket(bytes.NewReader([]byte{0, 16, 0, 0, 1, 2, 3, 4})); err2 == nil {
		t.Errorf("ReadPacket should fail if magic is wrong")
	}
	if _, err2 := ReadPacket(bytes.NewReader([]byte{0, 16, 0, 0, 0x81, 0xb, 0, 0xff})); err2 == nil {
		t.Errorf("ReadPacket should fail if source ID cannot be read")
	}
	if _, err2 := ReadPacket(bytes.NewReader([]byte{0, 16, 0, 0, 0x81, 0xb, 0, 0xff, 0, 0, 0, 0})); err2 == nil {
		t.Errorf("ReadPacket should fail if sequence number cannot be read")
	}
	if _, err2 := ReadPacket(bytes.NewReader([]byte{0, 16, 0, 9, 0x81, 0xb, 0, 0xff, 0, 0, 0, 0, 0, 0, 0, 0})); err2 == nil {
		t.Errorf("ReadPacket should fail if payload length is not a multiple of 8")
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

	// Try a timestamp, with and without units
	ts := PacketTimestamp{0x0000030405060708, 0}
	tp := timestampToPacket(&ts)
	tlvs, err = readTLV(bytes.NewReader(tp), 8)
	if err != nil {
		t.Errorf("readTLV() for PacketTimestamp returns %v", err)
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
	tlvs, err = readTLV(bytes.NewReader(tp), 16)
	if err != nil {
		t.Errorf("readTLV() for PacketTimestamp returns %v", err)
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
		tlvs, err = readTLV(bytes.NewReader(tp), 16)
		if err != nil {
			t.Errorf("readTLV() for PacketTimestamp returns %v", err)
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
	if _, err = readTLV(bytes.NewReader(tp), 16); err == nil {
		t.Errorf("readTLV() for PacketTimestamp succeeds with %d bits (>64), should fail", tp[2])
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
	x := []byte{tlvFORMAT, 1, '>', 'i', 'i', 0, 0, 0}
	tlvs, err = readTLV(bytes.NewReader(x), 8)
	if err != nil {
		t.Errorf("readTLV() for headPayloadFormat returns %v", err)
	}
	if len(tlvs) != 1 {
		t.Errorf("readTLV() for headPayloadFormat returns array length %d, want 1", len(tlvs))
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
	if _, err = readTLV(bytes.NewReader(x), 8); err != nil {
		t.Errorf("Error on readTLV with mixed format characters: %v", err)
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
		if h.dtype[0] != test.dtype {
			t.Errorf("readTLV for headPayloadFormat is dtype %v for tag '%c', want %v", h.dtype, test.tag, test.dtype)
		}
		if h.endian == binary.BigEndian {
			t.Errorf("readTLV for headPayloadFormat is BigEndian, want LittleEndian")
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
	if _, err := readTLV(bytes.NewReader([]byte{tlvFORMAT}), 8); err == nil {
		t.Errorf("readTLV on short string should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{tlvFORMAT, 0x2}), 8); err == nil {
		t.Errorf("readTLV with L=2 but size-8 string should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{tlvTIMESTAMP, 0x1}), 8); err == nil {
		t.Errorf("readTLV without timestamp upper bytes should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{tlvTIMESTAMP, 0x1, 0, 0}), 8); err == nil {
		t.Errorf("readTLV without timestamp lower bytes should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{tlvCOUNTER, 0x2}), 16); err == nil {
		t.Errorf("readTLV with L>1 counters should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{tlvCOUNTER, 0x1, 0}), 8); err == nil {
		t.Errorf("readTLV without counter ID should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{tlvCOUNTER, 0x1, 0, 0}), 8); err == nil {
		t.Errorf("readTLV without counter value should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{tlvFORMAT, 1}), 8); err == nil {
		t.Errorf("readTLV on payload format with short string should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{tlvSHAPE, 1, 0}), 8); err == nil {
		t.Errorf("readTLV on channel offset without any dimensions should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{tlvCHANOFFSET, 1, 0}), 8); err == nil {
		t.Errorf("readTLV on channel offset without padding should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{tlvCHANOFFSET, 1, 0, 9}), 8); err == nil {
		t.Errorf("readTLV on channel offset with nonzero padding should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{tlvCHANOFFSET, 1, 0, 0, 0}), 8); err == nil {
		t.Errorf("readTLV on channel offset without offset should error")
	}
	if _, err := readTLV(bytes.NewReader([]byte{0xff, 1}), 8); err == nil {
		t.Errorf("readTLV on unknown type should error")
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
		d16[i] = 2 * int16(i)
		d32[i] = 20 * int32(i)
		d64[i] = 20 * int64(i)
	}
	alldata := []interface{}{d16, d32, d64}
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
		arbVal := 48
		pfake := p.MakePretendPacket(arbSeqNum, arbVal)
		if pfake == p {
			t.Errorf("MakePretendPacket() result points to original packet")
		}
		if arbSeqNum != pfake.SequenceNumber() {
			t.Errorf("MakePretendPacket(%d) result has seq num %d, want %d", arbSeqNum,
				pfake.SequenceNumber(), arbSeqNum)
		}
		nsamp := pfake.Frames()
		for i := 0; i<nsamp; i++ {
			v := pfake.ReadValue(i)
			if v != arbVal {
				t.Errorf("Packet.ReadValue(%d)=%d, want %d", i, v, arbVal)
			}
		}
		v := pfake.ReadValue(-1)
		if v != 0 {
			t.Errorf("Packet.ReadValue(-1)=%d, want 0", v)
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
	datasource := "../testData/test1.bin"
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
