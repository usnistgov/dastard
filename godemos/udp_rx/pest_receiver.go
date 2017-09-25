// Reads the UDP packets sent by the ROACH-2 board or by its emulator PEST.

package main

import (
	"encoding/binary"
	"fmt"
	"net"
)

func main() {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:60000")
	if err != nil {
		fmt.Printf("Some error %v", err)
		return
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Printf("Some error %v", err)
		return
	}
	defer conn.Close()
	fmt.Println("connected")

	//simple Read. Just the first 10 packets
	buffer := make([]byte, 1024)
	for n := 0; n < 10; n++ {
		n, err := conn.Read(buffer)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Found %d bytes in ROACH droppings packet\n", n)
		packetType := buffer[0]
		packetVersion := buffer[1]
		numChannels := binary.LittleEndian.Uint16(buffer[2:])
		numSamples := binary.LittleEndian.Uint16(buffer[4:])
		packetFlags := binary.LittleEndian.Uint16(buffer[6:])
		firstSampleNumber := binary.LittleEndian.Uint64(buffer[8:])
		fmt.Printf("Packet type %d version %d. %d channels, %d samples each. Flags 0x%x. First samp # = %d\n",
			packetType, packetVersion, numChannels, numSamples, packetFlags, firstSampleNumber)
		for i := 16; i < n; i += 2 {
			fmt.Printf("%04x ", binary.LittleEndian.Uint16(buffer[i:]))
			if i%32 == 14 {
				fmt.Println()
			}
		}
		fmt.Println()
	}
}
