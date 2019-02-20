package dastard

import (
	"fmt"
	"net"
	"time"
)

// RoachDevice represents a single ROACH device producing data by UDP packets
type RoachDevice struct {
	host  string // in the form: "127.0.0.1:56789"
	rate  float64
	nchan int
	conn  *net.UDPConn // active UDP connection
}

// RoachSource represents multiple ROACH devices
type RoachSource struct {
	Ndevices    int
	active      []*RoachDevice
	readPeriod  time.Duration
	buffersChan chan AbacoBuffersType
	AnySource
}

// NewRoachDevice creates a new RoachDevice.
func NewRoachDevice(host string, rate float64) (dev *RoachDevice, err error) {
	dev = new(RoachDevice)
	dev.host = host
	dev.rate = rate
	var raddr *net.UDPAddr

	if raddr, err = net.ResolveUDPAddr("udp", host); err != nil {
		return nil, err
	}
	if dev.conn, err = net.DialUDP("udp", nil, raddr); err != nil {
		return nil, err
	}
	return dev, nil
}

// sampleCard reads a UDP packet and parses it
func (dev *RoachDevice) sampleCard() error {
	p := make([]byte, 16384)
	n, _, err := dev.conn.ReadFrom(p)
	fmt.Printf("%d bytes:\n%v\n", n, p)
	return err
}

// NewRoachSource creates a new RoachSource.
func NewRoachSource() (*RoachSource, error) {
	source := new(RoachSource)
	source.name = "Roach"
	return source, nil
}

// Sample determines key data facts by sampling some initial data.
func (rs *RoachSource) Sample() error {
	if len(rs.active) <= 0 {
		return fmt.Errorf("No Roach devices are configured")
	}
	for _, device := range rs.active {
		err := device.sampleCard()
		if err != nil {
			return err
		}
		rs.nchan += device.nchan
	}

	return nil
}

// Delete closes all active RoachDevices.
func (rs *RoachSource) Delete() {
	for _, device := range rs.active {
		device.conn.Close()
	}
	rs.active = make([]*RoachDevice, 0)
}

// StartRun tells the hardware to switch into data streaming mode.
// For ROACH µMUX systems, this is always happening. Then launch a goroutine to consume data.
func (rs *RoachSource) StartRun() error {
	// There's no data streaming mode on ROACH, so no need to start it?
	return nil
}
