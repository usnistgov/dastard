package dastard

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/fabiokung/shm"
	"github.com/usnistgov/dastard/packets"
	"github.com/usnistgov/dastard/ringbuffer"
)

// GroupIndex represents the specifics of a channel group.
// It should be globally unique across all Abaco data.
type GroupIndex struct {
	firstchan int  // first channel number in this group
	nchan int // how many channels in this group
}

// ByGroup implements sort.Interface for GroupIndex slices
type ByGroup []GroupIndex
func (g ByGroup) Len() int { return len(g) }
func (g ByGroup) Swap(i, j int) { g[i], g[j] = g[j], g[i] }
func (g ByGroup) Less(i, j int) bool { return g[i].firstchan < g[j].firstchan }

type AbacoGroup struct {
	index GroupIndex
}

// AbacoRing represents a single shared-memory ring buffer
// that stores an Abaco card's data. (Possible in the future that multiple cards could
// pack data into the same ring, however.)
type AbacoRing struct {
	ringnum    int
	nchan      int
	cgroups    []GroupIndex // channel groups found on this device
	// queues     map[GroupIndex] []*packets.Packet
	firstchan  int
	packetSize int // packet size, in bytes
	ring       *ringbuffer.RingBuffer
	unwrap     []*PhaseUnwrapper
	sampleRate float64
}

const maxAbacoRings = 4 // Don't allow more than this many ring buffers.

const abacoFractionBits = 13
const abacoBitsToDrop = 1
// That is, Abaco data is of the form iii.bbbb bbbb bbbb b with 3 integer bits
// and 13 fractional bits. In the unwrapping process, we drop 1, making it 4/12.

// NewAbacoRing creates a new AbacoRing and opens the underlying shared memory for reading.
func NewAbacoRing(ringnum int) (dev *AbacoRing, err error) {
	// Allow negative ringnum values, but for testing only!
	if ringnum >= maxAbacoRings {
		return nil, fmt.Errorf("NewAbacoRing() got ringnum=%d, want [0,%d]",
			ringnum, maxAbacoRings-1)
	}
	dev = new(AbacoRing)
	dev.ringnum = ringnum

	shmNameBuffer := fmt.Sprintf("xdma%d_c2h_0_buffer", dev.ringnum)
	shmNameDesc := fmt.Sprintf("xdma%d_c2h_0_description", dev.ringnum)
	if dev.ring, err = ringbuffer.NewRingBuffer(shmNameBuffer, shmNameDesc); err != nil {
		return nil, err
	}
	return dev, nil
}

// ReadAllPackets returns an array of *packet.Packet, as read from the device's RingBuffer.
func (device *AbacoRing) ReadAllPackets() ([]*packets.Packet, error) {
	data, err := device.ring.ReadMultipleOf(device.packetSize)
	if err != nil {
		return nil, err
	}
	allPackets := make([]*packets.Packet, 0)
	reader := bytes.NewReader(data)
	for {
		p, err := packets.ReadPacketPlusPad(reader, device.packetSize)
		if err == io.EOF {
			break
		} else if err != nil {
			return allPackets, err
		}
		allPackets = append(allPackets, p)
	}
	return allPackets, nil
}

