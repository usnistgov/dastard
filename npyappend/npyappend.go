package npyappend

import (
	"encoding/binary"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/usnistgov/dastard/getbytes"
)

// NpyAppender is a struct that handles appending data to a .npy file.
type NpyAppender[T any] struct {
	filename    string
	file        *os.File
	Last_header string
	shape       []int
}

// NewNpyAppender creates a new NpyAppender instance.
func NewNpyAppender[T any](filename string) (*NpyAppender[T], error) {
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	var dummy T

	appender := &NpyAppender[T]{
		filename: filename,
		file:     file,
		shape:    shapeFrom(reflect.ValueOf(dummy)),
	}

	if err := appender.writeHeader(); err != nil {
		return nil, err
	}

	return appender, nil
}

// Append appends a new item of type T to the .npy file.
func (a *NpyAppender[T]) Append(item T) error {
	// binary.Write is slow!!!! need to not use and also buffer here?
	if err := binary.Write(a.file, binary.LittleEndian, item); err != nil {
		return err
	}
	// a.file.WriteString("12345678")
	a.shape[0] += 1
	return nil
}

func bytesFrom(item interface{}) []byte {
	originalf64, ok := item.(float64)
	if ok {
		return getbytes.FromFloat64(originalf64)
	}
	originalf32, ok := item.(float32)
	if ok {
		return getbytes.FromFloat32(originalf32)
	}
	originalsliceu16, ok := item.([]uint16)
	if ok {
		return getbytes.FromSliceUint16(originalsliceu16)
	}
	panic("cant handle type")

}

func (a *NpyAppender[T]) Append2(item T) error {
	b := bytesFrom(item)
	_, err := a.file.Write(b)
	if err != nil {
		panic("fuck error handling in go, serioulsy")
	}
	a.shape[0] += 1
	return nil
}

// writeHeader writes the numpy header to the file.
func (a *NpyAppender[T]) writeHeader() error {
	var dummy T
	header_len := 128
	magic_string := "\x93NUMPY"
	version_bytes := "\x01\x00"
	header := fmt.Sprintf("%s%s%02d{'descr': '%s', 'fortran_order': False, 'shape': (%s), }",
		magic_string, version_bytes,
		header_len-len(magic_string)-len(version_bytes)-2, // -2 is for the size of the two bytes used writing this value
		dtypeFrom(reflect.ValueOf(dummy), reflect.TypeOf(dummy)), strings.Trim(strings.Replace(fmt.Sprint(a.shape), " ", ", ", -1), "[N]"))
	padding := header_len - len(header) - 1 // ensuring header length is equal to header_len
	header = fmt.Sprintf("%s%s\n", header, strings.Repeat(" ", padding))

	a.Last_header = header // store header for refreshHeader method
	if _, err := a.file.WriteAt([]byte(header), 0); err != nil {
		return err
	}
	a.file.Seek(0, 2)
	return nil
}

// refreshHeader refreshes the numpy header.
func (a *NpyAppender[T]) RefreshHeader() error {
	if err := a.writeHeader(); err != nil {
		return err
	}
	return nil
}

// SetSliceLen set the expected length for slice items
func (a *NpyAppender[T]) SetSliceLength(length int) {
	if a.shape[0] > 0 {
		panic("can't set slice len after appending an item")
	}
	a.shape[1] = length
}

// Tell returns the file size in bytes
func (a *NpyAppender[T]) Tell() int64 {
	info, _ := a.file.Stat()
	return info.Size()
}

// Close closes the file, ensuring the header is written first.
func (a *NpyAppender[T]) Close() error {
	if err := a.writeHeader(); err != nil {
		return err
	}
	return a.file.Close()
}

func shapeFrom(rv reflect.Value) []int {
	rt := rv.Type()
	switch rt.Kind() {
	case reflect.Array:
		return []int{0, rv.Len()}
	case reflect.Slice:
		return []int{0, rv.Len()}
	default:
		return []int{0}
	}
}

func dtypeFrom(rv reflect.Value, rt reflect.Type) string {

	switch rt.Kind() {
	case reflect.Bool:
		return "|b1"
	case reflect.Uint8:
		return "|u1"
	case reflect.Uint16:
		return "<u2"
	case reflect.Uint32:
		return "<u4"
	case reflect.Uint, reflect.Uint64:
		return "<u8"
	case reflect.Int8:
		return "|i1"
	case reflect.Int16:
		return "<i2"
	case reflect.Int32:
		return "<i4"
	case reflect.Int, reflect.Int64:
		return "<i8"
	case reflect.Float32:
		return "<f4"
	case reflect.Float64:
		return "<f8"
	case reflect.Complex64:
		return "<c8"
	case reflect.Complex128:
		return "<c16"

	case reflect.Array:
		et := rt.Elem()
		switch et.Kind() {
		default:
			return dtypeFrom(reflect.Value{}, et)
		case reflect.String:
			slice := rv.Slice(0, rt.Len()).Interface().([]string)
			n := 0
			for _, str := range slice {
				if len(str) > n {
					n = len(str)
				}
			}
			return fmt.Sprintf("<U%d", n)
		}

	case reflect.Slice:
		rt = rt.Elem()
		switch rt.Kind() {
		default:
			return dtypeFrom(reflect.Value{}, rt)
		case reflect.String:
			slice := rv.Interface().([]string)
			n := 0
			for _, str := range slice {
				if len(str) > n {
					n = len(str)
				}
			}
			return fmt.Sprintf("<U%d", n)
		}
	default:
		panic("don't know how to do that type yo")
	}
}
