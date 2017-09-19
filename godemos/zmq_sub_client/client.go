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
	pubURL := "tcp://localhost:8000"
	filter := "<"
	sub, err := czmq.NewSub(pubURL, "subscribe to almost nothing, except this")
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Destroy()

	sub.Connect(pubURL)
	sub.SetSubscribe(filter)
	for {
		msg, _, err := sub.RecvFrame()
		if err == nil {
			fmt.Printf("message: %s", msg)
		} else {
			fmt.Printf("%v\n", err)
			break
		}

	}
}
