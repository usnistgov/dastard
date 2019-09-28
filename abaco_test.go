package dastard

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"testing"

	"github.com/usnistgov/dastard/packets"
	"github.com/usnistgov/dastard/ringbuffer"
)

func TestGeneratePackets(t *testing.T) {

	ringname := "ring_test_buffer"
	ringdesc := "ring_test_description"
	rb, err := ringbuffer.NewRingBuffer(ringname, ringdesc)
	if err != nil {
		t.Fatalf("Could not open ringbuffer: %s", err)
	}
	defer rb.Unlink()
	if err = rb.Create(128 * 8192); err != nil {
		t.Fatalf("Failed RingBuffer.Create: %s", err)
	}

	p := packets.NewPacket(10, 20, 30, 0)

	const Nchan = 8
	const Nsamp = 20000
	d := make([]int16, Nchan*Nsamp)
	for i := 0; i < Nchan; i++ {
		freq := (float64(i + 2)) / float64(Nsamp)
		for j := 0; j < Nsamp; j++ {
			d[i+Nchan*j] = int16(30000.0 * math.Cos(freq*float64(j)))
		}
	}

	const stride = 400 // We'll put this many samples into a packet
	if stride*Nchan*2 > 8000 {
		t.Fatalf("Packet payload size %d exceeds 8000 bytes", stride*Nchan*2)
	}
	empty := make([]byte, 8192)
	dims := []int16{Nchan}
	for repeats := 0; repeats < 3; repeats++ {
		for i := 0; i < Nsamp; i += stride {
			p.NewData(d[i:i+stride*Nchan], dims)
			b := p.Bytes()
			b = append(b, empty[:8192-len(b)]...)
			rb.Write(b)
		}
		// Consume packetSize
		contents, err := rb.ReadMultipleOf(8192)
		if err != nil {
			t.Errorf("Could not read buffer: %s", err)
		}
		r := bytes.NewReader(contents)
		for {
			pkt, err := packets.ReadPacket(r)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("Error reading packets: %s", err)
				break
			}
			fmt.Printf("Packet read: %s\n", pkt.String())
		}
	}
}
