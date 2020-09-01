package dastard

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/fabiokung/shm"
	"github.com/usnistgov/dastard/packets"
	"github.com/usnistgov/dastard/ringbuffer"
)

//------------------------------------------------------------------------------------------------

// GroupIndex represents the specifics of a channel group.
// It should be globally unique across all Abaco data.
type GroupIndex struct {
	firstchan int // first channel number in this group
	nchan     int // how many channels in this group
}

// gindex converts a packet to the GroupIndex whose data it contains
func gIndex(p *packets.Packet) GroupIndex {
	nchan, offset := p.ChannelInfo()
	return GroupIndex{nchan: nchan, firstchan: offset}
}

// ByGroup implements sort.Interface for []GroupIndex so we can sort such slices.
type ByGroup []GroupIndex

func (g ByGroup) Len() int           { return len(g) }
func (g ByGroup) Swap(i, j int)      { g[i], g[j] = g[j], g[i] }
func (g ByGroup) Less(i, j int) bool { return g[i].firstchan < g[j].firstchan }

//------------------------------------------------------------------------------------------------

// AbacoGroup represents a channel group, a set of consecutively numbered
// channels.
type AbacoGroup struct {
	index      GroupIndex
	nchan      int
	sampleRate float64
	unwrap     []*PhaseUnwrapper
	queue      []*packets.Packet
	lasttime   time.Time
	seqnumsync uint32 // Global sequence number is referenced to this group's seq number at seqnumsync
	lastSN     uint32
}

// NewAbacoGroup creates an AbacoGroup given the specified GroupIndex.
func NewAbacoGroup(index GroupIndex) *AbacoGroup {
	g := new(AbacoGroup)
	g.index = index
	g.nchan = index.nchan
	g.queue = make([]*packets.Packet, 0)
	g.unwrap = make([]*PhaseUnwrapper, g.nchan)
	for i := range g.unwrap {
		g.unwrap[i] = NewPhaseUnwrapper(abacoFractionBits, abacoBitsToDrop)
	}
	return g
}

func (group *AbacoGroup) enqueuePacket(p *packets.Packet, now time.Time) {
	group.queue = append(group.queue, p)
	group.lasttime = now
}

func (group *AbacoGroup) samplePackets() error {
	// Capture timestamp and sample # for a range of packets. Use to find rate.
	var tsInit, tsFinal packets.PacketTimestamp
	var snInit, snFinal uint32
	samplesInPackets := 0

	for _, p := range group.queue {
		cidx := gIndex(p)
		if cidx != group.index {
			return fmt.Errorf("Packet with index %v got to group with index %v", cidx, group.index)
		}

		samplesInPackets += p.Frames()
		if ts := p.Timestamp(); ts != nil && ts.Rate != 0 {
			if tsInit.T == 0 {
				tsInit.T = ts.T
				tsInit.Rate = ts.Rate
				snInit = p.SequenceNumber()
				// For now: sync by assuming first packet seen by each group is simultaneous
				// TODO: eventually want to sync to the _sample_ level, not the packet level.
				group.seqnumsync = snInit
			}
			if tsFinal.T < ts.T {
				tsFinal.T = ts.T
				tsFinal.Rate = ts.Rate
				snFinal = p.SequenceNumber()
			}
		}
	}
	group.lastSN = group.queue[len(group.queue)-1].SequenceNumber()

	// Use the first and last timestamp to compute sample rate.
	if tsInit.T != 0 && tsFinal.T != 0 {
		dt := float64(tsFinal.T-tsInit.T) / tsInit.Rate
		// TODO: check for wrap of timestamp if < 48 bits
		// TODO: what if ts.Rate changes between Init and Final?

		// Careful: assume that any missed packets had same number of samples as
		// the packets that we did see. Thus find the average samples per packet.
		dserial := snFinal - snInit
		packetsRead := len(group.queue)
		if dserial > 0 {
			avgSampPerPacket := float64(samplesInPackets) / float64(packetsRead)
			group.sampleRate = float64(dserial) * avgSampPerPacket / dt
			fmt.Printf("Sample rate %.6g /sec determined from %d packets:\n\tΔt=%f sec, Δserial=%d, and %f samp/packet\n", group.sampleRate,
				packetsRead, dt, dserial, avgSampPerPacket)
		}
	}

	// Clear out the queue of packets
	group.queue = group.queue[:0]
	return nil
}

// firstSeqNum returns the global sequence number for the first packet in the group's packet queue,
// meaning the sequence number referenced to the group.seqnumsync offset.
func (group *AbacoGroup) firstSeqNum() (uint32, error) {
	if len(group.queue) == 0 {
		return 0, fmt.Errorf("No packets in queue")
	}
	return group.queue[0].SequenceNumber() - group.seqnumsync, nil
}