// samplePackets samples the data from a single AbacoRing. It scans enough packets to
// learn the number of channels, data rate, etc.
// Although it slows things down, it's best to discard all data in the ring when
// we open it, because we have no idea how old the data are (in particular, once full, a
// ring cannot take in new data to replace the old, so a full ring's data are arbitrarily old).
// TODO: consider whether the newest data a *non-full* ring can be used here?
func (device *AbacoRing) samplePackets() error {
	// Open the ring buffer and discard whatever is in it.
	if err := device.ring.Open(); err != nil {
		return err
	}
	psize, err := device.ring.PacketSize()
	if err != nil {
		return err
	}
	device.packetSize = int(psize)
	if err := device.ring.DiscardStride(uint64(device.packetSize)); err != nil {
		return err
	}

	// Now get the data we actually want: fresh data. Run for at least
	// a minimum time or a minimum number of packets.
	const minPacketsToRead = 100 // Not sure this is a good minimum?
	maxDelay := time.Duration(2000 * time.Millisecond)
	timeOut := time.NewTimer(maxDelay)

	// Capture timestamp and sample # for a range of packets. Use to find rate.
	var tsInit, tsFinal packets.PacketTimestamp
	var snInit, snFinal uint32
	samplesInPackets := 0

	allGroups := make(map[GroupIndex]bool)  // used as a set of GroupIndex objects
	packetsRead := 0
	packetReadLoop:
	for packetsRead < minPacketsToRead {
		select {
		case <-timeOut.C:
			fmt.Printf("AbacoRing.samplePackets() timer expired after %d packets read\n", packetsRead)
			break packetReadLoop

		default:
			time.Sleep(5 * time.Millisecond)
			allPackets, err := device.ReadAllPackets()
			if err != nil {
				fmt.Printf("Oh no! error in ReadAllPackets: %v\n", err)
				return err
			}
			packetsRead += len(allPackets)

			// Do something with Packet.ChannelInfo() here: track the GroupIndexs seen
			// and set device.nchan, device.firstchan based on the values here.
			for _, p := range allPackets {
				nchan, offset := p.ChannelInfo()
				g := GroupIndex{firstchan:offset, nchan:nchan}
				allGroups[g] = true

				samplesInPackets += p.Frames()
				if ts := p.Timestamp(); ts != nil && ts.Rate != 0 {
					if tsInit.T == 0 {
						tsInit.T = ts.T
						tsInit.Rate = ts.Rate
						snInit = p.SequenceNumber()
					}
					if tsFinal.T < ts.T {
						tsFinal.T = ts.T
						tsFinal.Rate = ts.Rate
						snFinal = p.SequenceNumber()
					}
				}
			}
		}
	}

	// Summarize all channelgroups and sort into device.cgroups
	device.nchan = 0
	for g := range allGroups {
		device.cgroups = append(device.cgroups, g)
		device.nchan += g.nchan
	}
	sort.Sort(ByGroup(device.cgroups))
	device.firstchan = device.cgroups[0].firstchan

	// Verify that no channel #s appear in 2 groups.
	highest := -1
	for i, g := range device.cgroups {
		if g.firstchan <= highest {
			return fmt.Errorf("Channel group %d=%v but previous range ended at %d", i, g, highest)
		}
		highest = g.firstchan + g.nchan - 1
	}

	// Use the first and last timestamp to compute sample rate.
	if tsInit.T != 0 && tsFinal.T != 0 {
		dt := float64(tsFinal.T-tsInit.T) / tsInit.Rate
		// TODO: check for wrap of timestamp if < 48 bits
		// TODO: what if ts.Rate changes between Init and Final?

		// Careful: assume that any missed packets had same number of samples as
		// the packets that we did see. Thus find the average samples per packet.
		dserial := snFinal - snInit
		if dserial > 0 {
			avgSampPerPacket := float64(samplesInPackets) / float64(packetsRead)
			device.sampleRate = float64(dserial) * avgSampPerPacket / dt
			fmt.Printf("Sample rate %.6g /sec determined from %d packets:\n\tΔt=%f sec, Δserial=%d, and %f samp/packet\n", device.sampleRate,
				packetsRead, dt, dserial, avgSampPerPacket)
		}
	}

	device.unwrap = make([]*PhaseUnwrapper, device.nchan)
	for i := range device.unwrap {
		device.unwrap[i] = NewPhaseUnwrapper(abacoFractionBits, abacoBitsToDrop)
	}
	return nil
}

// enumerateAbacoRings returns a list of abaco ring buffer numbers that exist
// in the devfs. If /dev/xdmaX_c2h_0_description exists, then X is added to the list.
// Does not handle cards with suffix other than *_c2h_0.
func enumerateAbacoRings() (devices []int, err error) {
	for cnum := 0; cnum < maxAbacoRings; cnum++ {
		name := fmt.Sprintf("xdma%d_c2h_0_description", cnum)
		if region, err := shm.Open(name, os.O_RDONLY, 0600); err == nil {
			region.Close()
			devices = append(devices, cnum)
		}
	}
	return devices, nil
}

// AbacoSource represents all AbacoRing ring buffers that can potentially supply data.
type AbacoSource struct {
	arings     map[int]*AbacoRing
	Nrings    int
	active      []*AbacoRing
	readPeriod  time.Duration
	buffersChan chan AbacoBuffersType
	AnySource
}

