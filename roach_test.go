package dastard

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func publishRoachPackets(port int, value uint16) (closer chan struct{}, err error) {

	host := fmt.Sprintf("127.0.0.1:%d", port)
	raddr, err := net.ResolveUDPAddr("udp", host)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, 4016)
	for i := 0; i < 4016; i++ {
		buffer[i] = byte(i % 256)
	}

	closer = make(chan struct{})
	go func() {
		defer conn.Close()
		for i := 0; ; i++ {
			select {
			case <-closer:
				return
			default:
				conn.Write(buffer)
				// conn.WriteTo(buffer, &raddr)
				fmt.Printf("Wrote buffer iteration %4d of size %d: %v\n", i, len(buffer), buffer[0:20])
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
	return closer, nil
}

// TestPublish checks ??
func TestPublish(t *testing.T) {
	port := 60001
	host := fmt.Sprintf("localhost:%d", port)
	closer, err := publishRoachPackets(port, 0xbeef)
	if err != nil {
		t.Errorf("publishRoachPackets returned %v", err)
	}

	dev, err := NewRoachDevice(host, 40000.0)
	if err != nil {
		t.Errorf("NewRoachDevice returned %v", err)
	}
	time.Sleep(time.Second)
	fmt.Printf("dev: %v\n", dev)
	err = dev.sampleCard()
	if err != nil {
		t.Errorf("sampleCard returned %v", err)
	}
	close(closer)
}