// fillMissingPackets looks for holes in the sequence numbers in group.queue, and replaces them with
// pretend packets.
func (group *AbacoGroup) fillMissingPackets() (numberAdded int) {
	if len(group.queue) == 0 {
		return
	}
	cap := group.queue[len(group.queue)-1].SequenceNumber() - group.lastSN
	newq := make([]*packets.Packet, 0, cap)
	snexpect := group.lastSN + 1
	for _, p := range group.queue {
		sn := p.SequenceNumber()
		for snexpect < sn {
			pfake := p.MakePretendPacket(snexpect, group.nchan)
			newq = append(newq, pfake)
			numberAdded++
			snexpect++
		}
		newq = append(newq, p)
		snexpect++
	}
	if numberAdded > 0 {
		group.queue = newq
	}
	group.lastSN = group.queue[len(group.queue)-1].SequenceNumber()
	// fmt.Printf("fillMissingPackets added %d packets to make queue size %d\n", numberAdded, len(group.queue))
	return
}

// trimPacketsBefore removes leading packets from the queue that "predate" firstSn
// firstSn is a "global sequence number", thus relative to the group.seqnumsync
func (group *AbacoGroup) trimPacketsBefore(firstSn uint32) {
	firstSn += group.seqnumsync
	for {
		sn0 := group.queue[0].SequenceNumber()
		if sn0 >= firstSn {
			return
		}
		group.queue = group.queue[1:]
		if len(group.queue) == 0 {
			return
		}
	}
}

// countSamplesInQueue counts the samples per channel in all packets in the group.queue.
func (group *AbacoGroup) countSamplesInQueue() int {
	// Go through packets and figure out the # of samples contained in all packets.
	valuesFound := 0
	for _, p := range group.queue {
		switch d := p.Data.(type) {
		case []int32:
			valuesFound += len(d)

		case []int16:
			valuesFound += len(d)

		default:
			panic("Cannot parse packets that aren't of type []int16 or []int32")
		}
	}
	return valuesFound / group.nchan
}

// demuxData demultiplexes the data in the packet queue into the datacopies slices (1 slice per channel)
// but only up to the requested number of frames.
func (group *AbacoGroup) demuxData(datacopies [][]RawType, frames int) int {
	// This is the demultiplexing step. Loops over packets, then values.
	// Within a slice of values, handle all channels for frame 0, then all for frame 1...
	totalBytes := 0
	packetsConsumed := 0
	for _, p := range group.queue {
		gidx := gIndex(p)
		if gidx != group.index {
			msg := fmt.Sprintf("Group %v received packet for index %v", group.index, gidx)
			panic(msg)
		}

		// Don't demux a packet that would over-fill the requested # of frames.
		frameAvail := p.Frames()
		if frameAvail > frames {
			break
		}

		frames -= frameAvail
		packetsConsumed++
		switch d := p.Data.(type) {
		case []int16:
			// Reading vector d in order was faster than the reverse.
			for j, val := range d {
				idx := j % group.nchan
				datacopies[idx] = append(datacopies[idx], RawType(val))
			}
			totalBytes += 2 * len(d)

		case []int32:
			// TODO: We are squeezing the 16 bits higher than the lowest
			// 12 bits into the 16-bit datacopies[] slice. If we need a
			// permanent solution to 32-bit raw data, then it might need to be flexible
			// about _which_ 16 bits are kept and which discarded. (JF 3/7/2020).
			for j, val := range d {
				idx := j % group.nchan
				datacopies[idx] = append(datacopies[idx], RawType(val/0x1000))
			}
			totalBytes += 4 * len(d)

		default:
			msg := fmt.Sprintf("Packets are of type %T, can only handle []int16 or []int32", p.Data)
			panic(msg)
		}
	}
	// fmt.Printf("Consumed %d of %d packets\n", packetsConsumed, len(group.queue))
	group.queue = group.queue[packetsConsumed:]

	// TODO: What if the # of frames points to the middle of a packet? For now, panic.
	if frames > 0 {
		msg := fmt.Sprintf("Consumed %d of %d packets, but there are still %d frames to fill\n",
			packetsConsumed, len(group.queue), frames)
		panic(msg)
	}

	for i, unwrap := range group.unwrap {
		unwrap.UnwrapInPlace(&datacopies[i])
	}

	return totalBytes
}

//------------------------------------------------------------------------------------------------

