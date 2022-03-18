// These functions use unsafe.Slice, which is available only from Go version 1.17+.
// For go -1.16 see publish_data_slices_oldversion.go (only one of the two will be built).

//go:build go1.17
// +build go1.17

package dastard

import (
	"unsafe"
)

// rawTypeToBytes convert a []RawType to []byte using unsafe.Slice
func rawTypeToBytes(slice_in []RawType) []byte {
	outlength := uintptr(len(slice_in)) * unsafe.Sizeof(slice_in[0]) / unsafe.Sizeof(byte(0))
	return unsafe.Slice((*byte)(unsafe.Pointer(&slice_in[0])), outlength)
}

// rawTypeToUint16convert a []RawType to []uint16 using unsafe
func rawTypeToUint16(slice_in []RawType) []uint16 {
	outlength := uintptr(len(slice_in)) * unsafe.Sizeof(slice_in[0]) / unsafe.Sizeof(uint16(0))
	return unsafe.Slice((*uint16)(unsafe.Pointer(&slice_in[0])), outlength)
}

func bytesToRawType(slice_in []byte) []RawType {
	outlength := uintptr(len(slice_in)) * unsafe.Sizeof(slice_in[0]) / unsafe.Sizeof(RawType(0))
	return unsafe.Slice((*RawType)(unsafe.Pointer(&slice_in[0])), outlength)
}

// bytesToInt32 converts a []byte slice to an []int32 slice, which is
// how we interpret the raw Abaco data.
func bytesToInt32(slice_in []byte) []int32 {
	outlength := uintptr(len(slice_in)) * unsafe.Sizeof(slice_in[0]) / unsafe.Sizeof(int32(0))
	return unsafe.Slice((*int32)(unsafe.Pointer(&slice_in[0])), outlength)
}
