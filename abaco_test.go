package dastard

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"math/rand"
	"testing"
	"time"

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
	const packetAlign = 8192
	if err = rb.Create(128 * packetAlign); err != nil {
		t.Fatalf("Failed RingBuffer.Create: %s", err)
	}

	p := packets.NewPacket(10, 20, 0x100, 0)

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
	empty := make([]byte, packetAlign)
	dims := []int16{Nchan}
	for repeats := 0; repeats < 3; repeats++ {
		for i := 0; i < Nsamp; i += stride {
			p.NewData(d[i:i+stride*Nchan], dims)
			b := p.Bytes()
			b = append(b, empty[:packetAlign-len(b)]...)
			rb.Write(b)
		}
		// Consume packetSize
		contents, err := rb.ReadMultipleOf(packetAlign)
		if err != nil {
			t.Errorf("Could not read buffer: %s", err)
		}
		r := bytes.NewReader(contents)
		for {
			bytesRemaining := r.Len()
			_, err := packets.ReadPacket(r)
			bytesUsed := bytesRemaining - r.Len()
			if bytesUsed%packetAlign > 0 {
				if _, err = r.Seek(int64(packetAlign-(bytesUsed%packetAlign)), io.SeekCurrent); err != nil {
					t.Errorf("Could not seek")
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("Error reading packets: %s", err)
				break
			}
		}
	}
}

func TestAbacoRing(t *testing.T) {
	if _, err := NewAbacoRing(99999); err == nil {
		t.Errorf("NewAbacoRing(99999) succeeded, want failure")
	}
	rand.Seed(time.Now().UnixNano())
	cardnum := -rand.Intn(99998) - 1 // Rand # between -1 and -99999
	dev, err := NewAbacoRing(cardnum)
	if err != nil {
		t.Fatalf("NewAbacoRing(%d) fails: %s", cardnum, err)
	}

	ringname := fmt.Sprintf("xdma%d_c2h_0_buffer", cardnum)
	ringdesc := fmt.Sprintf("xdma%d_c2h_0_description", cardnum)
	ring, err := ringbuffer.NewRingBuffer(ringname, ringdesc)
	if err != nil {
		t.Fatalf("Could not open ringbuffer: %s", err)
	}
	defer ring.Unlink()
	const packetAlign = 8192
	if err = ring.Create(128 * packetAlign); err != nil {
		t.Fatalf("Failed RingBuffer.Create: %s", err)
	}
	dev.samplePackets()

	p := packets.NewPacket(10, 20, 0x100, 0)
	const Nchan = 8
	const Nsamp = 20000
	d := make([]int16, Nchan*Nsamp)
	for i := 0; i < Nchan; i++ {
		freq := (float64(i + 2)) / float64(Nsamp)
		for j := 0; j < Nsamp; j++ {
			d[i+Nchan*j] = int16(30000.0 * math.Cos(freq*float64(j)))
		}
	}

	const stride = 500 // We'll put this many samples into a packet
	if stride*Nchan*2 > 8000 {
		t.Fatalf("Packet payload size %d exceeds 8000 bytes", stride*Nchan*2)
	}
	empty := make([]byte, packetAlign)
	dims := []int16{Nchan}
	const counterRate = 1e9
	counter := uint64(0)
	for repeats := 0; repeats < 6; repeats++ {
		// Repeat the data buffer `d` once per outer loop iteration
		for i := 0; i < Nsamp; i += stride {
			// Each inner loop iteration generates 1 packet. Nsamp/stride packets completes buffer `d`.
			p.NewData(d[i:i+stride*Nchan], dims)
			ts := packets.MakeTimestamp(uint16(counter>>32), uint32(counter), counterRate)
			p.SetTimestamp(ts)
			b := p.Bytes()

			b = append(b, empty[:packetAlign-len(b)]...)
			if ring.BytesWriteable() >= len(b) {
				ring.Write(b)
			}
			counter += stride * 1000
		}
		time.Sleep(Nsamp * time.Microsecond) // sample rate is 1/μs.
	}
	ps, err := dev.ReadAllPackets()
	if err != nil {
		t.Errorf("Error on AbacoRing.ReadAllPackets: %v", err)
	}
	if len(ps) <= 0 {
		t.Errorf("AbacoRing.samplePackets returned only %d packets", len(ps))
	}
	group := NewAbacoGroup(gIndex(ps[0]))
	group.queue = append(group.queue, ps...)
	err = group.samplePackets()
	if err != nil {
		t.Errorf("Error on AbacoGroup.samplePackets: %v", err)
	}

	if group.nchan != Nchan {
		t.Errorf("group.nchan=%d, want %d", group.nchan, Nchan)
	}
	if group.sampleRate <= 0.0 {
		t.Errorf("group.sampleRate=%.4g, want >0", group.sampleRate)
	}
}

func TestAbacoSource(t *testing.T) {
	source, err := NewAbacoSource()
	if err != nil {
		t.Fatalf("NewAbacoSource() fails: %s", err)
	}

	rand.Seed(time.Now().UnixNano())
	cardnum := -rand.Intn(99998) - 1 // Rand # between -1 and -99999

	dev, err := NewAbacoRing(cardnum)
	if err != nil {
		t.Fatalf("NewAbacoRing(%d) fails: %s", cardnum, err)
	}
	source.arings[cardnum] = dev
	source.Nrings++

	deviceCodes := []int{cardnum}
	var config AbacoSourceConfig
	config.ActiveCards = deviceCodes
	err = source.Configure(&config)
	if err != nil {
		t.Fatalf("AbacoSource.Configure(%v) fails: %s", config, err)
	}

	ringname := fmt.Sprintf("xdma%d_c2h_0_buffer", cardnum)
	ringdesc := fmt.Sprintf("xdma%d_c2h_0_description", cardnum)
	rb, err := ringbuffer.NewRingBuffer(ringname, ringdesc)
	if err != nil {
		t.Fatalf("Could not open ringbuffer: %s", err)
	}
	defer rb.Unlink()
	const packetAlign = 8192
	if err = rb.Create(256 * packetAlign); err != nil {
		t.Fatalf("Failed RingBuffer.Create: %s", err)
	}

	const Nchan = 8
	const Nsamp = 20000
	const stride = 500 // We'll put this many samples into a packet
	if stride*Nchan*2 > 8000 {
		t.Fatalf("Packet payload size %d exceeds 8000 bytes", stride*Nchan*2)
	}

	abortSupply := make(chan interface{})
	supplyDataForever := func() {
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
		timer := time.NewTicker(10 * time.Millisecond)
		packetcount := 0
		for ;; {
			select {
			case <-abortSupply:
				return
			case <-timer.C:
				for i := 0; i < Nsamp; i += stride {
					p.NewData(d[i:i+stride*Nchan], dims)
					// Skip 2 packets of every 46
					packetcount++
					if packetcount % 46 < 2 {
						continue
					}
					b := p.Bytes()
					b = append(b, empty[:packetAlign-len(b)]...)
					if rb.BytesWriteable() >= len(b) {
						rb.Write(b)
					}
				}
			}
		}
	}
	go supplyDataForever()

	queuedRequests := make(chan func())
	Npresamp := 256
	Nsamples := 1024
	if err := Start(source, queuedRequests, Npresamp, Nsamples); err != nil {
		fmt.Printf("Result of Start(source,...): %s\n", err)
		t.Fatal(err)
	}
	if source.nchan != Nchan {
		t.Errorf("source.nchan=%d, want %d", source.nchan, Nchan)
	}
	time.Sleep(250 * time.Millisecond)
	source.Stop()
	close(abortSupply)
	source.RunDoneWait()
}
