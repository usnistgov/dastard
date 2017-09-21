// A toy version of dastard: has a REQ-REP server and two PUB sockets.

package main

import (
	"bufio"
	"fmt"
	"log"
	"os"

	czmq "github.com/zeromq/goczmq"
)

func main() {
	commands, err := czmq.NewReq("tcp://localhost:8001")
	if err != nil {
		log.Fatal(err)
	}
	defer commands.Destroy()

	consolescanner := bufio.NewScanner(os.Stdin)

	// by default, bufio.Scanner scans newline-separated lines
	for consolescanner.Scan() {
		input := consolescanner.Text()
		err := commands.SendFrame([]byte(input), czmq.FlagNone)
		if err != nil {
			log.Fatal(err)
		}
		frames, err := commands.RecvMessage()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Message received with %d frames:\n", len(frames))
		for i, frame := range frames {
			fmt.Printf(" frame %d: %v\n", i, string(frame))
		}
	}

	// check once at the end to see if any errors
	// were encountered (the Scan() method will
	// return false as soon as an error is encountered)
	if err := consolescanner.Err(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
