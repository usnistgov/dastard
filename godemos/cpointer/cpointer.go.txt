package main

//#cgo LDFLAGS: -L. -lcptr
//#include <stdio.h>
//#include <stdlib.h>
//#include <string.h>
//char* echo(char* s);
//char* echoAlign(char* s);
import "C"
import (
    "fmt"
    "unsafe"
)

// See http://bit.ly/2fvGWZc at Stack Overflow for this code.

func main() {
    cs := C.CString("Hello from stdio\n")
    defer C.free(unsafe.Pointer(cs))

    var echoOut *C.char = C.echo(cs)
    defer C.free(unsafe.Pointer(echoOut))
    fmt.Println(C.GoString(echoOut))
    address := uintptr(unsafe.Pointer(echoOut))
    fmt.Printf("echoOut ptr = %v (lower 16 bits: 0x%04x)\n", echoOut, address&0xffff)

    var echoOut2 *C.char = C.echoAlign(cs)
    defer C.free(unsafe.Pointer(echoOut2))
    fmt.Println(C.GoString(echoOut2))
    address = uintptr(unsafe.Pointer(echoOut2))
    fmt.Printf("echoOut2 ptr = %v (lower 16 bits: 0x%04x)\n", echoOut2, address&0xffff)
}
