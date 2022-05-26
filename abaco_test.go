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
	timeout := 2000 * time.Millisecond
	dev.start()
	dev.samplePackets(timeout)

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
	const enable = true
	const resetAfter = 20000
	unwrapOpts := AbacoUnwrapOptions{}
	const pulseSign = +1
	group := NewAbacoGroup(gIndex(ps[0]), unwrapOpts)
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

func TestAbacoUDP(t *testing.T) {
	if _, err := NewAbacoUDPReceiver("nonexistenthost.remote.internet:4999"); err == nil {
		t.Errorf("NewAbacoUDPReceiver(\"nonexistenthost.remote.internet:4999\") succeeded, want failure")
	}
	device, err := NewAbacoUDPReceiver("localhost:4999")
	if err != nil {
		t.Errorf("NewAbacoUDPReceiver(\"localhost:4999\") failed: %v", err)
	}
	err = device.start()
	if err != nil {
		t.Errorf("AbacoUDP.start() failed: %v", err)
	}
	err = device.discardStale()
	if err != nil {
		t.Errorf("AbacoUDP.discardStale() failed: %v", err)
	}
	go func() { <-device.data }()
	err = device.stop()
	if err != nil {
		t.Errorf("AbacoUDP.stop() failed: %v", err)
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

	var config AbacoSourceConfig

	// Check that ActiveCards and HostPortUDP slices are unique-ified when source.Configure(&config) called.
	config.ActiveCards = []int{4, 3, 3, 3, 3, 3, 4, 3, 3}
	config.HostPortUDP = []string{"localhost:4444", "localhost:3333", "localhost:4444"}
	err = source.Configure(&config)
	if err == nil {
		t.Fatalf("AbacoSource.Configure(%v) should fail but doesn't", config)
	}
	if len(config.ActiveCards) > 2 {
		t.Errorf("AbacoSource.Configure() returns with repeats in ActiveCards=%v", config.ActiveCards)
	}
	if len(config.HostPortUDP) > 2 {
		t.Errorf("AbacoSource.Configure() returns with repeats in HostPortUDP=%v", config.HostPortUDP)
	}

	deviceCodes := []int{cardnum}
	config.ActiveCards = deviceCodes
	config.HostPortUDP = []string{}
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
		p := packets.NewPacket(10, 20, 100, 0)
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
		for {
			select {
			case <-abortSupply:
				return
			case <-timer.C:
				for i := 0; i < Nsamp; i += stride {
					p.NewData(d[i:i+stride*Nchan], dims)
					// Skip 2 packets of every 80, the 1st and 80th.
					packetcount++
					if packetcount%80 < 2 {
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
	source.RunDoneWait()

	// Start a 2nd time
	err = source.Configure(&config)
	if err != nil {
		t.Fatalf("AbacoSource.Configure(%v) fails: %s", config, err)
	}
	if err := Start(source, queuedRequests, Npresamp, Nsamples); err != nil {
		fmt.Printf("Result of Start(source,...): %s\n", err)
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	source.Stop()
	close(abortSupply)
	source.RunDoneWait()
}

func prepareDemux(nframes int) (*AbacoGroup, []*packets.Packet, [][]RawType) {
	const offset = 1
	const nchan = 16
	unwrapOpts := AbacoUnwrapOptions{Enable: true, ResetAfter: 20000, PulseSign: 1}

	group := NewAbacoGroup(GroupIndex{Firstchan: offset, Nchan: nchan}, unwrapOpts)
	group.unwrap = group.unwrap[:0] // get rid of phase unwrapping

	copies := make([][]RawType, nchan)
	for i := 0; i < nchan; i++ {
		copies[i] = make([]RawType, nframes)
	}
	const stride = 4096 / nchan
	dims := []int16{nchan}
	allpackets := make([]*packets.Packet, 0)
	for j := 0; j < nframes; j += stride {
		p := packets.NewPacket(10, 20, uint32(j/stride), offset)
		d := make([]int16, 0, 4096)
		for k := 0; k < stride; k++ {
			for m := 0; m < nchan; m++ {
				d = append(d, int16(m+j+k))
			}
		}
		p.NewData(d, dims)
		allpackets = append(allpackets, p)
	}
	return group, allpackets, copies
}

func TestDemux(t *testing.T) {
	const nframes = 32768
	group, allpackets, copies := prepareDemux(nframes)
	want := (nframes * len(copies)) / 4096
	if len(allpackets) != want {
		t.Errorf("prepareDemux returns %d packets, want %d", len(allpackets), want)
	}

	group.queue = append(group.queue, allpackets...)
	group.demuxData(copies, nframes)
	for chidx, datacopy := range copies {
		for i, val := range datacopy {
			want := RawType(i + chidx)
			if val != want {
				t.Fatalf("datacopies[%d][%d] =  0x%x, want 0x%x", chidx, i, val, want)
			}
		}
	}
}

func BenchmarkDemux(b *testing.B) {
	const nframes = 32768
	group, allpackets, copies := prepareDemux(nframes)
	for i := 0; i < b.N; i++ {
		group.queue = append(group.queue, allpackets...)
		group.demuxData(copies, nframes)
	}
}
