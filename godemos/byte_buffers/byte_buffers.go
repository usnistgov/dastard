package main

import "fmt"

func main() {
	// var b1 = [64]byte
	b2 := make([]byte, 64)
	// fmt.Println(&b1)
	p2 := &b2
	fmt.Printf("%v\n", p2)

	for i, _ := range b2 {
		b2[i] = byte(i)
	}

	var bb [][]byte
	bb = append(bb, b2[50:])
	bb = append(bb, b2[:10])
	for i, val := range bb[0] {
		fmt.Println("i=", i, " value = ", val)
	}
	for i, val := range bb[1] {
		fmt.Println("i=", i, " value = ", val)
	}
}