// AbacoRing represents a single shared-memory ring buffer that stores an Abaco card's data.
// Beware of a possible future: multiple cards could pack data into the same ring.
type AbacoRing struct {
	ringnum    int
	packetSize int // packet size, in bytes
	ring       *ringbuffer.RingBuffer
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
func (device *AbacoRing) samplePackets() (allPackets []*packets.Packet, err error) {
	// Open the ring buffer and discard whatever is in it.
	if err = device.ring.Open(); err != nil {
		return
	}
	psize, err := device.ring.PacketSize()
	if err != nil {
		return
	}
	device.packetSize = int(psize)
	if err = device.ring.DiscardStride(uint64(device.packetSize)); err != nil {
		return
	}

	// Now get the data we actually want: fresh data. Run for at least
	// a minimum time or a minimum number of packets.
	// The hard requirement is for at least 2 packets per channel group, so we can count samples taken
	// between 2 timestamps and thus the sample rate. More packets is better, because we can estimate
	// sample rate over a longer baseline, we are less likely to overlook a channel group.
	const minPacketsToRead = 100 // Is this a good minimum? If we start missing channel groups, increase this.
	maxSampleTime := time.Duration(2000 * time.Millisecond)
	timeOut := time.NewTimer(maxSampleTime)

	packetsRead := 0
	for packetsRead < minPacketsToRead {
		select {
		case <-timeOut.C:
			fmt.Printf("AbacoRing.samplePackets() timer expired after only %d packets read\n", packetsRead)
			return

		default:
			time.Sleep(5 * time.Millisecond)
			p, err := device.ReadAllPackets()
			if err != nil {
				fmt.Printf("Oh no! error in ReadAllPackets: %v\n", err)
				return allPackets, err
			}
			allPackets = append(allPackets, p...)
			packetsRead = len(allPackets)
		}
	}
	return allPackets, nil
}

// enumerateAbacoRings returns a list of abaco ring buffer numbers that exist
// in the devfs. If /dev/xdmaX_c2h_0_description exists, then X is added to the list.
// Does not handle cards with suffix other than *_c2h_0.
func enumerateAbacoRings() (rings []int, err error) {
	for cnum := 0; cnum < maxAbacoRings; cnum++ {
		name := fmt.Sprintf("xdma%d_c2h_0_description", cnum)
		if region, err := shm.Open(name, os.O_RDONLY, 0600); err == nil {
			region.Close()
			rings = append(rings, cnum)
		}
	}
	return rings, nil
}

//------------------------------------------------------------------------------------------------

// AbacoSource represents all AbacoRing ring buffers that can potentially supply data, as well
// as all AbacoGroups that are discovered in the SampleData phase.
type AbacoSource struct {
	Nrings int // number of available ring buffers
	arings map[int]*AbacoRing
	active []*AbacoRing

	groups          map[GroupIndex]*AbacoGroup
	groupKeysSorted []GroupIndex

	readPeriod  time.Duration
	buffersChan chan AbacoBuffersType
	AnySource
}

// NewAbacoSource creates a new AbacoSource.
func NewAbacoSource() (*AbacoSource, error) {
	source := new(AbacoSource)
	source.name = "Abaco"
	source.arings = make(map[int]*AbacoRing)
	source.groups = make(map[GroupIndex]*AbacoGroup)

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

// distributePackets sorts a slice of Abaco packets into the data queues according to the GroupIndex.
func (as *AbacoSource) distributePackets(allpackets []*packets.Packet, now time.Time) {
	for _, p := range allpackets {
		cidx := gIndex(p)
		as.groups[cidx].enqueuePacket(p, now)
	}
}

// Sample determines key data facts by sampling some initial data.
func (as *AbacoSource) Sample() error {
	if len(as.active) <= 0 {
		return fmt.Errorf("No Abaco ring buffers are active")
	}

	// Launch device.samplePackets as goroutines on each device, in parallel, to save time.
	type SampleResult struct {
		allpackets []*packets.Packet
		err        error
	}
	sampleResults := make(chan SampleResult)
	for _, ring := range as.active {
		go func(r *AbacoRing) {
			p, err := r.samplePackets()
			sampleResults <- SampleResult{allpackets: p, err: err}
		}(ring)
	}

	// Now sort the packets received into the right AbacoGroups
	as.nchan = 0
	for _ = range as.active {
		results := <-sampleResults
		now := time.Now()
		if results.err != nil {
			return results.err
		}
		// Create new AbacoGroup for each GroupIndex seen
		for _, p := range results.allpackets {
			cidx := gIndex(p)
			if _, ok := as.groups[cidx]; !ok {
				as.groups[cidx] = NewAbacoGroup(cidx)
				as.nchan += cidx.nchan
			}
		}
		as.distributePackets(results.allpackets, now)
	}

	// Verify that no channel # appears in 2 groups.
	known := make(map[int]bool)
	for _, g := range as.groups {
		cinit := g.index.firstchan
		cend := cinit + g.index.nchan
		for cnum := cinit; cnum < cend; cnum++ {
			if known[cnum] {
				return fmt.Errorf("Channel group %v sees channel %d, which was in another group", g.index, cnum)
			}
			known[cnum] = true
		}
	}

	// Compute a fixed ordering for iteration over the map as.groups
	var keys []GroupIndex
	for k := range as.groups {
		keys = append(keys, k)
	}
	sort.Sort(ByGroup(keys))
	as.groupKeysSorted = keys

	// Each AbacoGroup should process its sampled packets.
	for _, group := range as.groups {
		group.samplePackets()
	}

	// Treat groups as 1 row x N columns.
	as.rowColCodes = make([]RowColCode, as.nchan)
	i := 0
	for _, group := range as.groups {
		if as.sampleRate == 0 {
			as.sampleRate = group.sampleRate
		}
		if as.sampleRate != group.sampleRate {
			fmt.Printf("Oh crap! Two groups have different sample rates: %f, %f", as.sampleRate, group.sampleRate)
			panic("Oh crap! Two groups have different sample rates.")
			// TODO: what if multiple groups have unequal rates??
		}
		for j := 0; j < group.nchan; j++ {
			as.rowColCodes[i] = rcCode(0, i, 1, group.nchan)
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

awaitmoredata:
	for {
		select {
		case <-as.abortSelf:
			log.Printf("Abaco read was aborted")
			return

		case <-timeout.C:
			// Handle failure to return
			log.Printf("Abaco read timed out after %v", timeoutPeriod)
			return

		case <-ticker.C:
			// read from the ring buffer
			var lastSampleTime time.Time
			for _, ring := range as.active {
				allPackets, err := ring.ReadAllPackets()
				lastSampleTime = time.Now()
				if err != nil {
					fmt.Printf("AbacoRing.ReadAllPackets failed with error: %v\n", err)
					panic("AbacoRing.ReadAllPackets failed")
				}
				as.distributePackets(allPackets, lastSampleTime)
			}

			// Align the first sample in each group. Do this by checking the first global sequenceNumber
			// in each group and trimming leading packets if any precede the others.
			// Fill in for any missing packets first, so a missing first packet isn't a problem.
			firstSn := uint32(0)
			for idx, group := range as.groups {
				numberAdded := group.fillMissingPackets()
				if numberAdded > 0 && as.problemLogger != nil {
					cfirst := idx.firstchan
					clast := cfirst + idx.nchan - 1
					as.problemLogger.Printf("AbacoGroup %v=channels [%d,%d] filled in %d missing packets", idx,
						cfirst, clast, numberAdded)
				}
				sn0, err := group.firstSeqNum()
				if err != nil { // That is, no data available from this group
					continue awaitmoredata
				}
				if sn0 > firstSn {
					firstSn = sn0
				}
			}

			// For any queue that starts before maxsn0, trim the leading packets. Learn the # of samples.
			framesToDeMUX := math.MaxInt64
			for _, group := range as.groups {
				group.trimPacketsBefore(firstSn)
				nsamp := group.countSamplesInQueue()
				if nsamp < framesToDeMUX {
					framesToDeMUX = nsamp
				}
			}
			if framesToDeMUX <= 0 {
				continue awaitmoredata
			}

			// Demux data into this slice of slices of RawType (reserve capacity=framesToDeMUX)
			datacopies := make([][]RawType, as.nchan)
			for i := 0; i < as.nchan; i++ {
				datacopies[i] = make([]RawType, 0, framesToDeMUX)
			}
			chanProcessed := 0
			bytesProcessed := 0
			for _, k := range as.groupKeysSorted {
				group := as.groups[k]
				dc := datacopies[chanProcessed : chanProcessed+group.nchan]
				bytesProcessed += group.demuxData(dc, framesToDeMUX)
				chanProcessed += group.nchan
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
				totalBytes:     bytesProcessed,
			}
			if bytesProcessed > 0 {
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
					if err := as.closeRings(); err != nil {
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
	// dev := as.active[0]

	var wg sync.WaitGroup
	for channelIndex := 0; channelIndex < nchan; channelIndex++ {
		wg.Add(1)
		go func(channelIndex int) {
			defer wg.Done()
			data := datacopies[channelIndex]
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

// closeRings ends closes the ring buffers of all active AbacoRing objects.
func (as *AbacoSource) closeRings() error {
	// loop over as.active and do any needed stopping functions.
	for _, dev := range as.active {
		dev.ring.Close()
	}
	return nil
}
