package dastard

import (
	// "encoding/binary"
	"fmt"
	"io"

	czmq "github.com/zeromq/goczmq"
)

// PublishRecords publishes one data packet per DataRecord received on its input to
// a ZMQ PUB socket. It terminates when abort channel is closed.
func PublishRecords(dataToPub <-chan []*DataRecord, abort <-chan struct{}, portnum int) {
	hostname := fmt.Sprintf("tcp://*:%d", portnum)
	pubSocket, err := czmq.NewPub(hostname)
	if err != nil {
		return
	}
	defer pubSocket.Destroy()

	// inputname := fmt.Sprintf("inproc://records2publish")
	// inputSocket, err := czmq.NewPullChanneler(inputname)
	// defer inputSocket.Destroy()

	for {
		select {
		case <-abort:
			return
		case records := <-dataToPub:
			for _, rec := range records {
				io.WriteString(pubSocket, packet(rec))
			}
		}
	}
}

func packet(rec *DataRecord) string {
	return fmt.Sprintf("Packet of size %d", len(rec.data))
}
