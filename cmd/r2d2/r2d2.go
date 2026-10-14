// r2d2: The RFSoC-to-DASTARD tube
//
// Purpose: to translate RFSoC or Abaco packets from "monitor" form to the "pktdata" form that DASTARD requires.
//
// usage: r2d2 10.0.15.100:4001 localhost:4001
//
// Leave that running, and have Dastard connect to a resonator data source at localhost:4001, rather than at
// the actual firmware IP address (10.0.15.100 in this example)

package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"runtime"

	"github.com/usnistgov/dastard/packets"
	"golang.org/x/net/ipv4"
)

func main() {
	// Lock this goroutine to the current OS thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// 1. Setup the listener
	remotehost := "10.0.15.100"
	port := 4001
	sourceaddress := fmt.Sprintf("%s:%d", remotehost, port)
	listenAddr, err := net.ResolveUDPAddr("udp", sourceaddress)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	// Set buffers to 8MB (or higher, depending on your OS limits)
	conn.SetReadBuffer(8 * 1024 * 1024)
	conn.SetWriteBuffer(8 * 1024 * 1024)

	// Upgrade to an IPv4 packet connection
	pc := ipv4.NewPacketConn(conn)

	// 2. Setup the target destination
	target := fmt.Sprintf("localhost:%d", port)
	// targetAddr, err := net.ResolveUDPAddr("udp", target)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// 3. Pre-allocate multiple read and write buffers (typically 1500 bytes to fit standard MTU)
	const batchSize = 32
	readMsgs := make([]ipv4.Message, batchSize)
	writeMsgs := make([]ipv4.Message, batchSize)

	for i := range readMsgs {
		// Crucial: Each message MUST have its own independent backing array
		readMsgs[i].Buffers = [][]byte{make([]byte, 1500)}
		writeMsgs[i].Buffers = [][]byte{make([]byte, 1500)}
	}
	log.Printf("R2-D2 is listening on %s, forwarding to %s", sourceaddress, target)

	var payloadReader bytes.Reader
	for {
		// Read incoming packet
		n, err := pc.ReadBatch(readMsgs, 0)
		if err != nil {
			log.Println("Read error:", err)
			continue
		}

		// Read fraction of payload (e.g., first 12 bytes)
		// header := buffer[:12]

		// Your translation package should write directly into writeBuf
		// rather than returning a new []byte.
		for i := 0; i < n; i++ {
			// Extract the payload using the N field (bytes read)
			payloadLen := readMsgs[i].N
			incomingData := readMsgs[i].Buffers[0][:payloadLen]
			payloadReader.Reset(incomingData)

			packet, err := packets.ReadPacket(&payloadReader)
			if err != nil {
				log.Println("Packet generation error:", err)
			}

			fmt.Println(packet)

			// Run your translation. Assuming it returns the new length
			// written directly into the corresponding write buffer.
			// newLen := mypackage.Translate(incomingData, writeMsgs[i].Buffers[0])

			// Prepare the write message
			// writeMsgs[i].N = newLen
			// writeMsgs[i].Addr = targetAddr
		}

		// Write all processed packets in a single system call
		// We slice writeMsgs[:n] to only send the packets we just translated
		// _, err = pc.WriteBatch(writeMsgs[:n], 0)
		// if err != nil {
		// 	log.Println("Write error:", err)
		// }
	}
}
