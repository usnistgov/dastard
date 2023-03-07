// These functions use unsafe.Slice, which is available only from Go version 1.17+.
// Dastard version 0.2.16 showed how to use conditional compilation to handle that.

package dastard

import (
	"unsafe"
)

// rawTypeToBytes convert a []RawType to []byte using unsafe.Slice
func rawTypeToBytes(slice_in []RawType) []byte {
	if len(slice_in) == 0 {
		return []byte{}
	}
	outlength := uintptr(len(slice_in)) * unsafe.Sizeof(slice_in[0]) / unsafe.Sizeof(byte(0))
	return unsafe.Slice((*byte)(unsafe.Pointer(&slice_in[0])), outlength)
}

// rawTypeToUint16convert a []RawType to []uint16 using unsafe
func rawTypeToUint16(slice_in []RawType) []uint16 {
	if len(slice_in) == 0 {
		return []uint16{}
	}
	outlength := uintptr(len(slice_in)) * unsafe.Sizeof(slice_in[0]) / unsafe.Sizeof(uint16(0))
	return unsafe.Slice((*uint16)(unsafe.Pointer(&slice_in[0])), outlength)
}

func bytesToRawType(slice_in []byte) []RawType {
	if len(slice_in) == 0 {
		return []RawType{}
	}
	outlength := uintptr(len(slice_in)) * unsafe.Sizeof(slice_in[0]) / unsafe.Sizeof(RawType(0))
	return unsafe.Slice((*RawType)(unsafe.Pointer(&slice_in[0])), outlength)
}
