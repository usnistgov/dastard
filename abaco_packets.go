package dastard

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
	Count int32
}

// HeadPayloadFormat represents the payload format header item.
type HeadPayloadFormat struct {
	bigendian bool
	rawfmt    string
	ndim      int
	dtype     reflect.Kind
}

// addDimension adds a new value of type t to the payload array.
// Currently, it is an error to have a mix of types, though this design could be changed if needed.
func (h *HeadPayloadFormat) addDimension(t reflect.Kind) error {
	if h.dtype == reflect.Invalid || h.dtype == t {
		h.dtype = t
		h.ndim++
		return nil
	}
	return fmt.Errorf("Cannot use type %v to header already of type %v", t, h.dtype)
}

// type HeadPayloadShape struct {
// 	Sizes []int16
// }

// readTLV reads data for size bytes, generating a list of all TLV objects
func readTLV(data io.Reader, size int) (result []interface{}, err error) {
	var t uint8
	var tlvsize uint8
	for size >= 8 {
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
		if t == 0x12 { // Counter
			ctr := new(HeadCounter)
			if err = binary.Read(data, binary.BigEndian, &ctr.ID); err != nil {
				return result, err
			}
			if err = binary.Read(data, binary.BigEndian, &ctr.Count); err != nil {
				return result, err
			}
			fmt.Printf("New ctr! %v\n", ctr)
			result = append(result, ctr)

		} else if t == 0x21 { // Payload format descriptor
			b := make([]byte, 8*int(tlvsize)-2)
			if n, err := data.Read(b); err != nil || n < len(b) {
				return result, err
			}
			pfmt := new(HeadPayloadFormat)
			pfmt.rawfmt = string(b)
			for _, c := range pfmt.rawfmt {
				switch c {
				case '!', '>':
					pfmt.bigendian = true
				case '<':
					pfmt.bigendian = false
				case 'h':
					pfmt.addDimension(reflect.Int16)
				case 'H':
					pfmt.addDimension(reflect.Uint16)
				case 'i', 'l':
					pfmt.addDimension(reflect.Int32)
				case 'I', 'L':
					pfmt.addDimension(reflect.Uint32)
				case 'q':
					pfmt.addDimension(reflect.Int64)
				case 'Q':
					pfmt.addDimension(reflect.Uint64)
				default:
					return result, fmt.Errorf("Unknown data format character '%c' in format '%s'",
						c, pfmt.rawfmt)
				}
			}

		} else if t == 0x22 { // Payload shape descriptor

		}

		size -= 8 * int(tlvsize)
	}
	return
}
