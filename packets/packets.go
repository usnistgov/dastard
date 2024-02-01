package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"reflect"

	"github.com/usnistgov/dastard/getbytes"
)

// Packet represents the header of an Abaco data packet
type Packet struct {
	// Items in the required part of the header
	version        uint8
	headerLength   uint8
	payloadLength  uint16
	sourceID       uint32
	sequenceNumber uint32
	packetLength   int

	// Expected TLV objects. If 0 or 2+ examples, this cannot be processed
	format    *headPayloadFormat
	shape     *headPayloadShape
	offset    headChannelOffset
	timestamp *PacketTimestamp

	// Optional TLV objects.
	payloadLabel   PayloadLabel
	// Any other TLV objects.
	otherTLV []interface{}

	// The data payload
	Data interface{}
}

// packetMAGIC is the packet header's magic number.
const packetMAGIC uint32 = 0x810b00ff
const maxPACKETLENGTH int = 8192

// TLV types
const (
	tlvNULL          = byte(0)
	tlvTAG           = byte(0x09)
	tlvTIMESTAMP     = byte(0x11)
	tlvCOUNTER       = byte(0x12)
	tlvTIMESTAMPUNIT = byte(0x13)
	tlvFORMAT        = byte(0x21)
	tlvSHAPE         = byte(0x22)
	tlvCHANOFFSET    = byte(0x23)
	tlvPAYLOADLABEL  = byte(0x29)
	tlvINVALID       = byte(0xff)
)

// NewPacket generates a new packet with the given facts. No data are configured or stored.
func NewPacket(version uint8, sourceID uint32, sequenceNumber uint32, chanOffset int) *Packet {
	p := new(Packet)
	p.version = version
	p.headerLength = 24 // will update as info is added
	p.sourceID = sourceID
	p.sequenceNumber = sequenceNumber
	p.offset = headChannelOffset(chanOffset)
	return p
}

// ClearData removes the data payload from a packet.
func (p *Packet) ClearData() error {
	p.headerLength = 24
	if p.timestamp != nil {
		p.headerLength += 16
	}
	p.payloadLength = 0
	p.packetLength = int(p.headerLength)
	p.format = nil
	p.Data = nil
	p.shape = nil
	return nil
}

// String returns a string summarizing the packet's version, sequence number, and size.
func (p *Packet) String() string {
	return fmt.Sprintf("Packet v:0x%2.2x sn:0x%8.8x  Size (%2d+%4d)", p.version,
		p.sequenceNumber, p.headerLength, p.payloadLength)
}

// Length returns the length of the entire packet, in bytes
func (p *Packet) Length() int {
	return p.packetLength
}

// Frames returns the number of data frames in the packet
func (p *Packet) Frames() int {
	if p.shape == nil {
		return 0
	}
	nchan := 1
	for _, s := range p.shape.Sizes {
		if s > 0 {
			nchan *= int(s)
		}
	}

	switch d := p.Data.(type) {
	case []int16:
		return len(d) / nchan
	case []int32:
		return len(d) / nchan
	case []int64:
		return len(d) / nchan
	default:
		return 0
	}
}

// SequenceNumber returns the packet's internal sequenceNumber
func (p *Packet) SequenceNumber() uint32 {
	return p.sequenceNumber
}

// Timestamp returns a copy of the first PacketTimestamp found in the header, or nil if none.
func (p *Packet) Timestamp() *PacketTimestamp {
	if p.timestamp != nil {
		tsCopy := new(PacketTimestamp)
		tsCopy.Rate = p.timestamp.Rate
		tsCopy.T = p.timestamp.T
		return tsCopy
	}
	return nil
}

// SetTimestamp puts timestamp `ts` into the header.
func (p *Packet) SetTimestamp(ts *PacketTimestamp) error {
	if p.timestamp == nil {
		p.headerLength += 16
		p.packetLength += 16
	}
	p.timestamp = ts
	return nil
}

// ResetTimestamp removes any timestamp from the header.
func (p *Packet) ResetTimestamp() error {
	if p.timestamp != nil {
		p.headerLength -= 16
		p.packetLength -= 16
	}
	p.timestamp = nil
	return nil
}

// MakePretendPacket generates a copy of p with the given sequence number.
// Each channel will repeat the first value in p.
// Use it for making fake data to fill in where packets were dropped.
func (p *Packet) MakePretendPacket(seqnum uint32, nchan int) *Packet {
	pretend := *p
	pretend.sequenceNumber = seqnum
	switch d := p.Data.(type) {
	case []int16:
		x := make([]int16, len(d))
		for i := range d {
			x[i] = d[i%nchan]
		}
		pretend.Data = x
	case []int32:
		x := make([]int32, len(d))
		for i := range d {
			x[i] = d[i%nchan]
		}
		pretend.Data = x
	case []int64:
		x := make([]int64, len(d))
		for i := range d {
			x[i] = d[i%nchan]
		}
		pretend.Data = x
	}
	return &pretend
}

