// These functions work in Go versions up through 1.16, which lacks unsafe.Slice.
// For go 1.17 see publish_data_slices.go (only one of the two will be built).

//go:build !go1.17
// +build !go1.17

package dastard

import (
	"reflect"
	"unsafe"
)

// rawTypeToBytes convert a []RawType to []byte using unsafe
// see https://stackoverflow.com/questions/11924196/convert-between-slices-of-different-types
func rawTypeToBytes(d []RawType) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 2 // byte takes up half the space of RawType
	header.Len *= 2
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
}

// rawTypeToUint16convert a []RawType to []uint16 using unsafe
func rawTypeToUint16(d []RawType) []uint16 {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	data := *(*[]uint16)(unsafe.Pointer(&header))
	return data
}

func bytesToRawType(b []byte) []RawType {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&b))
	const ratio = int(unsafe.Sizeof(RawType(0)))
	header.Cap /= ratio // byte takes up twice the space of RawType
	header.Len /= ratio
	data := *(*[]RawType)(unsafe.Pointer(&header))
	return data
}
