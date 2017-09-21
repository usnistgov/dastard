// A toy version of dastard: has a REQ-REP server and two PUB sockets.

package main

import (
	"fmt"
	"io"
	"log"
	"time"

	czmq "github.com/zeromq/goczmq"
)

type dataProducer struct {
	channum int
	fact    int
	msgOut  chan<- string
	abort   <-chan struct{}
}

func (s *dataProducer) run() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.abort:
			return
		case <-ticker.C:
			msg := fmt.Sprintf("data chan %d: fact is %d, time is %v", s.channum,
				s.fact, time.Now().Format("15:04:05"))
			s.msgOut <- msg
		}
	}
}

func publisher(pub *czmq.Sock, outChan <-chan string, abort <-chan struct{}) {
	for {
		select {
		case <-abort:
			return
		case msg := <-outChan:
			_, err := io.WriteString(pub, msg)
			if err != nil {
				return
			}
		}
	}
}

func main() {
	nchan := 4

	// Make servers
	pub, err := czmq.NewPub("tcp://*:8000")
	if err != nil {
		log.Fatal(err)
	}

	commands, err := czmq.NewRep("tcp://*:8001")
	if err != nil {
		log.Fatal(err)
	}

	toPublish := make(chan string)
	abort := make(chan struct{})

	var servers []dataProducer
	for i := 0; i < nchan; i++ {
		s := dataProducer{channum: i, msgOut: toPublish, abort: abort}
		servers = append(servers, s)
		go s.run()
	}

	// Publish any messages that come from the dataProducers.
	go publisher(pub, toPublish, abort)
	defer func() { close(abort) }()

	// Now serve commands until a QUIT arrives
	for {
		// messages, j, err := commands.RecvFrame()
		frames, err := commands.RecvMessage()
		if err != nil {
			fmt.Printf("Command server Recv error: %s\n", err)
			return
		}
		fmt.Printf("Message received with %d frames:\n", len(frames))
		for i, frame := range frames {
			fmt.Printf(" frame %d: %v\n", i, string(frame))
		}

		frame := []byte("this is my response")
		err = commands.SendFrame(frame, czmq.FlagNone)
		if err != nil {
			fmt.Printf("Command server Send error: %s\n", err)
			return
		}
	}
}
