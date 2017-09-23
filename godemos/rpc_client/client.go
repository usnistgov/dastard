package main

import (
	"fmt"
	"log"
	"net/rpc"
)

func main() {
	serverAddress := "localhost"
	// client, err := rpc.DialHTTP("tcp", serverAddress+":4234")
	client, err := rpc.DialHTTPPath("tcp", serverAddress+":4234", "/gorpc")
	if err != nil {
		log.Fatal("dialing:", err)
	}
	// Synchronous call
	args := &Args{9, 17}
	var reply int
	err = client.Call("Arithmetic.Multiply", args, &reply)
	if err != nil {
		log.Fatal("arith error:", err)
	}
	fmt.Printf("Arith: %d*%d=%d\n", args.A, args.B, reply)
}
