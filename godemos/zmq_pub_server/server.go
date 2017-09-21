// Based on gopl.io/ch8/clock1
// But uses ZMQ. Use "go get github.com/zeromq/goczmq"
// On Mac Ports machine, you'll need to install zmq, czmq, and libsodium

// Copyright © 2016 Alan A. A. Donovan & Brian W. Kernighan.
// License: https://creativecommons.org/licenses/by-nc-sa/4.0/

// See page 222.

// Clock is a TCP server that periodically writes the time.
package main

import (
	"fmt"
	"io"
	"log"
	"time"

	czmq "github.com/zeromq/goczmq"
)

func main() {
	pub, err := czmq.NewPub("tcp://*:8000")
	if err != nil {
		log.Fatal(err)
	}
	//!+
	for {
		msg := fmt.Sprint(time.Now().Format("<15:04:05>"))
		_, err := io.WriteString(pub, msg)
		if err != nil {
			return // e.g., client disconnected
		}
		msg = fmt.Sprint(time.Now().Format("(15:04:05)"))
		_, err = io.WriteString(pub, msg)
		if err != nil {
			return // e.g., client disconnected
		}
		time.Sleep(1 * time.Second)
	}
	//!-
}
