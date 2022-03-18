// These functions work in Go versions up through 1.16, which lacks unsafe.Slice.
// For go 1.17 see publish_data_slices.go (only one of the two will be built).

//go:build !go1.17
// +build !go1.17

// Package getbytes converts things to []byte faster than binary.Write using unsafe
// see https://stackoverflow.com/questions/11924196/convert-between-slices-of-different-types?utm_medium=organic&utm_source=google_rich_qa&utm_campaign=google_rich_qa
package getbytes

import (
	"reflect"
	"unsafe"
)

// FromSliceUint8 convert a []uint8 to []byte using unsafe
func FromSliceUint8(d []uint8) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 1
	header.Len *= 1
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
}

// FromSliceUint16 convert a []uint16 to []byte using unsafe
func FromSliceUint16(d []uint16) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 2
	header.Len *= 2
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
}

// FromSliceUint32 convert a []uint32 to []byte using unsafe
func FromSliceUint32(d []uint32) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 4
	header.Len *= 4
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
}

// FromSliceUint64 convert a []uint64 to []byte using unsafe
func FromSliceUint64(d []uint64) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 8
	header.Len *= 8
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
}

// FromSliceInt8 convert a []int8 to []byte using unsafe
func FromSliceInt8(d []int8) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 1
	header.Len *= 1
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
}

// FromSliceInt16 convert a []int16 to []byte using unsafe
func FromSliceInt16(d []int16) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 2
	header.Len *= 2
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
}

// FromSliceInt32 convert a []int32 to []byte using unsafe
func FromSliceInt32(d []int32) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 4
	header.Len *= 4
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
}

// FromSliceInt64 convert a []int64 to []byte using unsafe
func FromSliceInt64(d []int64) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 8
	header.Len *= 8
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
}

// FromSliceFloat32 convert a []float32 to []byte using unsafe
func FromSliceFloat32(d []float32) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 4
	header.Len *= 4
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
}

// FromSliceFloat64 convert a []float64 to []byte using unsafe
func FromSliceFloat64(d []float64) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 8
	header.Len *= 8
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
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
