package main

import (
	"fmt"
	"math"
	"time"

	"github.com/usnistgov/dastard/packets"
	"github.com/usnistgov/dastard/ringbuffer"
)

func generateData(cardnum int, cancel chan struct{}) error {
	ringname := fmt.Sprintf("xdma%d_c2h_0_buffer", cardnum)
	ringdesc := fmt.Sprintf("xdma%d_c2h_0_description", cardnum)
	ring, err := ringbuffer.NewRingBuffer(ringname, ringdesc)
	if err != nil {
		return fmt.Errorf("Could not open ringbuffer: %s", err)
	}
	ring.Unlink()       // in case it exists from before
	defer ring.Unlink() // so it won't exist after
	const packetAlign = 8192
	if err = ring.Create(256 * packetAlign); err != nil {
		return fmt.Errorf("Failed RingBuffer.Create: %s", err)
	}

	const Nchan = 8
	const Nsamp = 20000
	const stride = 500 // We'll put this many samples into a packet
	if stride*Nchan*2 > 8000 {
		return fmt.Errorf("Packet payload size %d exceeds 8000 bytes", stride*Nchan*2)
	}

	fmt.Printf("Generating data in shm:%s\n", ringname)
	p := packets.NewPacket(10, 20, 0x100, 0)
	d := make([]int16, Nchan*Nsamp)
	for i := 0; i < Nchan; i++ {
		freq := (float64(i + 2)) / float64(Nsamp)
		for j := 0; j < Nsamp; j++ {
			d[i+Nchan*j] = int16(30000.0 * math.Cos(freq*float64(j)))
		}
	}

	empty := make([]byte, packetAlign)
	dims := []int16{Nchan}
	timer := time.NewTicker(40 * time.Millisecond)
	for {
		select {
		case <-cancel:
			return nil
		case <-timer.C:
			for i := 0; i < Nsamp; i += stride {
				p.NewData(d[i:i+stride*Nchan], dims)
				b := p.Bytes()
				b = append(b, empty[:packetAlign-len(b)]...)
				if ring.BytesWriteable() >= len(b) {
					ring.Write(b)
				}
			}
		}
	}
}

func main() {
	const cardnum = 3
	cancel := make(chan struct{})
	generateData(cardnum, cancel)
}
