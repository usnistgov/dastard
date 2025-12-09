package dastard

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net"
	"testing"
	"time"

	"github.com/usnistgov/dastard/packets"
)

func TestGeneratePackets(t *testing.T) {

	const packetAlign = 8192
	p := packets.NewPacket(10, 20, 0x100, 0)
	var rb bytes.Buffer

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
		contents := make([]byte, packetAlign)
		n, err := rb.Read(contents)
		if err != nil {
			t.Errorf("Could not read buffer: %s", err)
		} else if n < packetAlign {
			t.Errorf("buffer read gives %d bytes, want %d", n, packetAlign)
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
		t.Errorf("NewAbacoSource() fails: %s", err)
	}

	var config AbacoSourceConfig

	config.HostPortUDP = []string{}
	if err = source.Configure(&config); err != nil {
		t.Errorf("AbacoSource.Configure(%v) fails: %s", config, err)
	}

	// Check that HostPortUDP slices are unique-ified when source.Configure(&config) called.
	config.HostPortUDP = []string{"localhost:4444", "localhost:3333", "localhost:4444"}
	if err = source.Configure(&config); err != nil {
		t.Errorf("source.Configure(&c) fails with c=%v, err=%v", config, err)
	}
	if len(config.HostPortUDP) > 2 {
		t.Errorf("source.Configure() returns with repeats in HostPortUDP=%v", config.HostPortUDP)
	} else if len(config.HostPortUDP) < 2 {
		t.Errorf("source.Configure() returns output expected 2 ports in HostPortUDP=%v", config.HostPortUDP)
	}

	config.HostPortUDP = []string{"localhost:4444"}
	if err = source.Configure(&config); err != nil {
		t.Errorf("source.Configure(&c) fails with c=%v, err=%v", config, err)
	}
	if len(config.HostPortUDP) != 1 {
		t.Errorf("source.Configure() returns output expected 1 ports in HostPortUDP=%v", config.HostPortUDP)
	}

	const packetAlign = 8192
	const Nchan = 8
	const Nsamp = 20000
	const stride = 500 // We'll put this many samples into a packet
	if stride*Nchan*2 > 8000 {
		t.Fatalf("Packet payload size %d exceeds 8000 bytes", stride*Nchan*2)
	}


	abortSupply := make(chan interface{})
	supplyDataForever := func() {
		targetAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:4444")
		if err != nil {
			t.Fatalf("Error resolving UDP address: %v", err)
		}

		// Dial a UDP connection (this doesn't establish a persistent connection like TCP)
		conn, err := net.DialUDP("udp", nil, targetAddr)
		if err != nil {
			t.Fatalf("Error dialing UDP: %v", err)
		}
		defer conn.Close() // Close the connection when done

		p := packets.NewPacket(10, 20, 100, 0)
		d := make([]int16, Nchan*Nsamp)
		for i := 0; i < Nchan; i++ {
			freq := (float64(i + 2)) / float64(Nsamp)
			for j := 0; j < Nsamp; j++ {
				d[i+Nchan*j] = int16(30000.0 * math.Cos(freq*float64(j)))
			}
		}

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
					conn.Write(b)
				}
			}
		}
	}
	go supplyDataForever()

	queuedRequests := make(chan func())
	Npresamp := 256
	Nsamples := 1024
	fmt.Printf("Source: %v\n", source)
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

	// Check that config unwrap options are tested for validity.
	var unwrapTests = []struct {
		unwrap      bool
		rescale     bool
		shouldError bool
	}{
		{true, false, true},
		{true, true, false},
		{false, true, false},
		{false, false, false},
	}
	for _, test := range unwrapTests {
		config.Unwrap = test.unwrap
		config.RescaleRaw = test.rescale
		err = source.Configure(&config)
		diderror := (err != nil)
		if diderror != test.shouldError {
			if test.shouldError {
				t.Fatalf("AbacoSource.Configure(%v) does not fail with invalid AbacoUnwrapOptions", config)
			} else {
				t.Fatalf("AbacoSource.Configure(%v) fails with valid AbacoUnwrapOptions: %s", config, err)
			}
		}
	}
}

func prepareDemux(nframes int) (*AbacoGroup, []*packets.Packet, [][]RawType) {
	const offset = 1
	const nchan = 16
	unwrapOpts := AbacoUnwrapOptions{Unwrap: true, RescaleRaw: true, ResetAfter: 20000, PulseSign: 1}

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
