package main

import (
	"github.com/usnistgov/dastard"
)

func main() {
	messageChan := make(chan dastard.ClientUpdate)
	go dastard.RunClientUpdater(messageChan, dastard.PortStatus)
	dastard.RunRPCServer(messageChan, dastard.PortRPC)
}
