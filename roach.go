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
	host   string        // in the form: "127.0.0.1:56789"
	rate   float64       // Sampling rate (not reported by the device)
	period time.Duration // Sampling period = 1/rate
	nchan  int
	conn   *net.UDPConn // active UDP connection
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
	dev.period = time.Duration(1e9 / rate)
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

// parsePacket converts a roach packet into its constituent packetHeader and
// raw data. For now, assume all data are 2 bytes long and thus == RawType.
// TODO: In the future, we might need to accept 4-byte data and convert it
// (by rounding) into 2-byte data.
// TODO: verify that we don't need to add 0x8000 to convert signed->unsigned.
func parsePacket(packet []byte) (header packetHeader, data []RawType) {
	buf := bytes.NewReader(packet)
	if err := binary.Read(buf, binary.BigEndian, &header); err != nil {
		fmt.Println("binary.Read failed:", err)
	}
	data = make([]RawType, header.Nchan*header.Nsamp)
	if err := binary.Read(buf, binary.BigEndian, &data); err != nil {
		fmt.Println("binary.Read failed:", err)
	}
	return header, data
}

// samplePacket reads a UDP packet and parses it
func (dev *RoachDevice) samplePacket() error {
	p := make([]byte, 16384)
	deadline := time.Now().Add(time.Second)
	if err := dev.conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	_, _, err := dev.conn.ReadFromUDP(p)
	header, _ := parsePacket(p)
	dev.nchan = int(header.Nchan)
	return err
}

// readPackets
func (dev *RoachDevice) readPackets(nextBlock chan *dataBlock) {
	p := make([]byte, 16384)
	var err error
	for {
		deadline := time.Now().Add(time.Second)
		if err = dev.conn.SetReadDeadline(deadline); err != nil {
			block := dataBlock{err: err}
			nextBlock <- &block
			return
		}
		_, _, err = dev.conn.ReadFromUDP(p)
		readTime := time.Now()
		header, data := parsePacket(p)
		if dev.nchan != int(header.Nchan) {
			err = fmt.Errorf("RoachDevice Nchan changed from %d -> %d", dev.nchan,
				header.Nchan)
		}
		firstlastDelay := time.Duration(header.Nsamp-1) * dev.period
		firstTime := readTime.Add(-firstlastDelay)
		block := new(dataBlock)
		block.segments = make([]DataSegment, dev.nchan)
		block.nSamp = int(header.Nsamp)
		block.err = err
		for i := 0; i < dev.nchan; i++ {
			raw := make([]RawType, header.Nsamp)
			for j := 0; j < int(header.Nsamp); j++ {
				raw[j] = data[i+dev.nchan*j]
			}
			block.segments[i] = DataSegment{
				rawData:         raw,
				signed:          true,
				framesPerSample: 1,
				firstFramenum:   FrameIndex(header.Sampnum),
				firstTime:       firstTime,
				framePeriod:     dev.period,
			}
		}
		nextBlock <- block
		if err != nil {
			return
		}
	}
	return
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
		err := device.samplePacket()
		if err != nil {
			return err
		}
		rs.nchan += device.nchan
	}

	return nil
}

// Delete closes all active RoachDevices.
func (rs *RoachSource) Delete() {
	rs.sourceStateLock.Lock()
	defer rs.sourceStateLock.Unlock()
	for _, device := range rs.active {
		device.conn.Close()
	}
	rs.active = make([]*RoachDevice, 0)
}

// StartRun tells the hardware to switch into data streaming mode.
// For ROACH µMUX systems, this is always happening. Then launch a goroutine to consume data.
func (rs *RoachSource) StartRun() error {
	// There's no data streaming mode on ROACH, so nothing to "switch on"
	go func() {
		defer close(rs.nextBlock)
		for {
			select {
			case <-rs.abortSelf:
				rs.Delete()
				return
			default:
				break
			}
		}
	}()
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
