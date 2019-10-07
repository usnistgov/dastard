package dastard

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
	"unsafe"
)

// RoachDevice represents a single ROACH device producing data by UDP packets
type RoachDevice struct {
	host   string        // in the form: "127.0.0.1:56789"
	rate   float64       // Sampling rate (not reported by the device)
	period time.Duration // Sampling period = 1/rate
	nchan  int
	conn   *net.UDPConn // active UDP connection
	nextS  FrameIndex
	unwrap []*PhaseUnwrapper
}

// RoachSource represents multiple ROACH devices
type RoachSource struct {
	Ndevices   int
	active     []*RoachDevice
	readPeriod time.Duration
	// buffersChan chan AbacoBuffersType
	AnySource
}

// PhaseUnwrapper makes phase values continous by adding integers as needed
type PhaseUnwrapper struct {
	lastVal   int16
	offset    int16
	highCount int
	lowCount  int
}

const roachScale RawType = 1 // How to scale the raw data in UnwrapInPlace

// UnwrapInPlace unwraps in place
func (u *PhaseUnwrapper) UnwrapInPlace(data *[]RawType, scale RawType) {

	// as read from the Roach
	// data bytes representing a 2s complement integer
	// where 2^14 is 1 phi0
	// so int(data[i])/2^14 is a number from -0.5 to 0.5 phi0
	// after this function we want 2^12 to be 1 phi0
	// 2^12 = 4096
	// 2^14 = 16384
	bitsToKeep := uint(14)
	bitsToShift := 16 - bitsToKeep
	onePi := int16(1) << (bitsToKeep - 3)
	twoPi := onePi << 1
	//phi0_lim := (4 * (1 << (16 - bits_to_keep)) / 2) - 1
	for i, rawVal := range *data {
		v := int16(rawVal*scale) >> bitsToShift // scale=2 for ABACO HACK!! FIX TO GENERALIZE
		delta := v - u.lastVal

		// short term unwrapping
		if delta > onePi {
			u.offset -= twoPi
		} else if delta < -onePi {
			u.offset += twoPi
		}

		// long term keeping baseline at same phi0
		// if the offset is nonzero for a long time, set it to zero
		if u.offset >= twoPi {
			u.highCount++
			u.lowCount = 0
		} else if u.offset <= -twoPi {
			u.lowCount++
			u.highCount = 0
		} else {
			u.lowCount = 0
			u.highCount = 0
		}
		if (u.highCount > 2000) || (u.lowCount > 2000) { // 2000 should be a setable parameter
			u.offset = 0
			u.highCount = 0
			u.lowCount = 0
		}
		(*data)[i] = RawType(v + u.offset)
		u.lastVal = v
	}
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
	conn.SetReadBuffer(100000000)
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
// raw data.
// TODO: verify that we don't need to add 0x8000 to convert signed->unsigned.
func parsePacket(packet []byte) (header packetHeader, data []RawType) {
	buf := bytes.NewReader(packet)
	if err := binary.Read(buf, binary.BigEndian, &header); err != nil {
		panic(fmt.Sprintln("binary.Read failed:", err))
	}
	data = make([]RawType, header.Nchan*header.Nsamp)
	wordLen := int(1 << (header.Flags & 0x3)) // data word length in bytes
	switch wordLen {
	case 2:
		if err := binary.Read(buf, binary.BigEndian, &data); err != nil {
			panic(fmt.Sprintln("binary.Read failed:", err))
		}
	case 4:
		// this just throws away two bytes from each 4 byte word
		data4 := make([]RawType, header.Nchan*header.Nsamp*2)
		if err := binary.Read(buf, binary.BigEndian, &data4); err != nil {
			panic(fmt.Sprintln("binary.Read failed:", err))
		}
		for i := range data {
			data[i] = data4[i*2]
		}
	default:
		panic(fmt.Sprintf("wordLen %v not implemented", wordLen))
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
	dev.unwrap = make([]*PhaseUnwrapper, dev.nchan)
	for i := range dev.unwrap {
		dev.unwrap[i] = &(PhaseUnwrapper{})
	}
	return err
}

// readPackets watches for UDP data from the Roach and sends it on chan nextBlock.
// One trick is that the UDP packets are small and can come many thousand per second.
// We should bundle these up into larger blocks and send these more like 10-100
// times per second.
func (dev *RoachDevice) readPackets(nextBlock chan *dataBlock) {
	// The packetBundleTime is how much data is bundled together before futher
	// processing. The packetKeepaliveTime is how long we wait for even one packet
	// before declaring the ROACH source dead.
	const packetBundleTime = 100 * time.Millisecond
	const packetKeepaliveTime = 2 * time.Second
	const packetMaxSize = 16384

	var err error
	keepAlive := time.Now().Add(packetKeepaliveTime)

	// Two loops:
	// Outer loop over larger blocks sent on nextBlock
	for {
		savedPackets := make([][]byte, 0, 100)

		// This deadline tells us when to stop collecting packets and bundle them
		deadline := time.Now().Add(packetBundleTime)
		if err = dev.conn.SetReadDeadline(deadline); err != nil {
			block := dataBlock{err: err}
			nextBlock <- &block
			return
		}

		// Inner loop over single UDP packets
		var readTime time.Time // Time of last packet read.
		for {
			p := make([]byte, packetMaxSize)
			_, _, err = dev.conn.ReadFromUDP(p)
			readTime = time.Now()

			// Handle the "normal error" of a timeout, then all other read errors
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				err = nil
				break
			} else if err != nil {
				block := dataBlock{err: err}
				nextBlock <- &block
				return
			}
			savedPackets = append(savedPackets, p)
		}
		// Bundling timeout expired. Were there were no data?
		if len(savedPackets) == 0 {
			if time.Now().After(keepAlive) {
				block := dataBlock{err: fmt.Errorf("ROACH source timed out after %v", packetKeepaliveTime)}
				nextBlock <- &block
				return
			}
			continue
		}

		keepAlive = time.Now().Add(packetKeepaliveTime)

		// Now process multiple packets into a dataBlock
		totalNsamp := 0
		allData := make([][]RawType, 0, len(savedPackets))
		nsamp := make([]int, 0, len(savedPackets))
		var firstFramenum FrameIndex
		for i, p := range savedPackets {
			header, data := parsePacket(p)
			if dev.nchan != int(header.Nchan) {
				err = fmt.Errorf("RoachDevice Nchan changed from %d -> %d", dev.nchan,
					header.Nchan)
			}
			if i == 0 {
				firstFramenum = FrameIndex(header.Sampnum)
			}
			allData = append(allData, data)
			nsamp = append(nsamp, int(header.Nsamp))
			totalNsamp += nsamp[i]
			ns := len(data) / dev.nchan
			if ns != nsamp[i] {
				fmt.Printf("Warning: block length=%d, want %d\n", len(data), dev.nchan*nsamp[i])
				fmt.Printf("header: %v, len(data)=%d\n", header, len(data))
				nsamp[i] = ns
			}
			// fmt.Println("i, header.Sampnum", i, header.Sampnum)
		}
		firstlastDelay := time.Duration(totalNsamp-1) * dev.period
		firstTime := readTime.Add(-firstlastDelay)
		block := new(dataBlock)
		block.segments = make([]DataSegment, dev.nchan)
		block.nSamp = totalNsamp
		block.err = err
		if firstFramenum != dev.nextS && dev.nextS > 0 {
			d := int(firstFramenum-dev.nextS) - totalNsamp
			warning := ""
			if d > 0 {
				warning = fmt.Sprintf("  **** %6d samples this block or %6d too few", totalNsamp, d)
			} else {
				warning = fmt.Sprintf("  **** %6d samples this block or %6d too many", totalNsamp, -d)
			}
			fmt.Printf("POTENTIAL DROPPED DATA: Sample %9d  Δs = %7d%s\n",
				firstFramenum, firstFramenum-dev.nextS, warning)
		}

		dev.nextS = firstFramenum + FrameIndex(totalNsamp)
		for i := 0; i < dev.nchan; i++ {
			raw := make([]RawType, block.nSamp)
			idx := 0
			for idxdata, data := range allData {
				for j := 0; j < nsamp[idxdata]; j++ {
					raw[idx+j] = data[i+dev.nchan*j]
				}
				idx += nsamp[idxdata]
			}
			unwrap := dev.unwrap[i]
			unwrap.UnwrapInPlace(&raw, roachScale)
			block.segments[i] = DataSegment{
				rawData:         raw,
				signed:          true,
				framesPerSample: 1,
				firstFramenum:   firstFramenum,
				firstTime:       firstTime,
				framePeriod:     dev.period,
			}
		}
		nextBlock <- block
		if err != nil {
			return
		}
	}
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
	rs.nchan = 0
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
	for _, device := range rs.active {
		device.conn.Close()
	}
	rs.active = make([]*RoachDevice, 0)
}

// StartRun tells the hardware to switch into data streaming mode.
// For ROACH µMUX systems, this is always happening. What we do have to do is to
// start 1 goroutine per UDP source to wait on the data and package it properly.
func (rs *RoachSource) StartRun() error {
	go func() {
		defer rs.Delete()
		defer close(rs.nextBlock)
		nextBlock := make(chan *dataBlock)
		for _, dev := range rs.active {
			go dev.readPackets(nextBlock)
		}

		lastHB := time.Now()
		totalBytes := 0
		for {
			select {
			case <-rs.abortSelf:
				return
			case block := <-nextBlock:
				now := time.Now()
				timeDiff := now.Sub(lastHB)
				totalBytes += block.nSamp * len(block.segments) * int(unsafe.Sizeof(RawType(0)))

				// Don't send heartbeats too often! Once per 100 ms only.
				if rs.heartbeats != nil && timeDiff > 100*time.Millisecond {
					rs.heartbeats <- Heartbeat{Running: true, DataMB: float64(totalBytes) / 1e6,
						Time: timeDiff.Seconds()}
					lastHB = now
					totalBytes = 0
				}
				rs.nextBlock <- block
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
	for i, dev := range rs.active {
		if i == 0 {
			rs.samplePeriod = dev.period
			rs.sampleRate = dev.rate
		} else if rs.samplePeriod != dev.period {
			return fmt.Errorf("Roach device %d period %v != device[0] period %v",
				i, dev.period, rs.samplePeriod)
		} else if rs.sampleRate != dev.rate {
			return fmt.Errorf("Roach device %d rate %v != device[0] rate %v",
				i, dev.rate, rs.sampleRate)
		}
	}
	return nil
}
