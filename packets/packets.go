package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
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
	format *headPayloadFormat
	shape  *headPayloadShape
	offset headChannelOffset

	// Any other TLV objects.
	otherTLV []interface{}

	// The data payload
	data interface{}
}

// packetMAGIC is the packet header's magic number.
const packetMAGIC uint32 = 0x810b00ff
const maxPACKETLENGTH int = 8192

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
	p.payloadLength = 0
	p.packetLength = 24
	p.format = nil
	p.data = nil
	p.shape = nil
	return nil
}

// String returns a string summarizing the packet's version, sequence number, and size.
func (p *Packet) String() string {
	return fmt.Sprintf("Packet v0x%2.2x 0x%8.8x  Size (%2d+%5d)", p.version,
		p.sequenceNumber, p.headerLength, p.payloadLength)
}

// Length returns the length of the entire packet, in bytes
func (p *Packet) Length() int {
	return p.packetLength
}

// NewData adds data to the packet, and crates the format and shape TLV items to match.
func (p *Packet) NewData(data interface{}, dims []int16) error {
	ndim := len(dims)
	p.headerLength = 24
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
		p.data = d
	case []int32:
		pfmt.rawfmt = "<i"
		pfmt.dtype[0] = reflect.Int32
		pfmt.wordlen = 4
		p.payloadLength = uint16(pfmt.wordlen * len(d))
		p.data = d
	case []int64:
		pfmt.rawfmt = "<q"
		pfmt.dtype[0] = reflect.Int64
		pfmt.wordlen = 8
		p.payloadLength = uint16(pfmt.wordlen * len(d))
		p.data = d
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
	binary.Write(buf, binary.BigEndian, byte(0x23))
	binary.Write(buf, binary.BigEndian, byte(1))
	binary.Write(buf, binary.BigEndian, uint16(0))
	binary.Write(buf, binary.BigEndian, uint32(p.offset))

	if p.data != nil && p.shape != nil && p.format != nil {
		binary.Write(buf, binary.BigEndian, byte(0x21))
		binary.Write(buf, binary.BigEndian, byte(1))
		fmt := []byte(p.format.rawfmt)
		if len(fmt) > 6 {
			fmt = fmt[:6]
		}
		for len(fmt) < 6 {
			fmt = append(fmt, 0x0)
		}
		binary.Write(buf, binary.BigEndian, fmt)

		binary.Write(buf, binary.BigEndian, byte(0x22))
		binary.Write(buf, binary.BigEndian, byte(1+len(p.shape.Sizes)/4))
		for i := 0; i < len(p.shape.Sizes); i++ {
			binary.Write(buf, binary.BigEndian, p.shape.Sizes[i])
		}
		for i := len(p.shape.Sizes) % 4; i < 3; i++ {
			zero := int16(0)
			binary.Write(buf, binary.BigEndian, &zero)
		}

		binary.Write(buf, p.format.endian, p.data)
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

// ReadPacket returns a Packet read from an io.reader
func ReadPacket(data io.Reader) (h *Packet, err error) {
	h = new(Packet)
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
	if h.payloadLength%8 != 0 {
		return nil, fmt.Errorf("Header payload length is %d, expect multiple of 8", h.payloadLength)
	}
	h.packetLength = int(h.headerLength) + int(h.payloadLength)
	var magic uint32
	if err = binary.Read(data, binary.BigEndian, &magic); err != nil {
		return nil, err
	}
	if magic != packetMAGIC {
		return nil, fmt.Errorf("Magic was 0x%x, want 0x%x", magic, packetMAGIC)
	}
	if err = binary.Read(data, binary.BigEndian, &h.sourceID); err != nil {
		return nil, err
	}
	if err = binary.Read(data, binary.BigEndian, &h.sequenceNumber); err != nil {
		return nil, err
	}
	allTLV, err := readTLV(data, h.headerLength-MINLENGTH)
	if err != nil {
		return nil, err
	}

	for _, tlv := range allTLV {
		switch val := tlv.(type) {
		case headChannelOffset:
			h.offset = val
		case *headPayloadShape:
			h.shape = val
		case *headPayloadFormat:
			h.format = val
		default:
			h.otherTLV = append(h.otherTLV, val)
		}
	}

	if h.payloadLength > 0 && h.format != nil {
		if len(h.format.dtype) == 1 {

			switch h.format.dtype[0] {
			case reflect.Int16:
				result := make([]int16, h.payloadLength/2)
				if err = binary.Read(data, h.format.endian, result); err != nil {
					return nil, err
				}
				h.data = result

			case reflect.Int32:
				result := make([]int32, h.payloadLength/4)
				if err = binary.Read(data, h.format.endian, result); err != nil {
					return nil, err
				}
				h.data = result

			case reflect.Int64:
				result := make([]int64, h.payloadLength/8)
				if err = binary.Read(data, h.format.endian, result); err != nil {
					return nil, err
				}
				h.data = result

			default:
				return nil, fmt.Errorf("Did not know how to read type %v", h.format.dtype)
			}
		} else {
			result := make([]byte, h.payloadLength)
			if err = binary.Read(data, h.format.endian, result); err != nil {
				return nil, err
			}
			h.data = result
		}
	}

	return h, nil
}

// headTimestamp represents a single timestamp in the header
type headTimestamp uint64

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

// readTLV reads data for size bytes, generating a list of all TLV objects
func readTLV(data io.Reader, size uint8) (result []interface{}, err error) {
	var t uint8
	var tlvsize uint8
	for size > 0 {
		if size < 8 {
			return result, fmt.Errorf("readTLV needs to read multiples of 8 bytes")
		}
		if err = binary.Read(data, binary.BigEndian, &t); err != nil {
			return result, err
		}
		if err = binary.Read(data, binary.BigEndian, &tlvsize); err != nil {
			return result, err
		}
		if 8*tlvsize > size {
			return result, fmt.Errorf("TLV type %d has len 8*%d, but remaining hdr size is %d",
				t, tlvsize, size)
		}
		switch t {
		case 0x0: //NULL
			// do nothing

		case 0x11: // timestamps
			var x uint16
			var y uint32
			if err = binary.Read(data, binary.BigEndian, &x); err != nil {
				return result, err
			}
			if err = binary.Read(data, binary.BigEndian, &y); err != nil {
				return result, err
			}
			tstamp := headTimestamp(x) << 32
			tstamp += headTimestamp(y)
			result = append(result, tstamp)

		case 0x12: // counter
			ctr := new(HeadCounter)
			if tlvsize != 1 {
				return result, fmt.Errorf("TLV counter size %d, must be size 1 (32 bits) as currently implemented", tlvsize)
			}
			if err = binary.Read(data, binary.BigEndian, &ctr.ID); err != nil {
				return result, err
			}
			if err = binary.Read(data, binary.BigEndian, &ctr.Count); err != nil {
				return result, err
			}
			result = append(result, ctr)

		case 0x21: // Payload format descriptor
			b := make([]byte, 8*int(tlvsize)-2)
			if n, err := data.Read(b); err != nil || n < len(b) {
				return result, err
			}
			pfmt := new(headPayloadFormat)
			pfmt.rawfmt = string(b)
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

		case 0x22: // Payload shape
			shape := new(headPayloadShape)
			var d int16
			for i := 0; i < 8*int(tlvsize)-2; i += 2 {
				if err = binary.Read(data, binary.BigEndian, &d); err != nil {
					return result, err
				}
				if d > 0 {
					shape.Sizes = append(shape.Sizes, d)
				}
			}
			result = append(result, shape)

		case 0x23: // Channel offset
			var pad uint16
			var offset headChannelOffset
			if err = binary.Read(data, binary.BigEndian, &pad); err != nil {
				return result, err
			}
			if pad != 0 {
				return result, fmt.Errorf("channel offset packet contains padding %du, want 0", pad)
			}
			if err = binary.Read(data, binary.BigEndian, &offset); err != nil {
				return result, err
			}
			result = append(result, offset)

		default:
			return result, fmt.Errorf("Unknown TLV type %d", t)
		}

		size -= 8 * tlvsize
	}
	return
}
