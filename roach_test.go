//go:build !ci
// +build !ci

// Don't run these Roach tests on CI platforms. There were UDP problems 10/23/2020 on Travis.

package dastard

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"
)

func newBuffer(nchan, nsamp uint16, sampnum uint64) []byte {
	buf := new(bytes.Buffer)

	header := []any{
		uint8(0), // unused
		uint8(1), // flag: 1=flux-ramp demodulation
		nchan,
		nsamp,
		uint16(1), // flag: 1 means 2-byte words
		sampnum,
	}
	for _, v := range header {
		if err := binary.Write(buf, binary.BigEndian, v); err != nil {
			fmt.Println("binary.Write failed:", err)
			return buf.Bytes()
		}
	}
	for i := uint16(0); i < nchan*nsamp; i++ {
		if err := binary.Write(buf, binary.BigEndian, i); err != nil {
			fmt.Println("binary.Write failed:", err)
			break
		}
	}
	return buf.Bytes()
}

func publishRoachPackets(port int, nchan uint16) (closer chan struct{}, err error) {

	host := fmt.Sprintf("127.0.0.1:%d", port)
	raddr, err := net.ResolveUDPAddr("udp", host)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}

	nsamp := 8000 / (2 * nchan)

	closer = make(chan struct{})
	go func() {
		defer conn.Close()
		for i := uint64(0); ; i += uint64(nsamp) {
			select {
			case <-closer:
				return
			default:
				buffer := newBuffer(nchan, nsamp, i)
				conn.Write(buffer)
				// fmt.Printf("Wrote buffer iteration %4d of size %d: %v\n", i, len(buffer), buffer[0:20])
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
	return closer, nil
}

// TestDevice checks that the raw RoachDevice can receive and parse a header
func TestRoachDevice(t *testing.T) {
	// Start generating Roach packets, until closer is closed.
	port := 60001
	var nchan uint16 = 40
	packetSourceCloser, err := publishRoachPackets(port, nchan)
	defer close(packetSourceCloser)
	if err != nil {
		t.Errorf("publishRoachPackets returned %v", err)
	}

	host := fmt.Sprintf("localhost:%d", port)
	dev, err := NewRoachDevice(host, 40000.0)
	if err != nil {
		t.Errorf("NewRoachDevice returned %v", err)
	}
	dev.unwrapOpts.PulseSign = +1
	dev.unwrapOpts.Bias = false
	dev.unwrapOpts.RescaleRaw = true
	dev.unwrapOpts.Unwrap = true
	time.Sleep(50 * time.Millisecond)
	err = dev.samplePacket()
	if err != nil {
		t.Errorf("samplePacket returned %v", err)
	}
	if dev.nchan != int(nchan) {
		t.Errorf("parsed packet header says nchan=%d, want %d", dev.nchan, nchan)
	}

	nextBlock := make(chan *dataBlock)
	go dev.readPackets(nextBlock)
	timeout := time.NewTimer(time.Second)
	select {
	case <-timeout.C:
		t.Errorf("RoachDevice.readPackets launched but no data received after timeout")
	case block := <-nextBlock:
		if len(block.segments) != dev.nchan {
			t.Errorf("RoachDevice block has %d data segments, want %d", len(block.segments), dev.nchan)
		}
		for i, seg := range block.segments {
			want := RawType(i/4) + 4096
			if seg.rawData[0] != want {
				t.Errorf("RoachDevice block.segments[%d][0] = %d, want %d",
					i, seg.rawData[0], want)
			}
			if len(seg.rawData) != block.nSamp {
				t.Errorf("RoachDevice block.segments[%d] length=%d, want %d", i, len(seg.rawData), block.nSamp)
			}
		}
		if block.nSamp < 10 {
			t.Errorf("RoachDevice block.nSamp = %d, want at least 10", block.nSamp)
		}
	}
}

// TestDevice checks that the full RoachSource can receive and parse a header
func TestRoachSource(t *testing.T) {
	// Start generating Roach packets, until closer is closed.
	port := 60005
	var nchan uint16 = 40
	packetSourceCloser, err := publishRoachPackets(port, nchan)
	defer close(packetSourceCloser)

	if err != nil {
		t.Errorf("publishRoachPackets returned %v", err)
	}

	rs, err := NewRoachSource()
	if err != nil {
		t.Errorf("NewRoachSource returned %v", err)
	}
	host := fmt.Sprintf("localhost:%d", port)
	config := RoachSourceConfig{
		HostPort: []string{host},
		Rates:    []float64{40000.0, 50000.0}, // 2 Rates will be an error
		AbacoUnwrapOptions: AbacoUnwrapOptions{
			PulseSign: +1,
			Unwrap:    false,
		},
	}
	err = rs.Configure(&config)
	if err == nil {
		t.Errorf("RoachSource.Configure should fail when HostPort and Rates are of unequal length")
	}

	config.Rates = config.Rates[0:1] // Fix rates so it's now of length 1
	err = rs.Configure(&config)
	if err != nil {
		t.Errorf("RoachSource.Configure returned %v", err)
	}
	if len(rs.active) != 1 {
		t.Errorf("RoachSource.active has length %d, want 1", len(rs.active))
	}
	dev := rs.active[0]
	if dev.conn == nil {
		t.Errorf("RoachSource[0].conn is nil, should be connected")
	}
	if dev.nchan != 0 {
		t.Errorf("RoachSource[0].nchan before Sample is %d, should be 0", dev.nchan)
	}

	err = rs.Sample()
	if err != nil {
		t.Errorf("RoachSource.Sample returned %v", err)
	}
	if dev.nchan != int(nchan) {
		t.Errorf("RoachSource[0].nchan after Sample is %d, should be %d", dev.nchan, nchan)
	}

	queuedRequests := make(chan func())
	npre := 300
	nsamp := 1000
	err = Start(rs, queuedRequests, npre, nsamp)
	if err != nil {
		t.Errorf("Start(RoachSource,...) returned %v", err)
	}
	err = rs.Configure(&config)
	if err == nil {
		t.Errorf("RoachSource.Configure should fail when source is Active, but it didn't")
	}

	timeout := time.NewTimer(500 * time.Millisecond) // this was 200 Millisecond, but tests hung sometimes so I made it bigger
	select {
	case <-timeout.C:
		t.Errorf("RoachDevice.readPackets launched but no data received after timeout")

	case block := <-rs.nextBlock:
		if len(block.segments) != dev.nchan {
			t.Errorf("RoachSource block has %d data segments, want %d", len(block.segments), dev.nchan)
		}
		for i, seg := range block.segments {
			want := RawType(i/4) + 4096
			if seg.rawData[0] != want {
				t.Errorf("RoachSource block.segments[%d][0] = %d, want %d",
					i, seg.rawData[0], want)
			}
			if len(seg.rawData) != block.nSamp {
				t.Errorf("RoachSource block.segments[%d] length=%d, want %d", i, len(seg.rawData), block.nSamp)
			}
		}
		if block.nSamp < 10 {
			t.Errorf("RoachSource block.nSamp = %d, want at least 10", block.nSamp)
		}
		<-timeout.C
	}

	err = rs.Stop()
	if err != nil {
		t.Errorf("RoachSource.Stop returned %v", err)
	}
}