// NewAbacoSource creates a new AbacoSource.
func NewAbacoSource() (*AbacoSource, error) {
	source := new(AbacoSource)
	source.name = "Abaco"
	source.arings = make(map[int]*AbacoRing)

	deviceCodes, err := enumerateAbacoRings()
	if err != nil {
		return source, err
	}

	for _, cnum := range deviceCodes {
		ar, err := NewAbacoRing(cnum)
		if err != nil {
			log.Printf("warning: failed to create ring buffer for shm:xdma%d_c2h_0, though it should exist", cnum)
			continue
		}
		source.arings[cnum] = ar
		source.Nrings++
	}
	if source.Nrings == 0 && len(deviceCodes) > 0 {
		return source, fmt.Errorf("could not create ring buffer for any of shm:xdma*_c2h_0, though deviceCodes %v exist", deviceCodes)
	}
	return source, nil
}

// Delete closes the ring buffers for all AbacoRings.
func (as *AbacoSource) Delete() {
	for _, dev := range as.arings {
		dev.ring.Close()
	}
}

// AbacoSourceConfig holds the arguments needed to call AbacoSource.Configure by RPC.
type AbacoSourceConfig struct {
	ActiveCards    []int
	AvailableCards []int
}

// Configure sets up the internal buffers with given size, speed, and min/max.
func (as *AbacoSource) Configure(config *AbacoSourceConfig) (err error) {
	as.sourceStateLock.Lock()
	defer as.sourceStateLock.Unlock()
	// Update the slice AvailableCards.
	config.AvailableCards = make([]int, 0)
	for k := range as.arings {
		config.AvailableCards = append(config.AvailableCards, k)
	}
	sort.Ints(config.AvailableCards)

	if as.sourceState != Inactive {
		return fmt.Errorf("cannot Configure an AbacoSource if it's not Inactive")
	}

	// used to be sure the same device isn't listed twice in config.ActiveCards
	contains := func(s []*AbacoRing, e *AbacoRing) bool {
		for _, a := range s {
			if a == e {
				return true
			}
		}
		return false
	}

	// Activate the cards listed in the config request.
	as.active = make([]*AbacoRing, 0)
	for i, c := range config.ActiveCards {
		dev := as.arings[c]
		if dev == nil {
			err = fmt.Errorf("ActiveCards[%d]: card=%v, device == nil", i, c)
			break
		}
		if contains(as.active, dev) {
			err = fmt.Errorf("attempt to use same Abaco device two times: ActiveCards[%d], c=%v, config.ActiveCards=%v", i, c, config.ActiveCards)
			break
		}
		as.active = append(as.active, dev)
	}
	return
}

// Sample determines key data facts by sampling some initial data.
func (as *AbacoSource) Sample() error {
	as.nchan = 0
	if len(as.active) <= 0 {
		return fmt.Errorf("No Abaco ring buffers are active")
	}

	// Run device.samplePackets as goroutines on each device, in parallel, to save time.
	sampleErrors := make(chan error)
	for _, device := range as.active {
		go func(dev *AbacoRing) {
			sampleErrors <- dev.samplePackets()
		}(device)
	}
	for _ = range as.active {
		if err := <-sampleErrors; err != nil {
			return err
		}
	}
	for _, device := range as.active {
		as.nchan += device.nchan
	}

	// Treat devices as 1 row x N columns.
	as.rowColCodes = make([]RowColCode, as.nchan)
	i := 0
	for _, device := range as.active {
		as.sampleRate = device.sampleRate
		// TODO: what if multiple devices have unequal rates??
		for j := 0; j < device.nchan; j++ {
			as.rowColCodes[i] = rcCode(0, i, 1, as.nchan)
			i++
		}
	}

	as.samplePeriod = time.Duration(roundint(1e9 / as.sampleRate))

	return nil
}

// StartRun tells the hardware to switch into data streaming mode.
// For Abaco µMUX systems, we need to consume any initial data that constitutes
// a fraction of a frame. Then launch a goroutine to consume data.
func (as *AbacoSource) StartRun() error {
	// There's no data streaming mode on Abaco, so no need to start it?
	// Start by emptying all data from each device's ring buffer.
	for _, dev := range as.active {
		if err := dev.ring.DiscardStride(uint64(dev.packetSize)); err != nil {
			panic("AbacoRing.ring.DiscardStride failed")
		}
	}
	as.buffersChan = make(chan AbacoBuffersType, 100)
	as.readPeriod = 50 * time.Millisecond
	go as.readerMainLoop()
	return nil
}

