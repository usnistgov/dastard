package dastard

// Contain the ClientUpdater object, which publishes JSON-encoded messages
// giving the latest DASTARD state.

import (
	"fmt"

	czmq "github.com/zeromq/goczmq"
)

// ClientUpdate carries the messages to be published on the status port.
type ClientUpdate struct {
	tag     string
	message []byte
}

// RunClientUpdater forwards any message from its input channel to the ZMQ publisher socket
// to publish any information that clients need to know.
func RunClientUpdater(messages <-chan ClientUpdate, portstatus int) {
	hostname := fmt.Sprintf("tcp://*:%d", portstatus)
	pubSocket, err := czmq.NewPub(hostname)
	if err != nil {
		return
	}
	defer pubSocket.Destroy()

	for {
		select {
		case update := <-messages:
			pubSocket.SendFrame([]byte(update.tag), czmq.FlagMore)
			pubSocket.SendFrame(update.message, czmq.FlagNone)
		}
	}

}
