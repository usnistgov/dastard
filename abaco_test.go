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
			// fmt.Printf("Packet read: %s\n", pkt.String())
		}
	}
}

func TestAbacoDevice(t *testing.T) {
	if _, err := NewAbacoDevice(99999); err == nil {
		t.Errorf("NewAbacoDevice(99999) succeeded, want failure")
	}
	rand.Seed(time.Now().UnixNano())
	cardnum := -rand.Intn(99998) - 1 // Rand # between -1 and -99999
	dev, err := NewAbacoDevice(cardnum)
	if err != nil {
		t.Fatalf("NewAbacoDevice(%d) fails: %s", cardnum, err)
	}
	fmt.Printf("dev: %v\n", dev)

	ringname := fmt.Sprintf("xdma%d_c2h_0_buffer", cardnum)
	ringdesc := fmt.Sprintf("xdma%d_c2h_0_description", cardnum)
	rb, err := ringbuffer.NewRingBuffer(ringname, ringdesc)
	if err != nil {
		t.Fatalf("Could not open ringbuffer: %s", err)
	}
	defer rb.Unlink()
	const packetAlign = 8192
	if err = rb.Create(128 * packetAlign); err != nil {
		t.Fatalf("Failed RingBuffer.Create: %s", err)
	}

	go func() {
		err = dev.sampleCard()
		if err == nil {
			fmt.Printf("Result of dev.sampleCard: %v\n", dev)

		} else {
			fmt.Printf("Result of dev.sampleCard: %s\n", err)
		}
	}()

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
	for repeats := 0; repeats < 6; repeats++ {
		for i := 0; i < Nsamp; i += stride {
			p.NewData(d[i:i+stride*Nchan], dims)
			b := p.Bytes()
			b = append(b, empty[:packetAlign-len(b)]...)
			if rb.BytesWriteable() >= len(b) {
				rb.Write(b)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if dev.nchan != Nchan {
		t.Errorf("dev.nchan=%d, want %d", dev.nchan, Nchan)
	}
}

func TestAbacoSource(t *testing.T) {
	source, err := NewAbacoSource()
	if err != nil {
		t.Fatalf("NewAbacoSource() fails: %s", err)
	}

	rand.Seed(time.Now().UnixNano())
	cardnum := -rand.Intn(99998) - 1 // Rand # between -1 and -99999

	dev, err := NewAbacoDevice(cardnum)
	if err != nil {
		t.Fatalf("NewAbacoDevice(%d) fails: %s", cardnum, err)
	}
	fmt.Printf("dev: %v\n", dev)
	source.devices[cardnum] = dev
	source.Ndevices++
	fmt.Printf("source.devices: %v\n", source.devices)

	deviceCodes := []int{cardnum}
	var config AbacoSourceConfig
	config.ActiveCards = deviceCodes
	err = source.Configure(&config)
	if err != nil {
		t.Fatalf("AbacoSource.Configure(%v) fails: %s", config, err)
	}
	fmt.Printf("AbacoSource.Configure() produces %v\n", config)

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
		for {
			select {
			case <-abortSupply:
				fmt.Println("Abort supply channel closed, so supplyDataForever is ending.")
				return
			case <-timer.C:
				for i := 0; i < Nsamp; i += stride {
					p.NewData(d[i:i+stride*Nchan], dims)
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
	if dev.nchan != Nchan {
		t.Errorf("dev.nchan=%d, want %d", dev.nchan, Nchan)
	}
	time.Sleep(150 * time.Millisecond)
	source.Stop()
	close(abortSupply)
	source.RunDoneWait()
}
