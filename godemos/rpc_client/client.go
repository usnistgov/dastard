package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc/jsonrpc"
)

func main() {
	serverAddress := "localhost"

	// One command to dial AND set up jsonrpc client:
	//client, err := jsonrpc.Dial("tcp", serverAddress+":4234")

	// Or dial first and then set up the jsonrpc client.
	httpclient, err := net.Dial("tcp", serverAddress+":4234")
	if err != nil {
		log.Fatal("dialing:", err)
	}
	client := jsonrpc.NewClient(httpclient)

	// Synchronous call
	args := &Args{9, 107}
	var reply int
	err = client.Call("Arith.Multiply", args, &reply)
	if err != nil {
		log.Fatal("arith error:", err)
	}
	fmt.Printf("Arith: %d*%d=%d\n", args.A, args.B, reply)
}