// ReadValue returns a single sample from the packet's data payload.
// Not efficient for reading the whole data slice.
func (p *Packet) ReadValue(sample int) int {
	if sample < 0 || sample >= p.Frames() {
		return 0
	}
	switch d := p.Data.(type) {
	case []int16:
		return int(d[sample])
	case []int32:
		return int(d[sample])
	case []int64:
		return int(d[sample])
	}
	return 0
}

// NewData adds data to the packet, and creates the format and shape TLV items to match.
func (p *Packet) NewData(data interface{}, dims []int16) error {
	ndim := len(dims)
	p.headerLength = 24
	if p.timestamp != nil {
		p.headerLength += 16
	}
	pfmt := new(headPayloadFormat)
	pfmt.dtype = make([]reflect.Kind, 1)
	pfmt.endian = binary.LittleEndian
	pfmt.nvals = 1
	switch d := data.(type) {
	case []int16:
		pfmt.rawfmt = "<h"
		pfmt.dtype[0] = reflect.Int16
		pfmt.wordlen = 2
		p.payloadLength = uint16(pfmt.wordlen * len(d))
		p.Data = d
	case []int32:
		pfmt.rawfmt = "<i"
		pfmt.dtype[0] = reflect.Int32
		pfmt.wordlen = 4
		p.payloadLength = uint16(pfmt.wordlen * len(d))
		p.Data = d
	case []int64:
		pfmt.rawfmt = "<q"
		pfmt.dtype[0] = reflect.Int64
		pfmt.wordlen = 8
		p.payloadLength = uint16(pfmt.wordlen * len(d))
		p.Data = d
	default:
		return fmt.Errorf("Could not handle Packet.NewData of type %v", reflect.TypeOf(d))
	}
	p.format = pfmt
	p.headerLength += 8
	p.shape = new(headPayloadShape)
	p.shape.Sizes = make([]int16, 1)
	for i := 0; i < ndim; i++ {
		p.shape.Sizes[i] = dims[i]
	}
	p.headerLength += 8 * uint8(1+ndim/4)
	p.packetLength = int(p.headerLength) + int(p.payloadLength)
	if p.packetLength > maxPACKETLENGTH {
		return fmt.Errorf("packet length %d exceeds max of %d", p.packetLength, maxPACKETLENGTH)
	}
	p.sequenceNumber++
	return nil
}

// Bytes converts the Packet p to a []byte slice for transport.
func (p *Packet) Bytes() []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, p.version)
	binary.Write(buf, binary.BigEndian, p.headerLength)
	binary.Write(buf, binary.BigEndian, p.payloadLength)
	binary.Write(buf, binary.BigEndian, packetMAGIC)
	binary.Write(buf, binary.BigEndian, p.sourceID)
	binary.Write(buf, binary.BigEndian, p.sequenceNumber)

	// Channel offset
	binary.Write(buf, binary.BigEndian, byte(tlvCHANOFFSET))
	binary.Write(buf, binary.BigEndian, byte(1))
	binary.Write(buf, binary.BigEndian, uint16(0))
	binary.Write(buf, binary.BigEndian, uint32(p.offset))

	// Write any timestamps
	if ts := p.Timestamp(); ts != nil {
		var nbits uint8
		var exp int8
		var num, denom uint16
		nbits = 64
		exp = -11 // precise to 10 ps
		period := math.Pow10(-int(exp)) / ts.Rate
		denom = 1
		for ; period > 65535; period *= 0.5 {
			denom *= 2
		}
		num = uint16(math.Round(period))
		binary.Write(buf, binary.BigEndian, byte(tlvTIMESTAMPUNIT))
		binary.Write(buf, binary.BigEndian, byte(2))
		binary.Write(buf, binary.BigEndian, byte(nbits))
		binary.Write(buf, binary.BigEndian, byte(exp))
		binary.Write(buf, binary.BigEndian, num)
		binary.Write(buf, binary.BigEndian, denom)
		binary.Write(buf, binary.BigEndian, ts.T)
	}

	if p.Data != nil && p.shape != nil && p.format != nil {
		binary.Write(buf, binary.BigEndian, byte(tlvFORMAT))
		binary.Write(buf, binary.BigEndian, byte(1))
		rfmt := []byte(p.format.rawfmt)
		if len(rfmt) > 6 {
			rfmt = rfmt[:6]
		}
		for len(rfmt) < 6 {
			rfmt = append(rfmt, 0x0)
		}
		binary.Write(buf, binary.BigEndian, rfmt)

		binary.Write(buf, binary.BigEndian, byte(tlvSHAPE))
		binary.Write(buf, binary.BigEndian, byte(1+len(p.shape.Sizes)/4))
		for i := 0; i < len(p.shape.Sizes); i++ {
			binary.Write(buf, binary.BigEndian, p.shape.Sizes[i])
		}
		for i := len(p.shape.Sizes) % 4; i < 3; i++ {
			zero := int16(0)
			binary.Write(buf, binary.BigEndian, &zero)
		}

		if p.format.endian == binary.BigEndian {
			binary.Write(buf, p.format.endian, p.Data)
		} else {
			switch d := p.Data.(type) {
			case []int16:
				b := getbytes.FromSliceInt16(d)
				buf.Write(b)
			default:
				binary.Write(buf, p.format.endian, p.Data)
			}
		}
	}
	return buf.Bytes()
}

