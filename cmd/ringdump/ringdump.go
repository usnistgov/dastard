package main

import (
	"bytes"
	"flag"
	"fmt"
	"time"

	"github.com/usnistgov/dastard/internal/ringbuffer"
	"github.com/usnistgov/dastard/packets"
)

func dumpdata(data []byte, max int) {
	reader := bytes.NewReader(data)
	packet, _ := packets.ReadPacket(reader)
	fmt.Println(packet.String())
	fmt.Println("Data:")
	if max > len(data) {
		max = len(data)
	}
	if max%16 > 0 {
		max -= max % 16
	}
	for i := 0; i < max; i += 16 {
		for j := i; j < i+16; j++ {
			fmt.Printf("%2.2x ", data[j])
		}
		fmt.Println()
	}
}

func dump(cardnum int) error {
	ringname := fmt.Sprintf("xdma%d_c2h_0_buffer", cardnum)
	ringdesc := fmt.Sprintf("xdma%d_c2h_0_description", cardnum)
	fmt.Println("Dumping ring", ringname)
	ring, err := ringbuffer.NewRingBuffer(ringname, ringdesc)
	if err != nil {
		return err
	}
	err = ring.Open()
	if err != nil {
		return err
	}
	defer ring.Close()
	ps, _ := ring.PacketSize()
	br := ring.BytesReadable()
	bw := ring.BytesWriteable()
	bs := 1 + br + bw
	fmt.Printf("Buffer has packet size %3d and %7d bytes are readable, %7d writeable, %7d total.\n", ps, br, bw, bs)
	data1, err := ring.ReadMultipleOf(int(ps))
	if err != nil {
		return err
	}

	ring.DiscardStride(uint64(ps))
	fmt.Println("<empty buffer>")
	br = ring.BytesReadable()
	bw = ring.BytesWriteable()
	fmt.Printf("Buffer has packet size %3d and %7d bytes are readable, %7d writeable, %7d total.\n", ps, br, bw, bs)
	time.Sleep(200 * time.Millisecond)
	br = ring.BytesReadable()
	bw = ring.BytesWriteable()
	fmt.Println("<sleep 200ms>")
	fmt.Printf("Buffer has packet size %3d and %7d bytes are readable, %7d writeable, %7d total.\n", ps, br, bw, bs)
	data2, err := ring.ReadMultipleOf(int(ps))
	if err != nil {
		return err
	}
	dumpdata(data1, int(ps))
	dumpdata(data2, int(ps))
	return nil
}

func main() {
	cardnum := flag.Int("cardnum", 1, "Number of the ring buffer to open. 0-999 allowed")
	flag.Usage = func() {
		fmt.Println("ringdump, a program to dump the Abaco data ring buffer status")
		fmt.Println("Usage:")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *cardnum < 0 || *cardnum > 999 {
		fmt.Println("Cardnum must be in the range [0, 999].")
		return
	}

	err := dump(*cardnum)
	if err != nil {
		fmt.Println("dump returned error: ", err)
	}
}
