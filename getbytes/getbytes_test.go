package getbytes

import (
	"encoding/hex"
	"testing"
)

func TestFromGetBytes(t *testing.T) {
	var byteslicetests = []struct {
		byteslice []byte
		expect    string
	}{
		{FromSliceUint8([]uint8{0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89}), "abcdef0123456789"},
		{FromSliceUint16([]uint16{0xABCD, 0xEF01, 0x2345, 0x6789}), "cdab01ef45238967"},
		{FromSliceUint32([]uint32{0xABCDEF01, 0x23456789}), "01efcdab89674523"},
		{FromSliceUint64([]uint64{0xABCDEF0123456789}), "8967452301efcdab"},
		{FromSliceInt8([]int8{0x00, 0x0A, 0x0B, 0x0C, 0x0D, 0x0F, 0x01, 0x02}), "000a0b0c0d0f0102"},
		{FromSliceInt16([]int16{1, 2, 3, 4}), "0100020003000400"},
		{FromSliceInt32([]int32{1, 2}), "0100000002000000"},
		{FromSliceInt64([]int64{1}), "0100000000000000"},
		{FromSliceFloat32([]float32{1, 2}), "0000803f00000040"},
		{FromSliceFloat64([]float64{2, 4}), "00000000000000400000000000001040"},
		{FromSliceUint8([]uint8{}), ""},
		{FromSliceUint16([]uint16{}), ""},
		{FromSliceUint32([]uint32{}), ""},
		{FromSliceUint64([]uint64{}), ""},
		{FromSliceInt8([]int8{}), ""},
		{FromSliceInt16([]int16{}), ""},
		{FromSliceInt32([]int32{}), ""},
		{FromSliceInt64([]int64{}), ""},
		{FromSliceFloat32([]float32{}), ""},
		{FromSliceFloat64([]float64{}), ""},
	}
	for _, test := range byteslicetests {
		encodedStr := hex.EncodeToString(test.byteslice)
		if expectStr := test.expect; encodedStr != expectStr {
			t.Errorf("want %v, have %v", expectStr, encodedStr)
		}
	}

	var sizetests = []struct {
		dlen int
		want int
	}{
		{len(FromUint8(1)), 1},
		{len(FromUint16(1)), 2},
		{len(FromUint32(1)), 4},
		{len(FromUint64(1)), 8},
		{len(FromInt8(1)), 1},
		{len(FromInt16(1)), 2},
		{len(FromInt32(1)), 4},
		{len(FromInt64(1)), 8},
		{len(FromFloat32(1)), 4},
		{len(FromFloat64(1)), 8},
	}
	for _, test := range sizetests {
		if test.dlen != test.want {
			t.Errorf("wrong length: %d, want %d", test.dlen, test.want)
		}
	}
}
