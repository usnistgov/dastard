// These functions use unsafe.Slice, which is available only from Go version 1.17+.
// Dastard version 0.2.16 showed how to use conditional compilation to handle that.

package getbytes

import (
	"unsafe"
)

// FromSliceUint8 convert a []uint8 to []byte using unsafe
func FromSliceUint8(d []uint8) []byte {
	if len(d) == 0 {
		return []byte{}
	}
	outlength := uintptr(len(d)) * unsafe.Sizeof(d[0]) / unsafe.Sizeof(byte(0))
	return unsafe.Slice((*byte)(unsafe.Pointer(&d[0])), outlength)
}

// FromSliceUint16 convert a []uint16 to []byte using unsafe
func FromSliceUint16(d []uint16) []byte {
	if len(d) == 0 {
		return []byte{}
	}
	outlength := uintptr(len(d)) * unsafe.Sizeof(d[0]) / unsafe.Sizeof(byte(0))
	return unsafe.Slice((*byte)(unsafe.Pointer(&d[0])), outlength)
}

// FromSliceUint32 convert a []uint32 to []byte using unsafe
func FromSliceUint32(d []uint32) []byte {
	if len(d) == 0 {
		return []byte{}
	}
	outlength := uintptr(len(d)) * unsafe.Sizeof(d[0]) / unsafe.Sizeof(byte(0))
	return unsafe.Slice((*byte)(unsafe.Pointer(&d[0])), outlength)
}

// FromSliceUint64 convert a []uint64 to []byte using unsafe
func FromSliceUint64(d []uint64) []byte {
	if len(d) == 0 {
		return []byte{}
	}
	outlength := uintptr(len(d)) * unsafe.Sizeof(d[0]) / unsafe.Sizeof(byte(0))
	return unsafe.Slice((*byte)(unsafe.Pointer(&d[0])), outlength)
}

// FromSliceInt8 convert a []int8 to []byte using unsafe
func FromSliceInt8(d []int8) []byte {
	if len(d) == 0 {
		return []byte{}
	}
	outlength := uintptr(len(d)) * unsafe.Sizeof(d[0]) / unsafe.Sizeof(byte(0))
	return unsafe.Slice((*byte)(unsafe.Pointer(&d[0])), outlength)
}

// FromSliceInt16 convert a []int16 to []byte using unsafe
func FromSliceInt16(d []int16) []byte {
	if len(d) == 0 {
		return []byte{}
	}
	outlength := uintptr(len(d)) * unsafe.Sizeof(d[0]) / unsafe.Sizeof(byte(0))
	return unsafe.Slice((*byte)(unsafe.Pointer(&d[0])), outlength)
}

// FromSliceInt32 convert a []int32 to []byte using unsafe
func FromSliceInt32(d []int32) []byte {
	if len(d) == 0 {
		return []byte{}
	}
	outlength := uintptr(len(d)) * unsafe.Sizeof(d[0]) / unsafe.Sizeof(byte(0))
	return unsafe.Slice((*byte)(unsafe.Pointer(&d[0])), outlength)
}

// FromSliceInt64 convert a []int64 to []byte using unsafe
func FromSliceInt64(d []int64) []byte {
	if len(d) == 0 {
		return []byte{}
	}
	outlength := uintptr(len(d)) * unsafe.Sizeof(d[0]) / unsafe.Sizeof(byte(0))
	return unsafe.Slice((*byte)(unsafe.Pointer(&d[0])), outlength)
}

// FromSliceFloat32 convert a []float32 to []byte using unsafe
func FromSliceFloat32(d []float32) []byte {
	if len(d) == 0 {
		return []byte{}
	}
	outlength := uintptr(len(d)) * unsafe.Sizeof(d[0]) / unsafe.Sizeof(byte(0))
	return unsafe.Slice((*byte)(unsafe.Pointer(&d[0])), outlength)
}

// FromSliceFloat64 convert a []float64 to []byte using unsafe
func FromSliceFloat64(d []float64) []byte {
	if len(d) == 0 {
		return []byte{}
	}
	outlength := uintptr(len(d)) * unsafe.Sizeof(d[0]) / unsafe.Sizeof(byte(0))
	return unsafe.Slice((*byte)(unsafe.Pointer(&d[0])), outlength)
}

// FromUint8 converts a uint8 to []byte using unsafe
func FromUint8(d uint8) []byte {
	return FromSliceUint8([]uint8{d})
}

// FromUint16 converts a uint16 to []byte using unsafe
func FromUint16(d uint16) []byte {
	return FromSliceUint16([]uint16{d})
}

// FromUint32 converts a uint32 to []byte using unsafe
func FromUint32(d uint32) []byte {
	return FromSliceUint32([]uint32{d})
}

// FromUint64 converts a uint64 to []byte using unsafe
func FromUint64(d uint64) []byte {
	return FromSliceUint64([]uint64{d})
}

// FromInt8 converts a uint8 to []byte using unsafe
func FromInt8(d int8) []byte {
	return FromSliceInt8([]int8{d})
}

// FromInt16 converts a int16 to []byte using unsafe
func FromInt16(d int16) []byte {
	return FromSliceInt16([]int16{d})
}

// FromInt32 converts a int32 to []byte using unsafe
func FromInt32(d int32) []byte {
	return FromSliceInt32([]int32{d})
}

// FromInt64 converts a int64 to []byte using unsafe
func FromInt64(d int64) []byte {
	return FromSliceInt64([]int64{d})
}

// FromFloat32 converts a float32 to []byte using unsafe
func FromFloat32(d float32) []byte {
	return FromSliceFloat32([]float32{d})
}

// FromFloat64 converts a float65 to []byte using unsafe
func FromFloat64(d float64) []byte {
	return FromSliceFloat64([]float64{d})
}