// ChannelInfo returns the number of channels in this packet, and the first one
func (p *Packet) ChannelInfo() (nchan, offset int) {
	nchan = 1
	for _, s := range p.shape.Sizes {
		if s > 0 {
			nchan *= int(s)
		}
	}
	return nchan, int(p.offset)
}

// ReadPacketPlusPad reads a packet from data, then consumes the padding bytes
// that follow (if any) so that a multiple of stride bytes is read.
func ReadPacketPlusPad(data io.Reader, stride int) (p *Packet, err error) {
	p, err = ReadPacket(data)
	if err != nil {
		return p, err
	}

	// Seek past the padding bytes
	overhang := p.Length() % stride
	if overhang > 0 {
		padsize := int64(stride - overhang)
		// _, err = data.Seek(int64(padsize), io.SeekCurrent); err != nil {
		if _, err = io.CopyN(ioutil.Discard, data, padsize); err != nil {
			return nil, err
		}
	}

	return p, nil
}

func byteSwap2(b []byte, nb int) error {
	switch nb {
	case 2:
		for i := 0; i < len(b); i += nb {
			b[i], b[i+1] = b[i+1], b[i]
		}
	case 4:
		for i := 0; i < len(b); i += nb {
			b[i], b[i+3] = b[i+3], b[i]
			b[i+1], b[i+2] = b[i+2], b[i+1]
		}
	case 8:
		for i := 0; i < len(b); i += nb {
			b[i], b[i+7] = b[i+7], b[i]
			b[i+1], b[i+6] = b[i+6], b[i+1]
			b[i+2], b[i+5] = b[i+5], b[i+2]
			b[i+3], b[i+4] = b[i+4], b[i+3]
		}
	default:
		return fmt.Errorf("byteSwap2(b, nb) with nb=%d not allowed", nb)
	}
	return nil
}

func byteSwap(vectorIn interface{}) error {
	switch v := vectorIn.(type) {
	case []uint8:
	case []int8:
		return nil
	case []uint16:
		b := getbytes.FromSliceUint16(v)
		return byteSwap2(b, 2)
	case []int16:
		b := getbytes.FromSliceInt16(v)
		return byteSwap2(b, 2)

	case []uint32:
		b := getbytes.FromSliceUint32(v)
		return byteSwap2(b, 4)
	case []int32:
		b := getbytes.FromSliceInt32(v)
		return byteSwap2(b, 4)

	case []uint64:
		b := getbytes.FromSliceUint64(v)
		return byteSwap2(b, 8)
	case []int64:
		b := getbytes.FromSliceInt64(v)
		return byteSwap2(b, 8)

	default:
		return fmt.Errorf("Cannot byte swap object of type %T", v)
	}
	return nil
}

