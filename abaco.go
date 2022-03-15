package dastard

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/fabiokung/shm"
	"github.com/usnistgov/dastard/packets"
	"github.com/usnistgov/dastard/ringbuffer"
)

const abacoFractionBits = 16 // changed from 13 to 16 in Jan 2021.
const abacoBitsToDrop = 4

// That is, Abaco data is of the form bbbb bbbb bbbb bbbb with 0 integer bits
// and 16 fractional bits. In the unwrapping process, we drop 4, making it 4/12, or
// iiii.bbbb bbbb bbbb. This gives room for up to ±8ϕ0 (actually, -8ϕ0 and +8ϕ0 both map to 0x8000).

// gindex converts a packet to the GroupIndex whose data it contains
func gIndex(p *packets.Packet) GroupIndex {
	nchan, offset := p.ChannelInfo()
	return GroupIndex{Firstchan: offset, Nchan: nchan}
}

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
func NewAbacoGroup(index GroupIndex, unwrapEnable, unwrapBias bool, unwrapReset, pulseSign int) *AbacoGroup {
	g := new(AbacoGroup)
	g.index = index
	g.nchan = index.Nchan
	g.queue = make([]*packets.Packet, 0)
	g.unwrap = make([]*PhaseUnwrapper, g.nchan)
	var bias int
	if unwrapBias {
		// Assume 2^16 equals exactly one ϕ0, then bias based on the assumption of critically
		// damped pulses. In that case, the largest falling and rising slopes are in the ratio
		// -0.12 to +0.88. Hence, a bias of ±0.38*ϕ0 (sign given by the pulseSign).
		bias = int(math.Round(+0.38 * 65536))
		if pulseSign < 0 {
			bias = -bias
		}
	}
	for i := range g.unwrap {
		g.unwrap[i] = NewPhaseUnwrapper(abacoFractionBits, abacoBitsToDrop, unwrapEnable, bias, unwrapReset, pulseSign)
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
			return fmt.Errorf("packet with index %v got to group with index %v", cidx, group.index)
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
		return 0, fmt.Errorf("no packets in queue")
	}
	return group.queue[0].SequenceNumber() - group.seqnumsync, nil
}

// fillMissingPackets looks for holes in the sequence numbers in group.queue, and replaces them with
// pretend packets.
func (group *AbacoGroup) fillMissingPackets() (bytesAdded, packetsAdded, framesAdded int) {
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
			packetsAdded++
			framesAdded += p.Frames()
			bytesAdded += p.Length()
			snexpect++
		}
		newq = append(newq, p)
		snexpect++
	}
	if packetsAdded > 0 {
		group.queue = newq
	}
	group.lastSN = group.queue[len(group.queue)-1].SequenceNumber()
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
	samplesConsumed := 0
	nchan := group.nchan

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
			// Reading vector d in channel order was faster than in packet-data order.
			nsamp := len(d) / nchan
			for idx, dc := range datacopies {
				for i, j := 0, idx; i < nsamp; i++ {
					dc[i+samplesConsumed] = RawType(d[j])
					j += nchan
				}
			}
			samplesConsumed += nsamp
			totalBytes += 2 * len(d)

		case []int32:
			// TODO: We are squeezing the 16 highest bits into the 16-bit datacopies[] slice.
			// This was changed Jan 2021 (issue 227). It was previously the 16 lowest above the 12 lowest.
			// If we need a permanent solution to 32-bit raw data, then it might need to be flexible
			// about _which_ 16 bits are kept and which discarded. (JF 3/7/2020).

			nsamp := len(d) / nchan
			for idx, dc := range datacopies {
				for i, j := 0, idx; i < nsamp; i++ {
					dc[i+samplesConsumed] = RawType(d[j] / 0x10000)
					j += nchan
				}
			}
			samplesConsumed += nsamp
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

	// Apply phase unwrapping to each channel's data. Do in parallel to use multiple processors.
	var wg sync.WaitGroup
	for i, unwrapper := range group.unwrap {
		wg.Add(1)
		go func(up *PhaseUnwrapper, dc *[]RawType) {
			defer wg.Done()
			up.UnwrapInPlace(dc)
		}(unwrapper, &datacopies[i])
	}
	wg.Wait()

	return totalBytes
}

//------------------------------------------------------------------------------------------------