// AbacoBuffersType is an internal message type used to allow
// a goroutine to read from the Abaco card and put data on a buffered channel
type AbacoBuffersType struct {
	datacopies     [][]RawType
	lastSampleTime time.Time
	timeDiff       time.Duration
	totalBytes     int
}

func (as *AbacoSource) readerMainLoop() {
	defer close(as.buffersChan)
	const timeoutPeriod = 5 * time.Second
	timeout := time.NewTimer(timeoutPeriod)
	ticker := time.NewTicker(as.readPeriod)
	defer ticker.Stop()
	defer timeout.Stop()
	as.lastread = time.Now()

	for {
		select {
		case <-as.abortSelf:
			log.Printf("Abaco read was aborted")
			return

		case <-timeout.C:
			// Handle failure to return
			log.Printf("Abaco read timed out")
			return

		case <-ticker.C:
			// read from the ring buffer
			// send bytes actually read on a channel
			framesUsed := 0
			totalBytes := 0
			datacopies := make([][]RawType, as.nchan)
			nchanPrevDevices := 0
			var lastSampleTime time.Time
			for _, dev := range as.active {
				allPackets, err := dev.ReadAllPackets()
				lastSampleTime = time.Now()
				if err != nil {
					fmt.Printf("AbacoRing.ReadAllPackets failed with error: %v\n", err)
					panic("AbacoRing.ReadAllPackets failed")
				}

				// Go through packets and figure out the # of frames contained in all packets.
				// We want this so we can make slices have the right capacity on creation.
				// TODO: this will break if channel offsets other than 0 are in the data.
				for _, p := range allPackets {
					switch d := p.Data.(type) {
					case []int32:
						framesUsed += len(d) / dev.nchan

					case []int16:
						framesUsed += len(d) / dev.nchan
						// fmt.Printf("Found [%d]int16 payload = %d frames: %v\n",
						// 	len(d), framesUsed, d[:5])

					default:
						panic("Cannot parse packets that aren't of type []int16 or []int32")
					}
				}

				// Demux data into this slice of slices of RawType (reserve capacity=framesUsed)
				for i := 0; i < dev.nchan; i++ {
					datacopies[i+nchanPrevDevices] = make([]RawType, 0, framesUsed)
				}

				// This is the demultiplexing step. Loops over packets, then values.
				// Within a slice of values, its all channels for frame 0, then all for frame 1...
				for _, p := range allPackets {
					nchan, offset := p.ChannelInfo()
					if offset < 0 || offset+nchan > dev.nchan {
						panic("Cannot handle packets with offset out of range")
						//continue
					}

					switch d := p.Data.(type) {
					case []int16:
						// Reading vector d in order was faster than the reverse, before packets:
						for j, val := range d {
							idx := (j % nchan) + offset + nchanPrevDevices
							datacopies[idx] = append(datacopies[idx], RawType(val))
						}
						totalBytes += 2 * len(d)

					case []int32:
						// TODO: We are squeezing the 16 bits higher than the lowest
						// 12 bits into the 16-bit datacopies[] slice. If we need a
						// permanent solution to 32-bit raw data, then it might need to be flexible
						// about _which_ 16 bits are kept and which discarded. (JF 3/7/2020).
						for j, val := range d {
							idx := (j % nchan) + offset + nchanPrevDevices
							datacopies[idx] = append(datacopies[idx], RawType(val/0x1000))
						}
						totalBytes += 4 * len(d)

					default:
						msg := fmt.Sprintf("Packets are of type %T, can only handle []int16 or []int32", p.Data)
						panic(msg)
					}
				}
			}
			timeDiff := lastSampleTime.Sub(as.lastread)
			if timeDiff > 2*as.readPeriod {
				fmt.Println("timeDiff in abaco reader", timeDiff)
			}
			as.lastread = lastSampleTime

			if len(as.buffersChan) == cap(as.buffersChan) {
				msg := fmt.Sprintf("internal buffersChan full, len %v, capacity %v", len(as.buffersChan), cap(as.buffersChan))
				fmt.Printf("Panic! %s\n", msg)
				panic(msg)
			}
			as.buffersChan <- AbacoBuffersType{
				datacopies:     datacopies,
				lastSampleTime: lastSampleTime,
				timeDiff:       timeDiff,
				totalBytes:     totalBytes,
			}
			if totalBytes > 0 {
				timeout.Reset(timeoutPeriod)
			}
		}
	}
}

