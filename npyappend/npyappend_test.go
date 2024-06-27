package npyappend_test

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/usnistgov/dastard/npyappend"
)

func TestNpyAppenderFloat64(t *testing.T) {
	filename := "test_float64.npy"
	defer os.Remove(filename)

	appender, err := npyappend.NewNpyAppender[float64](filename)
	if err != nil {
		t.Fatalf("Failed to create NpyAppender: %v", err)
	}
	if appender.Tell() != 128 {
		t.Fatalf("wrong file length after writing header")
	}

	for i := 0; i < 10; i++ {
		if err := appender.Append(1.23); err != nil {
			t.Fatalf("Failed to append data: %v", err)
		}
	}
	if appender.Tell() != 128+80 {
		fmt.Println("fucking tell", appender.Tell())
		t.Fatalf("wrong file length after writing data")
	}

	if err := appender.RefreshHeader(); err != nil {
		t.Fatalf("Failed to refresh header: %v", err)
	}
	appender.Close()

	// Check header length
	headerLen := len(appender.Last_header)
	expectedLen := 128
	if headerLen != expectedLen {
		t.Fatalf("Expected header length %d, got %d", expectedLen, headerLen)
	}

	// // Check header content
	fileData, err := os.ReadFile(filename)
	fmt.Println(len(fileData))
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	headerData := fileData[:headerLen]
	read_header := string(headerData)
	if !strings.Contains(read_header, "{'descr': '<f8', 'fortran_order': False, 'shape': (10), }") {
		t.Fatalf("Header does not contain expected shape")
	}
	for i := 0; i < 10; i++ {
		itemdata := fileData[headerLen+i*8 : headerLen+(i+1)*8]
		a := binary.LittleEndian.Uint64(itemdata)
		a2 := math.Float64frombits(a)
		if a2 != 1.23 {
			fmt.Println(i)
			fmt.Println(itemdata)
			t.Fatalf("file data wrong")
		}
	}
}

func TestNpyAppenderFloat32(t *testing.T) {
	filename := "test_float32.npy"
	defer os.Remove(filename)

	// Create a new NpyAppender instance for float32 data
	appender, err := npyappend.NewNpyAppender[float32](filename)
	if err != nil {
		t.Fatalf("Failed to create NpyAppender: %v", err)
	}
	defer appender.Close()

	// Append float32 data to the .npy file
	for i := 0; i < 10; i++ {
		if err := appender.Append(float32(1.23)); err != nil {
			t.Fatalf("Failed to append data: %v", err)
		}
	}

	// Refresh the header to ensure it's updated with the latest data
	if err := appender.RefreshHeader(); err != nil {
		t.Fatalf("Failed to refresh header: %v", err)
	}

	// Check header length
	headerLen := len(appender.Last_header)
	expectedLen := 128
	if headerLen != expectedLen {
		t.Fatalf("Expected header length %d, got %d", expectedLen, headerLen)
	}

	// Read the entire file to check header content
	fileData, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Extract the header data based on the expected length
	headerData := fileData[:headerLen]
	readHeader := string(headerData)

	// Check if the expected shape information is present in the header
	expectedShape := "{'descr': '<f4', 'fortran_order': False, 'shape': (10), }"
	if !strings.Contains(readHeader, expectedShape) {
		t.Fatalf("Header does not contain expected shape: %s", expectedShape)
	}
	for i := 0; i < 10; i++ {
		itemdata := fileData[headerLen+i*4 : headerLen+(i+1)*4]
		a := binary.LittleEndian.Uint32(itemdata)
		a2 := math.Float32frombits(a)
		if a2 != 1.23 {
			fmt.Println(i)
			fmt.Println(itemdata)
			t.Fatalf("file data wrong")
		}
	}
}

func TestNpyAppender2DUint16(t *testing.T) {
	filename := "test2d_uint16.npy"
	defer os.Remove(filename)

	appender, err := npyappend.NewNpyAppender[[]uint16](filename)
	if err != nil {
		t.Fatalf("Failed to create NpyAppender: %v", err)
	}
	defer appender.Close()
	if appender.Tell() != 128 {
		t.Fatalf("wrong file length after writing header")
	}

	data := []uint16{1, 2, 3}
	appender.SetSliceLength(len(data))
	for i := 0; i < 10; i++ {
		if err := appender.Append(data); err != nil {
			t.Fatalf("Failed to append data: %v", err)
		}
	}
	if appender.Tell() != 128+3*10*2 {
		fmt.Println("fucking tell", appender.Tell())
		t.Fatalf("wrong file length after writing data")
	}

	if err := appender.RefreshHeader(); err != nil {
		t.Fatalf("Failed to refresh header: %v", err)
	}

	// Check header length
	headerLen := len(appender.Last_header)
	expectedLen := 128
	if headerLen != expectedLen {
		t.Fatalf("Expected header length %d, got %d", expectedLen, headerLen)
	}

	// Read the entire file to check header content
	fileData, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Extract the header data based on the expected length
	headerData := fileData[:headerLen]
	readHeader := string(headerData)

	// Check if the expected shape information is present in the header
	expectedShape := "{'descr': '<u2', 'fortran_order': False, 'shape': (10, 3), }"
	if !strings.Contains(readHeader, expectedShape) {
		t.Fatalf("Header does not contain expected shape: %s", expectedShape)
	}
}