// PacketProducer is the interface for data sources that produce packets.
// Implementations wrap ring buffers and UDP servers.
type PacketProducer interface {
	ReadAllPackets() ([]*packets.Packet, error)
	samplePackets(d time.Duration) ([]*packets.Packet, error)
	start() error
	discardStale() error
	stop() error
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

// start starts the ring buffer by opening it and discarding whatever is in it.
func (device *AbacoRing) start() (err error) {
	if err = device.ring.Open(); err != nil {
		return
	}
	psize, err := device.ring.PacketSize()
	if err != nil {
		return
	}
	device.packetSize = int(psize)
	return device.discardStale()
}

// discardStale discards all data currently sitting in the ring.
// TODO: consider whether the newest data a *non-full* ring can be used here?
func (device *AbacoRing) discardStale() error {
	return device.ring.DiscardStride(uint64(device.packetSize))
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
func (device *AbacoRing) samplePackets(maxSampleTime time.Duration) (allPackets []*packets.Packet, err error) {
	// Run for at least a minimum time or a minimum number of packets.
	// The hard requirement is for at least 2 packets per channel group, so we can count samples taken
	// between 2 timestamps and thus the sample rate. More packets is better, because we can estimate
	// sample rate over a longer baseline, we are less likely to overlook a channel group.
	const minPacketsToRead = 100 // Is this a good minimum? If we start missing channel groups, increase this.
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

// stop closes the ring buffer
func (device *AbacoRing) stop() error {
	return device.ring.Close()
}

// enumerateAbacoRings returns a list of abaco ring buffer numbers that exist
// in the shared memory system. If xdmaX_c2h_0_description exists, then X is added to the list.
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

// AbacoUDPReceiver represents a single Abaco device producing data by UDP packets to a single UDP port.
type AbacoUDPReceiver struct {
	host     string       // in the form: "127.0.0.1:56789"
	conn     *net.UDPConn // active UDP connection
	data     chan []*packets.Packet
	sendmore chan bool
}

// NewAbacoUDPReceiver creates a new AbacoUDPReceiver and binds as a server to the requested host:port
func NewAbacoUDPReceiver(hostport string) (dev *AbacoUDPReceiver, err error) {
	dev = new(AbacoUDPReceiver)
	dev.host = hostport
	if _, err := net.ResolveUDPAddr("udp", hostport); err != nil {
		return nil, err
	}
	return dev, nil
}

// start starts the UDP device by...?
func (device *AbacoUDPReceiver) start() (err error) {
	raddr, err := net.ResolveUDPAddr("udp", device.host)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", raddr)
	if err != nil {
		return err
	}
	const UDPPacketSize = 8192
	bufferPool := sync.Pool{
		New: func() interface{} { return make([]byte, UDPPacketSize) },
	}
	device.conn = conn

	device.sendmore = make(chan bool)
	device.data = make(chan []*packets.Packet)
	singlepackets := make(chan []byte, 20)
	// This goroutine handles UDP message sent one at a time on singlepackets by the other goroutine.
	// It converts them into packet.Packet objects, queues them into a slice of *packets.Packet
	// pointers and sends the whole slice when a request comes on device.sendmore.
	go func() {
		defer close(device.data)
		defer func() { device.conn = nil }()

		const initialQueueCapacity = 2048
		queue := make([]*packets.Packet, 0, initialQueueCapacity)
		for {
			select {
			case _, ok := <-device.sendmore:
				device.data <- queue
				if !ok {
					return
				}
				queue = make([]*packets.Packet, 0, initialQueueCapacity)

			case message := <-singlepackets:
				p, err := packets.ReadPacket(bytes.NewReader(message))
				bufferPool.Put(message)
				if err != nil {
					if err != io.EOF {
						fmt.Printf("Error converting UDP to packet: err %v, packet %v\n", err, p)
					}
					return
				}
				queue = append(queue, p)
			}
		}
	}()
	// This goroutine reads the UDP socket and puts the resulting packets on channel singlepackets.
	// They will be removed and queued by the previous goroutine.
	go func() {
		defer close(singlepackets)
		for {
			message := bufferPool.Get().([]byte)
			if _, _, err := device.conn.ReadFrom(message); err != nil {
				// Getting an error here is the normal way to detect closed connection.
				bufferPool.Put(message)
				return
			}
			singlepackets <- message
		}
	}()
	return nil
}

// discardStale discards all data currently ready for reading at the UDP socket.
// At least, it is supposed to. We are not sure it actually works.
func (device *AbacoUDPReceiver) discardStale() error {
	device.conn.SetReadBuffer(0)
	device.conn.SetReadBuffer(67108864)
	return nil
}

// ReadAllPackets returns an array of *packet.Packet, as read from the device's network connection.
// It is a non-blocking wrapper around the UDP connection read (a blocking operation).
func (device *AbacoUDPReceiver) ReadAllPackets() ([]*packets.Packet, error) {
	device.sendmore <- true
	allPackets, ok := <-device.data
	if !ok {
		fmt.Printf("device.data closed after %d packets\n", len(allPackets))
	}
	return allPackets, nil
}

// samplePackets samples the data from a single AbacoUDPReceiver. It scans enough packets to
// learn the number of channels, data rate, etc.
func (device *AbacoUDPReceiver) samplePackets(maxSampleTime time.Duration) (allPackets []*packets.Packet, err error) {
	// Run for at least a minimum time or a minimum number of packets.
	// The hard requirement is for at least 2 packets per channel group, so we can count samples taken
	// between 2 timestamps and thus the sample rate. More packets is better, because we can estimate
	// sample rate over a longer baseline, we are less likely to overlook a channel group.
	const minPacketsToRead = 100 // Is this a good minimum? If we start missing channel groups, increase this.
	timeOut := time.NewTimer(maxSampleTime)

	packetsRead := 0
	for packetsRead < minPacketsToRead {
		select {
		case <-timeOut.C:
			fmt.Printf("AbacoUDPReceiver.samplePackets() timer expired after only %d packets read\n", packetsRead)
			return

		default:
			time.Sleep(25 * time.Millisecond)
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

// stop closes the UDP connection
func (device *AbacoUDPReceiver) stop() error {
	err := device.conn.Close()
	close(device.sendmore)
	return err
}

//------------------------------------------------------------------------------------------------

// AbacoSource represents all AbacoRing ring buffers and AbacoUDPReceiver objects
// that can potentially supply data, as well as all AbacoGroups that are discovered in the
// SampleData phase.
// We currently expect the use of EITHER ring buffers or UDP receivers but not both. We won't
// enforce this as a requirement unless it proves important.
type AbacoSource struct {
	// These items have to do with shared memory ring buffers
	Nrings      int // number of available ring buffers
	arings      map[int]*AbacoRing
	activeRings []*AbacoRing

	// These items have to do with UDP data sources
	udpReceivers []*AbacoUDPReceiver

	// Below here are independent of ring buffers vs UDP data sources.
	producers   []PacketProducer
	groups      map[GroupIndex]*AbacoGroup
	readPeriod  time.Duration
	buffersChan chan AbacoBuffersType

	// PhaseUnwrapper parameters must be stored for use when each AbacoGroup is created.
	unwrapEnable    bool // whether to activate unwrapping
	unwrapResetSamp int  // unwrap resets after this many samples (or never if ≤0)
	pulseSign       int  // one of (+1,-1) to say expected pulse direction
	biasUnwrapping  bool // whether to activate biased unwrapping
	AnySource
}

// NewAbacoSource creates a new AbacoSource.
func NewAbacoSource() (*AbacoSource, error) {
	source := new(AbacoSource)
	source.name = "Abaco"
	source.arings = make(map[int]*AbacoRing)
	source.udpReceivers = make([]*AbacoUDPReceiver, 0)
	source.producers = make([]PacketProducer, 0)
	source.groups = make(map[GroupIndex]*AbacoGroup)
	source.channelsPerPixel = 1

	// Probe for ring buffers that exist, and set up possible receivers.
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
	as.closeDevices()
}

// AbacoSourceConfig holds the arguments needed to call AbacoSource.Configure by RPC.
type AbacoSourceConfig struct {
	ActiveCards     []int
	AvailableCards  []int
	HostPortUDP     []string // host:port pairs to listen for UDP packets
	Unwrapping      bool     // whether to activate unwrapping
	UnwrapResetSamp int      // unwrap resets after this many samples (or never if ≤0)
	PulseSign       int      // should be one of (-1,+1) to choose the reset point
	Bias            bool     // whether to use biased unwrapping
}

// Configure sets up the internal buffers with given size, speed, and min/max.
func (as *AbacoSource) Configure(config *AbacoSourceConfig) (err error) {
	// Make sure entries in the ActiveCards slice are unique and sorted
	cardseen := make(map[int]bool)
	i := 0
	for _, cnum := range config.ActiveCards {
		if !cardseen[cnum] {
			cardseen[cnum] = true
			config.ActiveCards[i] = cnum
			i++
		}
	}
	config.ActiveCards = config.ActiveCards[:i]
	sort.Ints(config.ActiveCards)

	// Make sure entries in the HostPortUDP slice are unique and sorted
	hpseen := make(map[string]bool)
	i = 0
	for _, hp := range config.HostPortUDP {
		if !hpseen[hp] {
			hpseen[hp] = true
			config.HostPortUDP[i] = hp
			i++
		}
	}
	config.HostPortUDP = config.HostPortUDP[:i]
	sort.Strings(config.HostPortUDP)

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

	as.unwrapEnable = config.Unwrapping
	as.unwrapResetSamp = config.UnwrapResetSamp
	as.biasUnwrapping = config.Bias
	as.pulseSign = config.PulseSign

	// Activate the cards listed in the config request.
	as.producers = make([]PacketProducer, 0)
	as.activeRings = make([]*AbacoRing, 0)
	for i, c := range config.ActiveCards {
		device := as.arings[c]
		if device == nil {
			return fmt.Errorf("ActiveCards[%d]: card=%v, device == nil", i, c)
		}
		as.activeRings = append(as.activeRings, device)
		as.producers = append(as.producers, device)
	}

	// Bind servers at each host:port pair in the list of requested UDP receivers
	as.udpReceivers = make([]*AbacoUDPReceiver, 0)
	for _, hostport := range config.HostPortUDP {
		device, err := NewAbacoUDPReceiver(hostport)
		if err != nil {
			fmt.Printf("Could not bind server at udp://%s/, %v\n", hostport, err)
			return err
		}
		as.udpReceivers = append(as.udpReceivers, device)
		as.producers = append(as.producers, device)
	}
	return nil
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
	if len(as.producers) <= 0 {
		return fmt.Errorf("No Abaco ring buffers or UDP receivers are active")
	}

	// Launch device.samplePackets as goroutines on each device, in parallel, to save time.
	type SampleResult struct {
		allpackets []*packets.Packet
		err        error
	}
	sampleResults := make(chan SampleResult)
	timeout := 2000 * time.Millisecond
	for _, pp := range as.producers {
		go func(pp PacketProducer) {
			pp.start()
			p, err := pp.samplePackets(timeout)
			sampleResults <- SampleResult{allpackets: p, err: err}
		}(pp)
	}

	// Now sort the packets received into the right AbacoGroups
	as.nchan = 0
	as.groups = make(map[GroupIndex]*AbacoGroup)
	for _ = range as.producers {
		results := <-sampleResults
		now := time.Now()
		if results.err != nil {
			return results.err
		}
		// Create new AbacoGroup for each GroupIndex seen
		for _, p := range results.allpackets {
			cidx := gIndex(p)
			if _, ok := as.groups[cidx]; !ok {
				as.groups[cidx] = NewAbacoGroup(cidx, as.unwrapEnable, as.biasUnwrapping, as.unwrapResetSamp, as.pulseSign)
				as.nchan += cidx.Nchan
			}
		}
		as.distributePackets(results.allpackets, now)
	}

	// Verify that no channel # appears in 2 groups.
	known := make(map[int]bool)
	for _, g := range as.groups {
		cinit := g.index.Firstchan
		cend := cinit + g.index.Nchan
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

	as.sampleRate = 0
	for _, group := range as.groups {
		if as.sampleRate == 0 {
			as.sampleRate = group.sampleRate
		}
		// For now (Aug 2021), let's declare 2 groups to have approximately equal
		// sample rates if they agree to 1 part per billion. This means we don't test
		// floating point numbers for exact equality.
		diff := math.Abs(as.sampleRate - group.sampleRate)
		if diff/as.sampleRate > 1e-9 {
			fmt.Printf("Oh crap! Two groups have different sample rates: %f, %f, |diff|=%f", as.sampleRate, group.sampleRate, diff)
			panic("Oh crap! Two groups have different sample rates.")
			// TODO: what if multiple groups have truly unequal rates??
		}
	}
	as.samplePeriod = time.Duration(roundint(1e9 / as.sampleRate))

	return nil
}

// PrepareChannels configures an AbacoSource by initializing all data structures that
// have to do with channels and their naming/numbering.
func (as *AbacoSource) PrepareChannels() error {
	as.channelsPerPixel = 1

	// Fill the channel names and numbers slices
	// For rowColCodes, treat each channel group as a "column" and number chan within it
	// as rows 0...g.nchan-1.
	as.chanNames = make([]string, 0, as.nchan)
	as.chanNumbers = make([]int, 0, as.nchan)
	as.rowColCodes = make([]RowColCode, 0, as.nchan)
	ncol := len(as.groups)
	for col, g := range as.groupKeysSorted {
		for row := 0; row < g.Nchan; row++ {
			cnum := row + g.Firstchan
			name := fmt.Sprintf("chan%d", cnum)
			as.chanNames = append(as.chanNames, name)
			as.chanNumbers = append(as.chanNumbers, cnum)
			as.rowColCodes = append(as.rowColCodes, rcCode(row, col, g.Nchan, ncol))
		}
	}
	return nil
}

// StartRun tells the hardware to switch into data streaming mode.
// Discard existing data, then launch a goroutine to consume data.
func (as *AbacoSource) StartRun() error {
	for _, pp := range as.producers {
		pp.discardStale()
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
	droppedBytes   int
	droppedFrames  int
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
			var droppedFrames int
			var droppedBytes int
			for _, pp := range as.producers {
				allPackets, err := pp.ReadAllPackets()
				lastSampleTime = time.Now()
				if err != nil {
					fmt.Printf("PacketProducer.ReadAllPackets failed with error: %v\n", err)
					panic("PacketProducer.ReadAllPackets failed")
				}
				as.distributePackets(allPackets, lastSampleTime)
			}

			// var t1, t2 time.Time
			// t1, t2 = t2, time.Now()
			// fmt.Printf("Time required to distributePackets: %v\n", t2.Sub(t1))

			// Align the first sample in each group. Do this by checking the first global sequenceNumber
			// in each group and trimming leading packets if any precede the others.
			// Fill in for any missing packets first, so a missing first packet isn't a problem.
			firstSn := uint32(0)
			for idx, group := range as.groups {
				bytesAdded, packetsAdded, framesAdded := group.fillMissingPackets()
				droppedFrames += framesAdded
				droppedBytes += bytesAdded
				if packetsAdded > 0 && ProblemLogger != nil {
					cfirst := idx.Firstchan
					clast := cfirst + idx.Nchan - 1
					ProblemLogger.Printf("AbacoGroup %v=channels [%d,%d] filled in %d missing packets (%d frames)", idx,
						cfirst, clast, packetsAdded, framesAdded)
				}
				sn0, err := group.firstSeqNum()
				if err != nil { // That is, no data available from this group
					continue awaitmoredata
				}
				if sn0 > firstSn {
					firstSn = sn0
				}
			}
			// t1, t2 = t2, time.Now()
			// fmt.Printf("Time required to fillMissingPackets: %v\n", t2.Sub(t1))

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
			// t1, t2 = t2, time.Now()
			// fmt.Printf("Time required to trimPacketsBefore: %v\n", t2.Sub(t1))

			// Demux data into this slice of slices of RawType
			datacopies := make([][]RawType, as.nchan)
			for i := 0; i < as.nchan; i++ {
				datacopies[i] = make([]RawType, framesToDeMUX)
			}
			chanProcessed := 0
			bytesProcessed := 0
			for _, k := range as.groupKeysSorted {
				group := as.groups[k]
				dc := datacopies[chanProcessed : chanProcessed+group.nchan]
				bytesProcessed += group.demuxData(dc, framesToDeMUX)
				chanProcessed += group.nchan
			}
			// t1, t2 = t2, time.Now()
			// fmt.Printf("Time required to demuxData: %v\n", t2.Sub(t1))

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
				droppedBytes:   droppedBytes,
				droppedFrames:  droppedFrames,
			}
			if bytesProcessed > 0 {
				timeout.Reset(timeoutPeriod)
			}
		}
	}
}

// getNextBlock returns the channel on which data sources send data and any errors.
// Waiting on this channel = waiting on the source to produce a data block.
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
				firstFrameIndex: as.nextFrameNum,
				firstTime:       firstTime,
				signed:          true,
				droppedFrames:   buffersMsg.droppedFrames,
			}
			block.segments[channelIndex] = seg
			block.nSamp = len(data)
		}(channelIndex)
	}
	wg.Wait()
	as.nextFrameNum += FrameIndex(framesUsed)
	if as.heartbeats != nil {
		pmb := float64(buffersMsg.totalBytes) / 1e6
		hwmb := float64(buffersMsg.totalBytes-buffersMsg.droppedBytes) / 1e6
		as.heartbeats <- Heartbeat{Running: true, HWactualMB: hwmb, DataMB: pmb,
			Time: timeDiff.Seconds()}
	}
	now := time.Now()
	delay := now.Sub(lastSampleTime)
	if delay > 100*time.Millisecond {
		log.Printf("Buffer %v/%v, now-firstTime %v\n", len(as.buffersChan), cap(as.buffersChan), now.Sub(firstTime))
	}

	return block
}

// closeDevices ends closes the ring buffers of all active AbacoRing objects and all UDP servers.
func (as *AbacoSource) closeDevices() error {
	for _, pp := range as.producers {
		pp.stop()
	}
	as.producers = as.producers[:0]
	return nil
}