// getNextBlock returns the channel on which data sources send data and any errors.
// More importantly, wait on this returned channel to await the source having a data block.
// This goroutine will end by putting a valid or error-ish dataBlock onto as.nextBlock.
// If the block has a non-nil error, this goroutine will also close as.nextBlock.
// The AbacoSource version also has to monitor the timeout channel and wait for
// the buffersChan to yield real, valid Abaco data.
// TODO: if there are any configuations that can change mid-run (analogous to Mix
// for Lancero), we'll also want to handle those changes in this loop.
func (as *AbacoSource) getNextBlock() chan *dataBlock {
	panicTime := time.Duration(cap(as.buffersChan)) * as.readPeriod
	go func() {
		for {
			select {
			case <-time.After(panicTime):
				panic(fmt.Sprintf("timeout, no data from Abaco after %v / readPeriod is %v", panicTime, as.readPeriod))

			case buffersMsg, ok := <-as.buffersChan:
				//  Check is buffersChan closed? Recognize that by receiving zero values and/or being drained.
				if buffersMsg.datacopies == nil || !ok {
					if err := as.closeDevices(); err != nil {
						block := new(dataBlock)
						block.err = err
						as.nextBlock <- block
					}
					close(as.nextBlock)
					return
				}

				// as.buffersChan contained valid data, so act on it.
				block := as.distributeData(buffersMsg)
				as.nextBlock <- block
				if block.err != nil {
					close(as.nextBlock)
				}
				return
			}
		}
	}()
	return as.nextBlock
}

func (as *AbacoSource) distributeData(buffersMsg AbacoBuffersType) *dataBlock {
	datacopies := buffersMsg.datacopies
	lastSampleTime := buffersMsg.lastSampleTime
	timeDiff := buffersMsg.timeDiff
	totalBytes := buffersMsg.totalBytes
	framesUsed := len(datacopies[0])

	// Backtrack to find the time associated with the first sample.
	segDuration := time.Duration(roundint((1e9 * float64(framesUsed-1)) / as.sampleRate))
	firstTime := lastSampleTime.Add(-segDuration)
	block := new(dataBlock)
	nchan := len(datacopies)
	block.segments = make([]DataSegment, nchan)

	// In the Lancero data this is where we scan for external triggers.
	// That doesn't exist yet in Abaco.

	// TODO: we should loop over devices here, matching devices to channels.
	dev := as.active[0]

	var wg sync.WaitGroup
	for channelIndex := 0; channelIndex < nchan; channelIndex++ {
		wg.Add(1)
		go func(channelIndex int) {
			defer wg.Done()
			data := datacopies[channelIndex]
			if dev != nil {
				unwrap := dev.unwrap[channelIndex]
				unwrap.UnwrapInPlace(&data)
			}
			seg := DataSegment{
				rawData:         data,
				framesPerSample: 1, // This will be changed later if decimating
				framePeriod:     as.samplePeriod,
				firstFramenum:   as.nextFrameNum,
				firstTime:       firstTime,
				signed:          true,
			}
			block.segments[channelIndex] = seg
			block.nSamp = len(data)
		}(channelIndex)
	}
	wg.Wait()
	as.nextFrameNum += FrameIndex(framesUsed)
	if as.heartbeats != nil {
		as.heartbeats <- Heartbeat{Running: true, DataMB: float64(totalBytes) / 1e6,
			Time: timeDiff.Seconds()}
	}
	now := time.Now()
	delay := now.Sub(lastSampleTime)
	if delay > 100*time.Millisecond {
		log.Printf("Buffer %v/%v, now-firstTime %v\n", len(as.buffersChan), cap(as.buffersChan), now.Sub(firstTime))
	}

	return block
}

// closeDevices ends closes the ring buffers of all active AbacoRing objects.
func (as *AbacoSource) closeDevices() error {
	// loop over as.active and do any needed stopping functions.
	for _, dev := range as.active {
		dev.ring.Close()
	}
	return nil
}
