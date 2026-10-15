// r2d2: The RFSoC-to-DASTARD tube
//
// Purpose: to translate RFSoC or Abaco packets from "monitor" form to the "pktdata" form that DASTARD requires.
//
// usage: r2d2 10.0.15.100:4001
//
// Leave that running, and have Dastard connect to a resonator data source at localhost:4001, rather than at
// the actual firmware IP address (10.0.15.100 in this example).
// TODO: Let the translated source appear on localhost at a different port number than the original port.

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"

	"github.com/usnistgov/dastard/packets"
	"golang.org/x/net/ipv4"
)

func rewritePacket(p *packets.Packet) {

	Nframe := p.Frames()
	Nchan, _ := p.ChannelInfo()
	data, ok := p.Data.([]byte)
	if !ok {
		panic("couldn't cast p.Data to []byte")
	}

	const bitshift = 0
	newdata := make([]int16, 2*Nframe*Nchan)
	for i := range Nframe * Nchan {
		offset := i*16 + 8
		val1 := binary.LittleEndian.Uint32(data[offset : offset+4])
		val2 := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		atan0 := int16(int32(val1) >> bitshift)
		rot := int16(int32(val2) >> bitshift)
		newdata[i*2] = atan0
		newdata[i*2+1] = rot
	}
	dims := []int16{int16(2 * Nchan)}
	p.NewData(newdata, dims)

}

func main() {
	// Lock this goroutine to the current OS thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Enforce exactly one command-line argument (plus the program name itself)
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s <remote-ip:port>\nExample: %s 10.0.15.100:4001", os.Args[0], os.Args[0])
	}

	// Set up the listener
	sourceaddress := os.Args[1]
	listenAddr, err := net.ResolveUDPAddr("udp", sourceaddress)
	if err != nil {
		log.Fatal(err)
	}

	// Extract the port to mirror it for localhost
	_, portString, err := net.SplitHostPort(sourceaddress)
	if err != nil {
		log.Fatalf("Invalid address format. Expected host:port. Error: %v", err)
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
	packconn := ipv4.NewPacketConn(conn)

	// Set up the target destination
	target := fmt.Sprintf("localhost:%s", portString)
	targetAddr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("R2-D2 is listening on %s, forwarding to %s", sourceaddress, target)
	translate(packconn, targetAddr)
}

func translate(packconn *ipv4.PacketConn, targetAddr *net.UDPAddr) {

	const batchSize = 32
	readMsgs := make([]ipv4.Message, batchSize)
	writeMsgs := make([]ipv4.Message, batchSize)

	// Pre-allocate multiple read and write buffers
	for i := range readMsgs {
		// Crucial: Each message MUST have its own independent backing array
		readMsgs[i].Buffers = [][]byte{make([]byte, 8192)}
		writeMsgs[i].Buffers = [][]byte{make([]byte, 8192)}
	}

	var payloadReader bytes.Reader
	for {
		// Read batch of incoming packets
		n, err := packconn.ReadBatch(readMsgs, 0)
		if err != nil {
			log.Println("Read error:", err)
			continue
		}

		for i := range n {
			// Extract the payload using the N field (bytes read)
			payloadLen := readMsgs[i].N
			incomingData := readMsgs[i].Buffers[0][:payloadLen]
			payloadReader.Reset(incomingData)

			packet, err := packets.ReadPacket(&payloadReader)
			if err != nil {
				log.Println("Packet generation error:", err)
			}

			// fmt.Println(packet)

			rewritePacket(packet)
			newLen := packet.Length()

			// Prepare the write message
			writeMsgs[i].N = newLen
			writeMsgs[i].Addr = targetAddr
			writeMsgs[i].Buffers[0] = packet.Bytes()
		}

		// Write all processed packets in a single system call
		// We slice writeMsgs[:n] to only send the packets we just translated
		_, err = packconn.WriteBatch(writeMsgs[:n], 0)
		if err != nil {
			log.Println("Write error:", err)
		}
	}
}
