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
	httpclient, err := net.Dial("tcp", serverAddress+":5500")
	if err != nil {
		log.Fatal("dialing:", err)
	}
	client := jsonrpc.NewClient(httpclient)

	// Synchronous call
	args := &Args{153, 13}
	var reply int
	for b := 10; b < 20; b++ {
		args.B = b
		err = client.Call("Arith.Multiply", args, &reply)
		if err != nil {
			log.Print("Reply: ", reply)
			log.Fatal("arith error 1:", err)
		}
		fmt.Printf("Arith: %d * %d = %d\n", args.A, args.B, reply)
	}
	var replyQ Quotient
	err = client.Call("Arith.Divide", args, &replyQ)
	if err != nil {
		log.Fatal("arith error 2:", err)
	}
	fmt.Printf("Arith: %d / %d = %d (rem %d)\n", args.A, args.B, replyQ.Quo, replyQ.Rem)
}
