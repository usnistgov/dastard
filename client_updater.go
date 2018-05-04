package dastard

// Contain the ClientUpdater object, which publishes JSON-encoded messages
// giving the latest DASTARD state.

import (
	"fmt"
	"time"

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

	// Save the state to the standard saved-state file this often.
	savePeriod := time.Second
	saveStateTicker := time.NewTicker(savePeriod)
	defer saveStateTicker.Stop()

	// Here, store the last message of each type seen. Use when storing state.
	lastMessages := make(map[string]string)

	for {
		select {
		case update := <-messages:
			lastMessages[update.tag] = string(update.message)
			pubSocket.SendFrame([]byte(update.tag), czmq.FlagMore)
			pubSocket.SendFrame(update.message, czmq.FlagNone)
		case <-saveStateTicker.C:
			fmt.Printf("Save state: %v\n", lastMessages)
		}
	}

}
