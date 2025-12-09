// Package appendablenpy provides classes that write in numpy's *.npy
// format, but extendable

package appendablenpy

import (
	"fmt"
	"os"
)

// npy file header must be a multiple of 64 bytes
const HEADER_UNITS = 64

type AppendableNPY struct {
	writer *os.File
	dtype_description string
	size_max_digits int
	shape_ptr int
	items_written int
}

func OpenAppendableNPY(fp *os.File, dtype string) (an *AppendableNPY) {
	an = new(AppendableNPY)
	an.writer = fp
	an.dtype_description = dtype
	an.size_max_digits = 10
	header := []byte{0x93,  0x4e,  0x55,  0x4d,  0x50,  0x59,  0x01,  0, 0, 0}
	header = append(header, []byte("{'descr': ")...)
	header = append(header, []byte(dtype)...)
	header = append(header, []byte(", 'fortran_order': False, 'shape': (0         ,),}")...)
	an.shape_ptr = len(header) - len([]byte("0         ,),}"))

	// Put header size into bytes 8-9, little-endian. It's a multiple of 64 bytes
	const PREHEADER_SIZE = 10
	nunits := (len(header) + HEADER_UNITS) / HEADER_UNITS
	header_size := nunits * HEADER_UNITS - PREHEADER_SIZE
	header[8] = byte(header_size % 256)
	header[9] = byte(header_size / 256)

	// Pad header with spaces plus one newline (0x20 and 0x0a, respectively) to the promised size
	npad := header_size + PREHEADER_SIZE - (1 + len(header))
	if npad > 0 {
		spaces := make([]byte, npad)
		for i := 0; i < npad; i++ {
			spaces[i] = 0x20
		}
		header = append(header, spaces...)
	}
	header = append(header, 0x0a)
	an.writer.Write(header)
	return an
}

func (an *AppendableNPY) Write(data [][]byte) error {
	// First count the data and write it
	n := len(data)
	for _, d := range data {
		if _, err := an.writer.Write(d); err != nil {
			return err
		}
	}

	// Now seek backwards to update the header then forward to the end of file
	an.items_written += n
	shape_text := fmt.Sprintf("%-10d", an.items_written)
	shape_bytes := []byte(shape_text)
	an.writer.Seek(int64(an.shape_ptr), 0)
	an.writer.Write(shape_bytes)
	an.writer.Seek(0, 2)
	return nil
}
