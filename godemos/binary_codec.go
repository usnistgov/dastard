package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Show how to convert between byte slices and uint32 values, by the
// encoding/binary package.
func main() {
	var pattern uint32 = 0xdeadbeef
	buf1 := make([]byte, 4)
	fmt.Printf("Pattern = %x\n", pattern)

	binary.LittleEndian.PutUint32(buf1, pattern)
	fmt.Printf("Bytes = %v or %x\n", buf1, buf1)

	decoded := binary.LittleEndian.Uint32(buf1)
	fmt.Printf("Decoded = %x\n", decoded)

	// That was for 1 word at a time. For a stream,...
	var pi float64
	b := []byte{0x18, 0x2d, 0x44, 0x54, 0xfb, 0x21, 0x09, 0x40}
	buf := bytes.NewReader(b)
	err := binary.Read(buf, binary.LittleEndian, &pi)
	if err != nil {
		fmt.Println("binary.Read failed:", err)
	}
	fmt.Println(b)
	fmt.Println(pi)

}

// See also func Read(r io.Reader, order ByteOrder, data interface{}) error
// and matching func Write(...). Helpful blog at
// https://medium.com/go-walkthrough/go-walkthrough-encoding-binary-96dc5d4abb5d