// ReadPacket returns a Packet read from an io.reader
func ReadPacket(data io.Reader) (p *Packet, err error) {
	const MINLENGTH uint8 = 16
	hdr := make([]byte, MINLENGTH)
	if _, err = io.ReadFull(data, hdr); err != nil {
		return nil, err
	}

	p = new(Packet)
	p.version = hdr[0]
	p.headerLength = hdr[1]
	p.payloadLength = binary.BigEndian.Uint16(hdr[2:])
	if p.headerLength < MINLENGTH {
		return nil, fmt.Errorf("Header length is %d, expect at least %d", p.headerLength, MINLENGTH)
	}
	p.packetLength = int(p.headerLength) + int(p.payloadLength)

	magic := binary.BigEndian.Uint32(hdr[4:])
	p.sourceID = binary.BigEndian.Uint32(hdr[8:])
	p.sequenceNumber = binary.BigEndian.Uint32(hdr[12:])
	if magic != packetMAGIC {
		return nil, fmt.Errorf("Magic was 0x%x, want 0x%x", magic, packetMAGIC)
	}

	tlvdata := make([]byte, p.headerLength-MINLENGTH)
	if _, err = io.ReadFull(data, tlvdata); err != nil {
		return nil, err
	}
	allTLV, err := parseTLV(tlvdata)
	if err != nil {
		return nil, err
	}

	for _, tlv := range allTLV {
		switch val := tlv.(type) {
		case headChannelOffset:
			p.offset = val
		case *headPayloadShape:
			p.shape = val
		case *headPayloadFormat:
			p.format = val
		case *PacketTimestamp:
			p.timestamp = val
		case PayloadLabel:
			p.payloadLabel = val
		default:
			p.otherTLV = append(p.otherTLV, val)
		}
	}

	if p.payloadLength > 0 && p.format != nil {
		if len(p.format.dtype) == 1 {

			switch p.format.dtype[0] {
			case reflect.Int16:
				result := make([]int16, p.payloadLength/2)
				bslice := getbytes.FromSliceInt16(result)
				if _, err = io.ReadFull(data, bslice); err != nil {
					return nil, err
				}
				p.Data = result

			case reflect.Int32:
				result := make([]int32, p.payloadLength/4)
				bslice := getbytes.FromSliceInt32(result)
				if _, err = io.ReadFull(data, bslice); err != nil {
					return nil, err
				}
				p.Data = result

			case reflect.Int64:
				result := make([]int64, p.payloadLength/8)
				bslice := getbytes.FromSliceInt64(result)
				if _, err = io.ReadFull(data, bslice); err != nil {
					return nil, err
				}
				p.Data = result

			default:
				return nil, fmt.Errorf("Did not know how to read type %v", p.format.dtype)
			}
			if p.format.endian == binary.BigEndian {
				if err = byteSwap(p.Data); err != nil {
					return nil, err
				}
			}
		} else {
			result := make([]byte, p.payloadLength)
			if err = binary.Read(data, p.format.endian, result); err != nil {
				return nil, err
			}
			p.Data = result
		}
	}

	return p, nil
}

// PacketTimestamp represents a single timestamp in the header
type PacketTimestamp struct {
	T    uint64  // Counter offset
	Rate float64 // Count rate, in counts per second
}

// PacketTag represents a data type tag.
type PacketTag uint32

// PayloadLabel represents the label (field names) for a payload's data section
type PayloadLabel string

// MakeTimestamp creates a `PacketTimestamp` from data
func MakeTimestamp(x uint16, y uint32, rate float64) *PacketTimestamp {
	ts := new(PacketTimestamp)
	ts.T = uint64(x)<<32 + uint64(y)
	ts.Rate = rate
	return ts
}

// HeadCounter represents a counter found in a packet header
type HeadCounter struct {
	ID    int16
	Count int32
}

// headPayloadFormat represents the payload format header item.
type headPayloadFormat struct {
	endian  binary.ByteOrder
	rawfmt  string
	wordlen int
	nvals   int
	dtype   []reflect.Kind
}

// headChannelOffset represents the offset of the first channel in this packet
type headChannelOffset uint32

// addDataComponent adds a new component of type t to the payload array.
// Currently, it is an error to have a mix of types, though this design could be changed if needed.
func (h *headPayloadFormat) addDataComponent(t reflect.Kind, nb int) error {
	h.dtype = append(h.dtype, t)
	h.nvals++
	h.wordlen += nb
	return nil
}

// headPayloadShape describes the multi-dimensional shape of the payload
type headPayloadShape struct {
	Sizes []int16
}

