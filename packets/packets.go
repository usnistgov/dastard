package packets

import (
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
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
const PACKETMAGIC uint32 = 0x810b00ff

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
	if h.payloadLength%8 != 0 {
		return nil, fmt.Errorf("Header payload length is %d, expect multiple of 8", h.payloadLength)
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

// headTimestamp represents a single timestamp in the header
type headTimestamp uint64

// HeadCounter represents a counter found in a packet header
type HeadCounter struct {
	ID    int16
	Count int32
}

// headPayloadFormat represents the payload format header item.
type headPayloadFormat struct {
	bigendian bool
	rawfmt    string
	nvals     int
	dtype     reflect.Kind
}

// headChannelOffset represents the offset of the first channel in this packet
type headChannelOffset uint32

// addDimension adds a new value of type t to the payload array.
// Currently, it is an error to have a mix of types, though this design could be changed if needed.
func (h *headPayloadFormat) addDimension(t reflect.Kind) error {
	if h.dtype == reflect.Invalid || h.dtype == t {
		h.dtype = t
		h.nvals++
		return nil
	}
	return fmt.Errorf("Cannot use type %v in header already of type %v", t, h.dtype)
}

// headPayloadShape describes the multi-dimensional shape of the payload
type headPayloadShape struct {
	Sizes []int16
}

// readTLV reads data for size bytes, generating a list of all TLV objects
func readTLV(data io.Reader, size int) (result []interface{}, err error) {
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
		if 8*int(tlvsize) > size {
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
				case 0:
					// ignore null characters
				case '!', '>':
					pfmt.bigendian = true
				case '<':
					pfmt.bigendian = false
				case 'h':
					err = pfmt.addDimension(reflect.Int16)
				case 'H':
					err = pfmt.addDimension(reflect.Uint16)
				case 'i', 'l':
					err = pfmt.addDimension(reflect.Int32)
				case 'I', 'L':
					err = pfmt.addDimension(reflect.Uint32)
				case 'q':
					err = pfmt.addDimension(reflect.Int64)
				case 'Q':
					err = pfmt.addDimension(reflect.Uint64)
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

		size -= 8 * int(tlvsize)
	}
	return
}
