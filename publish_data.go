package dastard

import (
	// "encoding/binary"
	"fmt"

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

	for {
		select {
		case <-abort:
			return
		case records := <-dataToPub:
			for _, rec := range records {
				pubSocket.Write(packet(rec))
			}
		}
	}
}

func packet(rec *DataRecord) []byte {
	return []byte(fmt.Sprintf("Chan %d packet of size %d", rec.channum, len(rec.data)))
}