// parseTLV parses data, generating a list of all TLV objects
func parseTLV(data []byte) (result []interface{}, err error) {
	bytesRemaining := len(data)
	for bytesRemaining > 0 {
		if bytesRemaining < 8 {
			return result, fmt.Errorf("parseTLV needs to read multiples of 8 bytes")
		}
		t := data[0]
		tlvsize := 8 * int(data[1])
		if tlvsize > bytesRemaining {
			return result, fmt.Errorf("TLV type 0x%x has len %d, but remaining hdr size is %d",
				t, tlvsize, bytesRemaining)
		}
		if tlvsize <= 0 {
			return result, fmt.Errorf("TLV reports negative size %d", tlvsize)
		}
		switch t {
		case tlvTAG:
			x := binary.BigEndian.Uint16(data[2:])
			tag := PacketTag(binary.BigEndian.Uint32(data[4:]))
			if x != 0 {
				return result, fmt.Errorf("TAG TLV has value 0x%x, expect 0", x)
			}
			result = append(result, tag)

		case tlvTIMESTAMP: // timestamps without units
			x := binary.BigEndian.Uint16(data[2:])
			y := binary.BigEndian.Uint32(data[4:])

			// we assume a 64 bit int with units of nanoseconds
			// has its 16 least significant bits truncated
			// so the rate is 1e9 but we shift up the number by 16 bits
			ts := MakeTimestamp(x, y, 1e9)
			ts.T = ts.T << 16
			result = append(result, ts)
			// fmt.Println("tlvTIMESTAMP x, y, result", x, y, ts.T)
			// fmt.Println(data[0:8])

		case tlvCOUNTER:
			if tlvsize != 8 {
				return result, fmt.Errorf("TLV counter size %d, must be size 8 (32 bit counter) as currently implemented", tlvsize)
			}
			ctr := new(HeadCounter)
			ctr.ID = int16(binary.BigEndian.Uint16(data[2:]))
			ctr.Count = int32(binary.BigEndian.Uint32(data[4:]))
			result = append(result, ctr)

		case tlvTIMESTAMPUNIT:
			if tlvsize < 16 {
				return result, fmt.Errorf("TLV timestamp-with-unit size %d, must be size at least 16 as currently implemented", tlvsize)
			}
			nbits := data[2]
			exp := int8(data[3])
			num := binary.BigEndian.Uint16(data[4:])
			denom := binary.BigEndian.Uint16(data[6:])
			t := binary.BigEndian.Uint64(data[8:])
			switch {
			case nbits < 64:
				mask := ^(uint64(math.MaxUint64) << nbits)
				t = t & mask
			case nbits == 64:
			default:
				return result, fmt.Errorf("TLV timestamp with unit calls for %d bits", nbits)
			}
			ts := new(PacketTimestamp)
			ts.T = t
			// (num/denom) * pow(10, exp) is the clock period. We want rate = 1/period, so...
			ts.Rate = float64(denom) / float64(num) * math.Pow10(-int(exp))
			result = append(result, ts)

		case tlvFORMAT:
			pfmt := new(headPayloadFormat)
			pfmt.rawfmt = string(data[2:int(tlvsize)])
			for _, c := range pfmt.rawfmt {
				switch c {
				case 0, ' ':
					// ignore null and space characters
				case '!', '>':
					pfmt.endian = binary.BigEndian
				case '<':
					pfmt.endian = binary.LittleEndian
				case 'x':
					err = pfmt.addDataComponent(reflect.Invalid, 1)
				case 'b':
					err = pfmt.addDataComponent(reflect.Int8, 1)
				case 'B':
					err = pfmt.addDataComponent(reflect.Uint8, 1)
				case 'h':
					err = pfmt.addDataComponent(reflect.Int16, 2)
				case 'H':
					err = pfmt.addDataComponent(reflect.Uint16, 2)
				case 'i', 'l':
					err = pfmt.addDataComponent(reflect.Int32, 4)
				case 'I', 'L':
					err = pfmt.addDataComponent(reflect.Uint32, 4)
				case 'q':
					err = pfmt.addDataComponent(reflect.Int64, 8)
				case 'Q':
					err = pfmt.addDataComponent(reflect.Uint64, 8)
				default:
					return result, fmt.Errorf("Unknown data format character '%c' in format '%s'",
						c, pfmt.rawfmt)
				}
				if err != nil {
					return result, err
				}
			}
			result = append(result, pfmt)

		case tlvSHAPE:
			shape := new(headPayloadShape)
			for i := 2; i < tlvsize; i += 2 {
				d := int16(binary.BigEndian.Uint16(data[i:]))
				if d > 0 {
					shape.Sizes = append(shape.Sizes, d)
				}
			}
			if len(shape.Sizes) == 0 {
				return result, fmt.Errorf("shape TLV contains no positive sizes")
			}
			result = append(result, shape)

		case tlvCHANOFFSET:
			pad := binary.BigEndian.Uint16(data[2:])
			offset := headChannelOffset(binary.BigEndian.Uint32(data[4:]))
			if pad != 0 {
				return result, fmt.Errorf("channel offset TLV contains padding %du, want 0", pad)
			}
			result = append(result, offset)

		case tlvPAYLOADLABEL:
			label := PayloadLabel(string(data[2:int(tlvsize)]))
			result = append(result, label)

		default:
			// Ignore the remainder of the TLV
		}

		data = data[tlvsize:]
		bytesRemaining -= tlvsize
	}
	return
}
