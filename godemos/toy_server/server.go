package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"time"

	czmq "github.com/zeromq/goczmq"
)

// Sniff packets thus:
// sudo tcpdump -nnvXSs 0 -i lo0 port 4234

func handler(rw http.ResponseWriter, req *http.Request) {
	fmt.Println("Handler h was called with request: ")
	fmt.Println(req.URL)
	response := fmt.Sprintf("1")
	fmt.Println("Response: ", response)
	fmt.Fprintf(rw, response)
}

type dataProducer struct {
	channum int
	fact    int
	msgOut  chan<- string
	abort   <-chan struct{}
}

func (s *dataProducer) run() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	fmt.Printf("   Starting producer %d\n", s.channum)
	for {
		select {
		case <-s.abort:
			fmt.Printf("   Stopping producer %d\n", s.channum)
			return
		case <-ticker.C:
			msg := fmt.Sprintf("data chan %d: fact is %d, time is %v", s.channum,
				s.fact, time.Now().Format("15:04:05"))
			s.msgOut <- msg
		}
	}
}

// DataChannels holds a number of dataProducer objects.
type DataChannels struct {
	producers []*dataProducer
	stop      chan struct{}
	running   bool
}

func (dc *DataChannels) addProducer(channum int, msgOut chan string) {
	p := &dataProducer{channum: channum, msgOut: msgOut, abort: dc.stop}
	dc.producers = append(dc.producers, p)
}

// NewFact is for blah blah.
type NewFact struct {
	channum int
	fact    int
}

// UpdateFact changes the fact for one channel
func (dc *DataChannels) UpdateFact(newFact *NewFact, reply *bool) error {
	p := dc.producers[newFact.channum]
	old := p.fact
	p.fact = newFact.fact
	fmt.Printf("Setting channel %d 'fact' from %d to %d\n", p.channum, old, p.fact)
	msg := fmt.Sprintf("data chan %d: fact set to %d", p.channum, p.fact)
	p.msgOut <- msg
	*reply = true
	return nil
}

// Start starts all the producers
func (dc *DataChannels) Start(_ *struct{}, reply *bool) error {
	fmt.Println("RPC server asked to start the data channels")
	if !dc.running {
		for _, p := range dc.producers {
			go p.run()
		}
	}
	dc.running = true
	*reply = true
	return nil
}

// Stop stops all the producers
func (dc *DataChannels) Stop(_ *struct{}, reply *bool) error {
	fmt.Println("RPC server asked to stop the data channels")
	if dc.running {
		for _ = range dc.producers {
			dc.stop <- struct{}{}
		}
	}
	dc.running = false
	*reply = true
	return nil
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
	const PORT int = 4444
	const useRPC bool = true
	const nchan int = 4

	// Make producers
	rpcport := fmt.Sprintf(":%d", PORT)
	pubhost := fmt.Sprintf("tcp://*:%d", PORT+1)
	pub, err := czmq.NewPub(pubhost)
	if err != nil {
		log.Fatal(err)
	}
	defer pub.Destroy()

	toPublish := make(chan string)
	abort := make(chan struct{})

	producers := DataChannels{stop: abort}
	for i := 0; i < nchan; i++ {
		producers.addProducer(i, toPublish)
	}

	// Publish any messages that come from the dataProducers.
	go publisher(pub, toPublish, abort)
	defer func() { close(abort) }()

	if useRPC {
		server := rpc.NewServer()
		server.Register(&producers)
		server.HandleHTTP(rpc.DefaultRPCPath, rpc.DefaultDebugPath)
		listener, e := net.Listen("tcp", rpcport)
		if e != nil {
			log.Fatal("listen error:", e)
		}
		for {
			if conn, err := listener.Accept(); err != nil {
				log.Fatal("accept error: " + err.Error())
			} else {
				log.Printf("new connection established\n")
				go server.ServeCodec(jsonrpc.NewServerCodec(conn))
			}
		}
	} else {
		http.HandleFunc("/", handler)
		log.Fatal(http.ListenAndServe(rpcport, nil))

	}
}
