package getbytes

import (
	"encoding/hex"
	"testing"
)

func TestFromGetBytes(t *testing.T) {
	encodedStr := hex.EncodeToString(FromSliceUint8([]uint8{0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89}))
	if expectStr := "abcdef0123456789"; encodedStr != expectStr {
		t.Errorf("want %v, have %v", expectStr, encodedStr)
	}
	encodedStr = hex.EncodeToString(FromSliceUint16([]uint16{0xABCD, 0xEF01, 0x2345, 0x6789}))
	if expectStr := "cdab01ef45238967"; encodedStr != expectStr {
		t.Errorf("want %v, have %v", expectStr, encodedStr)
	}
	encodedStr = hex.EncodeToString(FromSliceUint32([]uint32{0xABCDEF01, 0x23456789}))
	if expectStr := "01efcdab89674523"; encodedStr != expectStr {
		t.Errorf("want %v, have %v", expectStr, encodedStr)
	}
	encodedStr = hex.EncodeToString(FromSliceUint64([]uint64{0xABCDEF0123456789}))
	if expectStr := "8967452301efcdab"; encodedStr != expectStr {
		t.Errorf("want %v, have %v", expectStr, encodedStr)
	}
	encodedStr = hex.EncodeToString(FromSliceInt8([]int8{0x00, 0x0A, 0x0B, 0x0C, 0x0D, 0x0F, 0x01, 0x02}))
	if expectStr := "000a0b0c0d0f0102"; encodedStr != expectStr {
		t.Errorf("want %v, have %v", expectStr, encodedStr)
	}
	encodedStr = hex.EncodeToString(FromSliceInt16([]int16{1, 2, 3, 4}))
	if expectStr := "0100020003000400"; encodedStr != expectStr {
		t.Errorf("want %v, have %v", expectStr, encodedStr)
	}
	encodedStr = hex.EncodeToString(FromSliceInt32([]int32{1, 2}))
	if expectStr := "0100000002000000"; encodedStr != expectStr {
		t.Errorf("want %v, have %v", expectStr, encodedStr)
	}
	encodedStr = hex.EncodeToString(FromSliceInt64([]int64{1}))
	if expectStr := "0100000000000000"; encodedStr != expectStr {
		t.Errorf("want %v, have %v", expectStr, encodedStr)
	}
	encodedStr = hex.EncodeToString(FromSliceFloat32([]float32{1, 2}))
	if expectStr := "0000803f00000040"; encodedStr != expectStr {
		t.Errorf("want %v, have %v", expectStr, encodedStr)
	}
	encodedStr = hex.EncodeToString(FromSliceFloat64([]float64{2}))
	if expectStr := "0000000000000040"; encodedStr != expectStr {
		t.Errorf("want %v, have %v", expectStr, encodedStr)
	}
	if len(FromUint8(1)) != 1 {
		t.Error("wrong length")
	}
	if len(FromUint16(1)) != 2 {
		t.Error("wrong length")
	}
	if len(FromUint32(1)) != 4 {
		t.Error("wrong length")
	}
	if len(FromUint64(1)) != 8 {
		t.Error("wrong length")
	}
	if len(FromInt8(1)) != 1 {
		t.Error("wrong length")
	}
	if len(FromInt16(1)) != 2 {
		t.Error("wrong length")
	}
	if len(FromInt32(1)) != 4 {
		t.Error("wrong length")
	}
	if len(FromInt64(1)) != 8 {
		t.Error("wrong length")
	}
	if len(FromFloat32(1)) != 4 {
		t.Error("wrong length")
	}
	if len(FromFloat64(1)) != 8 {
		t.Error("wrong length")
	}
}
