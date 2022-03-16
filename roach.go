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
	AnySource
}

const roachFractionBits = 14
const roachBitsToDrop = 2

// That is, ROACH data is of the form ii.bbbb bbbb bbbb bb with 2 integer bits
// and 14 fractional bits. In the unwrapping process, we drop 2, making it 4/12.

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
		// Throw away the 2 least-significant bytes from each 4 byte word.
		// Because binary.Read does the big->little endian swapping, but only
		// within the 16-bit words of data4[:], we have data4[0] representing
		// the most significant 16 bits of 32, and data4[1] the least.
		data4 := make([]RawType, header.Nchan*header.Nsamp*2)
		if err := binary.Read(buf, binary.BigEndian, &data4); err != nil {
			panic(fmt.Sprintln("binary.Read failed:", err))
		}
		for i := range data {
			data[i] = data4[i*2]
		}
	default:
		msg := fmt.Sprintf("wordLen %v not implemented. Header: %v", wordLen, header)
		panic(msg)
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
	dev.nextS = FrameIndex(header.Nsamp) + FrameIndex(header.Sampnum)
	dev.nchan = int(header.Nchan)
	dev.unwrap = make([]*PhaseUnwrapper, dev.nchan)
	biaslevel := 0
	pulseSign := +1
	for i := range dev.unwrap {
		const enable = true
		const resetAfter = 20000
		dev.unwrap[i] = NewPhaseUnwrapper(roachFractionBits, roachBitsToDrop, enable, biaslevel, resetAfter, pulseSign)
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
		var firstFrameIndex FrameIndex
		for i, p := range savedPackets {
			header, data := parsePacket(p)
			if dev.nchan != int(header.Nchan) {
				err = fmt.Errorf("RoachDevice Nchan changed from %d -> %d", dev.nchan,
					header.Nchan)
			}
			if i == 0 {
				firstFrameIndex = FrameIndex(header.Sampnum)
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
		}
		firstlastDelay := time.Duration(totalNsamp-1) * dev.period
		firstTime := readTime.Add(-firstlastDelay)
		block := new(dataBlock)
		block.segments = make([]DataSegment, dev.nchan)
		block.nSamp = totalNsamp
		block.err = err
		if firstFrameIndex != dev.nextS && dev.nextS > 0 {
			d := int(firstFrameIndex-dev.nextS) - totalNsamp
			warning := ""
			if d > 0 {
				warning = fmt.Sprintf(" **** %6d samples this block or %6d too few", totalNsamp, d)
			} else {
				warning = fmt.Sprintf(" **** %6d samples this block or %6d too many", totalNsamp, -d)
			}
			fmt.Printf("POTENTIAL DROPPED DATA: Sample %9d  Δs = %7d (want 0) %s\n",
				firstFrameIndex, firstFrameIndex-dev.nextS, warning)
		}

		dev.nextS = firstFrameIndex + FrameIndex(totalNsamp)
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
			unwrap.UnwrapInPlace(&raw)
			block.segments[i] = DataSegment{
				rawData:         raw,
				signed:          true,
				framesPerSample: 1,
				firstFrameIndex: firstFrameIndex,
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
					mb := float64(totalBytes) / 1e6
					rs.heartbeats <- Heartbeat{Running: true, HWactualMB: mb, DataMB: mb,
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

// PrepareChannels configures a RoachSource by initializing all data structures that
// have to do with channels and their naming/numbering.
func (rs *RoachSource) PrepareChannels() error {
	rs.channelsPerPixel = 1

	// Fill the channel names and numbers slices, treating source as a single channel group.
	// Number channels starting at zero.
	rs.chanNames = make([]string, 0, rs.nchan)
	rs.chanNumbers = make([]int, 0, rs.nchan)
	rs.rowColCodes = make([]RowColCode, 0, rs.nchan)
	rs.groupKeysSorted = make([]GroupIndex, 0)
	ncol := 1
	nrow := rs.nchan
	col := 0
	for row := 0; row < rs.nchan; row++ {
		channum := row
		name := fmt.Sprintf("chan%d", channum)
		rs.chanNames = append(rs.chanNames, name)
		rs.chanNumbers = append(rs.chanNumbers, channum)
		rs.rowColCodes = append(rs.rowColCodes, rcCode(row, col, nrow, ncol))
	}
	rs.groupKeysSorted = append(rs.groupKeysSorted, GroupIndex{Firstchan: 0, Nchan: nrow})

	return nil
}
