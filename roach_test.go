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

	header := []interface{}{
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

func publishRoachPackets(port int, nchan uint16, value uint16) (closer chan struct{}, err error) {

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
		for i := uint64(0); ; i++ {
			select {
			case <-closer:
				return
			default:
				buffer := newBuffer(nchan, nsamp, i)
				conn.Write(buffer)
				fmt.Printf("Wrote buffer iteration %4d of size %d: %v\n", i, len(buffer), buffer[0:20])
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
	return closer, nil
}

// TestDevice checks that the raw RoachDevice can receive and parse a header
func TestDevice(t *testing.T) {
	// Start generating Roach packets, until closer is closed.
	port := 60001
	var nchan uint16 = 40
	closer, err := publishRoachPackets(port, nchan, 0xbeef)
	if err != nil {
		t.Errorf("publishRoachPackets returned %v", err)
	}

	host := fmt.Sprintf("localhost:%d", port)
	dev, err := NewRoachDevice(host, 40000.0)
	if err != nil {
		t.Errorf("NewRoachDevice returned %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	err = dev.sampleCard()
	if err != nil {
		t.Errorf("sampleCard returned %v", err)
	}
	if dev.nchan != int(nchan) {
		t.Errorf("parsed packet header says nchan=%d, want %d", dev.nchan, nchan)
	}
	close(closer)
}

// TestDevice checks that the raw RoachDevice can receive and parse a header
func TestRoachSource(t *testing.T) {
	// Start generating Roach packets, until closer is closed.
	port := 60002
	var nchan uint16 = 40
	closer, err := publishRoachPackets(port, nchan, 0xbeef)
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
		Rates:    []float64{40000.0},
	}
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

	err = rs.StartRun()
	if err != nil {
		t.Errorf("RoachSource.StartRun returned %v", err)
	}
	// Start(rs, )

	close(closer)
	rs.Delete()
}
