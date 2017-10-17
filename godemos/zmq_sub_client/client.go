// ZMQ SUBscriber (client) demonstration.
// Listens for PUBlished messages by SUBscribing to localhost:8000
// Filters messages so that only those starting with "<" are received.

package main

import (
	"fmt"
	"log"

	czmq "github.com/zeromq/goczmq"
)

func main() {
	pubURL := "tcp://localhost:32002"
	filter := ""
	sub, err := czmq.NewSub(pubURL, "")
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Destroy()

	sub.Connect(pubURL)
	sub.SetSubscribe(filter)
	i := 0
	for {

		msg, _, err := sub.RecvFrame()
		if err == nil {
			fmt.Printf("message %4d: %s\n", i, msg)
		} else {
			fmt.Printf("Error: %v\n", err)
			break
		}
		i++
	}
}
