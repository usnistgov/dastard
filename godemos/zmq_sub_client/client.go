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
	pubURL := "tcp://localhost:5501"
	filter := ""
	sub, err := czmq.NewSub(pubURL, "")
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Destroy()

	sub.Connect(pubURL)
	sub.SetSubscribe(filter)
	iframe := 0
	imsg := 0
	newmsg := true
	for {
		msg, more, err := sub.RecvFrame()
		if err == nil {
			if newmsg {
				fmt.Printf("message %d: ", imsg)
				iframe = 0
				imsg++
			}
			fmt.Printf("frame %4d: [%s]", iframe, msg)
			if more > 0 {
				fmt.Printf("...")
				newmsg = false
				iframe++
			} else {
				newmsg = true
				fmt.Println()
			}
		} else {
			fmt.Printf("Error: %v\n", err)
			break
		}
	}
}
