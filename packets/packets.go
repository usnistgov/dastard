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

// HeadTimestamp represents a single timestamp in the header
type HeadTimestamp uint64

// HeadCounter represents a counter found in a packet header
type HeadCounter struct {
	ID    int16
	Count int32
}

// HeadPayloadFormat represents the payload format header item.
type HeadPayloadFormat struct {
	bigendian bool
	rawfmt    string
	nvals     int
	dtype     reflect.Kind
}

// addDimension adds a new value of type t to the payload array.
// Currently, it is an error to have a mix of types, though this design could be changed if needed.
func (h *HeadPayloadFormat) addDimension(t reflect.Kind) error {
	if h.dtype == reflect.Invalid || h.dtype == t {
		h.dtype = t
		h.nvals++
		return nil
	}
	return fmt.Errorf("Cannot use type %v in header already of type %v", t, h.dtype)
}

// type HeadPayloadShape struct {
// 	Sizes []int16
// }

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
			tstamp := HeadTimestamp(x) << 32
			tstamp += HeadTimestamp(y)
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
			pfmt := new(HeadPayloadFormat)
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
		case 0x23: // Channel offset

		default:
			return result, fmt.Errorf("Unknown TLV type %d", t)
		}

		size -= 8 * int(tlvsize)
	}
	return
}
