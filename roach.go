package dastard

import (
	"bytes"
	"encoding/binary"
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
	Ndevices   int
	active     []*RoachDevice
	readPeriod time.Duration
	// buffersChan chan AbacoBuffersType
	AnySource
}

// NewRoachDevice creates a new RoachDevice.
func NewRoachDevice(host string, rate float64) (dev *RoachDevice, err error) {
	dev = new(RoachDevice)
	dev.host = host
	dev.rate = rate
	raddr, err := net.ResolveUDPAddr("udp", host)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", raddr)
	if err != nil {
		return nil, err
	}
	dev.conn = conn
	return dev, nil
}

type packetHeader struct {
	Unused   uint8
	Fluxramp uint8
	Nchan    uint16
	Nsamp    uint16
	Flags    uint16
	Sampnum  uint64
}

func parsePacket(packet []byte) (header packetHeader, data []uint16) {
	buf := bytes.NewReader(packet)
	if err := binary.Read(buf, binary.BigEndian, &header); err != nil {
		fmt.Println("binary.Read failed:", err)
	}
	data = make([]uint16, header.Nchan*header.Nsamp)
	if err := binary.Read(buf, binary.BigEndian, &data); err != nil {
		fmt.Println("binary.Read failed:", err)
	}
	return header, data
}

// sampleCard reads a UDP packet and parses it
func (dev *RoachDevice) sampleCard() error {
	p := make([]byte, 16384)
	deadline := time.Now().Add(time.Second)
	if err := dev.conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	n, _, err := dev.conn.ReadFromUDP(p)
	fmt.Printf("%d bytes:\n", n)
	header, _ := parsePacket(p)
	dev.nchan = int(header.Nchan)
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

// RoachSourceConfig holds the arguments needed to call RoachSource.Configure by RPC.
type RoachSourceConfig struct {
	HostPort []string
	Rates    []float64
}

// Configure sets up the internal buffers with given size, speed, and min/max.
func (rs *RoachSource) Configure(config *RoachSourceConfig) (err error) {
	rs.sourceStateLock.Lock()
	defer rs.sourceStateLock.Unlock()
	if rs.sourceState != Inactive {
		return fmt.Errorf("cannot Configure a RoachSource if it's not Inactive")
	}
	n := len(config.HostPort)
	nr := len(config.Rates)
	if n != nr {
		return fmt.Errorf("cannot Configure a RoachSource with %d addresses and %d data rates (%d != %d)",
			n, nr, n, nr)
	}
	rs.Delete()
	for i, host := range config.HostPort {
		rate := config.Rates[i]
		dev, err := NewRoachDevice(host, rate)
		if err != nil {
			return err
		}
		rs.active = append(rs.active, dev)
	}
	return nil
}
