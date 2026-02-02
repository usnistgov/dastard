package dastard

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/lorenzosaino/go-sysctl"
	"github.com/usnistgov/dastard/packets"
)

const abacoSubframeDivisions = 64 // The external triggers will be resolved this much finer than the frame rate
// We are writing external trigger info at a clock rate finer than the frame rate, the "subframe" rate.
// In TDM systems the subframe rate was equal to the row rate, thus historical usage has conflated the two.
// For the Abaco it's arbitrary how finely we divide frames, but even at the lowest frame rate, 64 subdivisions
// seems adequate.

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

// FrameTimingCorrespondence tracks how we convert timestamps to FrameIndex
type FrameTimingCorrepondence struct {
	TimestampCountsPerSubframe uint64 // Ratio of timestamp rate to subframe division rate
	LastFirmwareTimestamp      packets.PacketTimestamp
	LastSubframeCount          FrameIndex
}

const minimum_buffer_size = 4 * 1024 * 1024
const recommended_buffer_size = 64 * 1024 * 1024

// Return non-nil error if the UDP receive buffer isn't at least 4 MB (system default is 200 kB on most Ubuntu, which is bad)
func verifyLargeUDPBuffer(min_buf_size int) error {
	// For now, we only check UDP buffer size on Linux. All others, use at your own risk.
	if runtime.GOOS != "linux" {
		return nil
	}
	val, err := sysctl.Get("net.core.rmem_max")
	if err != nil {
		return err
	}
	if bsize, err := strconv.Atoi(val); err != nil {
		return err
	} else if bsize < min_buf_size {
		return fmt.Errorf(
			`udp receive buffer is %d, require >= %d, recommend %d.
The UDP receive buffer must be larger than the default for Dastard to work with µMUX data sources.
To fix it for this session do: 'sudo sysctl -w net.core.rmem_max=%d'
to fix it in future sessions add the line 'net.core.rmem_max=%d' to the file /etc/sysctl.conf`,
			bsize, min_buf_size, recommended_buffer_size, recommended_buffer_size, recommended_buffer_size)
	}
	return nil
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
	FrameTimingCorrepondence
}

// AbacoUnwrapOptions contains options to control phase unwrapping.
type AbacoUnwrapOptions struct {
	RescaleRaw bool  // are we rescaling (left-shifting) the raw data? If not, the rest is ignored
	Unwrap     bool  // are we even unwrapping at all?
	Bias       bool  // should the unwrapping be biased?
	ResetAfter int   // reset the phase unwrap offset back to 0 after this many samples (≈auto-relock)
	PulseSign  int   // direction data will go when pulse arrives, used to calculate bias level
	InvertChan []int // invert channels with these numbers before unwrapping phase
}

func (u AbacoUnwrapOptions) isvalid() error {
	if u.Unwrap && !u.RescaleRaw {
		return fmt.Errorf("should not have Unwrap=true when RescaleRaw=false in AbacoUnwrapOpts")
	}
	return nil
}

func (u AbacoUnwrapOptions) calcBiasLevel() int {
	var biasLevel int
	if u.Bias {
		// Assume 2^16 equals exactly one ϕ0, then bias based on the assumption of critically
		// damped pulses. In that case, the largest falling and rising slopes are in the ratio
		// -0.12 to +0.88. Hence, a bias of ±0.38*ϕ0 (sign given by the pulseSign).
		biasLevel = int(math.Round(+0.38 * 65536))
		if u.PulseSign < 0 {
			biasLevel = -biasLevel
		}
	}
	return biasLevel
}

// NewAbacoGroup creates an AbacoGroup given the specified GroupIndex.
func NewAbacoGroup(index GroupIndex, opt AbacoUnwrapOptions) *AbacoGroup {
	var bitsToDrop uint
	if opt.RescaleRaw {
		bitsToDrop = abacoBitsToDrop
	}

	g := new(AbacoGroup)
	g.index = index
	g.nchan = index.Nchan
	g.queue = make([]*packets.Packet, 0)
	g.unwrap = make([]*PhaseUnwrapper, g.nchan)

	isInverted := func(channum int) bool {
		return slices.Contains(opt.InvertChan, channum)
	}

	for i := range g.unwrap {
		channum := i + index.Firstchan
		invert := isInverted(channum)
		g.unwrap[i] = NewPhaseUnwrapper(abacoFractionBits, bitsToDrop, opt.Unwrap,
			opt.calcBiasLevel(), opt.ResetAfter, opt.PulseSign, invert)
	}
	return g
}

// updateFrameTiming will update the group's `FrameTimingCorrespondence` info,
// as long as the packet has a valid timestamp, and the frameIdx is greater than any
// previously seen. (Otherwise, return without effect.)
func (group *AbacoGroup) updateFrameTiming(p *packets.Packet, frameIdx FrameIndex) {
	ts := p.Timestamp()
	if ts == nil {
		return
	}
	newSubframeCount := frameIdx * abacoSubframeDivisions
	deltaSubframe := newSubframeCount - group.LastSubframeCount
	deltaTs := ts.T - group.LastFirmwareTimestamp.T

	// It is perfectly normal for this to be called dozens of times in a row with identical
	// values of `FrameIndex`, yielding `deltaSubFrame == 0`. In such cases, we have effectively
	// no new information about frame timing, so return without effect (also helps us avoid a
	// divide-by-zero error).
	if deltaSubframe <= 0 || deltaTs == 0 {
		return
	}

	// There will be transient nonsense in (rare) cases where the timestamp.Rate changes. I think the
	// best approach is to proceed. After the next (consistent) timestamp, it will recover.
	group.LastFirmwareTimestamp = *ts
	group.LastSubframeCount = newSubframeCount
	group.TimestampCountsPerSubframe = deltaTs / uint64(deltaSubframe)
}

// subframeCountFromTimestamp uses the group.FrameTimingCorrespondence data to
// convert a timestamp (in raw Abaco clock counts) to a FrameIndex
func (group *AbacoGroup) subframeCountFromTimestamp(firmwareTimestamp uint64) FrameIndex {
	if group.TimestampCountsPerSubframe == 0 {
		return 0
	}
	// This will be in error if the two timestamps have unequal Rates, but
	// we don't have that info on a per-timestamp basis. And anyway,
	// that problem should be rare and go away as soon as new Rate is established.
	deltaTs := int64(firmwareTimestamp) - int64(group.LastFirmwareTimestamp.T)
	deltaF := deltaTs / int64(group.TimestampCountsPerSubframe)
	return FrameIndex(int64(group.LastSubframeCount) + deltaF)
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
			fmt.Printf("Sample rate for chan [%4d-%4d] %.6g /sec determined from %4d packets: Δt=%f sec, Δserial=%d, and %.3f samp/packet\n",
				group.index.Firstchan, group.index.Firstchan+group.nchan-1, group.sampleRate,
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
	lastPacketSize := 0
	nchan := group.nchan

	for _, p := range group.queue {
		gidx := gIndex(p)
		if gidx != group.index {
			msg := fmt.Sprintf("Group %v received packet for index %v", group.index, gidx)
			panic(msg)
		}

		// Don't demux a packet that would over-fill the requested # of frames.
		frameAvail := p.Frames()
		lastPacketSize = frameAvail
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
		msg := fmt.Sprintf("Consumed %d of available %d packets, but there are still %d frames to fill and %d frames in packet\n",
			packetsConsumed, len(group.queue), frames, lastPacketSize)
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
// Implementations include only UDP servers, at this time.
type PacketProducer interface {
	ReadAllPackets() ([]*packets.Packet, error)
	samplePackets(d time.Duration) ([]*packets.Packet, error)
	start() error
	discardStale() error
	stop() error
}

//------------------------------------------------------------------------------------------------

// AbacoUDPReceiver represents a single Abaco device producing data by UDP packets to a single UDP port.
type AbacoUDPReceiver struct {
	host     string                 // in the form: "127.0.0.1:56789"
	conn     *net.UDPConn           // active UDP connection
	data     chan []*packets.Packet // channel for sending next bunch of packets to downstream processor
	sendmore chan bool              // a channel by which downstream processor signals readiness for next bunch of packets
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
	device.conn = conn

	const UDPPacketSize = 8192

	device.sendmore = make(chan bool)
	device.data = make(chan []*packets.Packet)

	// Two goroutines:
	// 1. Read from the UDP socket, put packet on channel singlepackets.
	// 2. Read packet from channel singlepackets and convert to a dastard `packets.Packet` type.
	//    Build a slice of all such objects; put slice onto `device.data` when `device.sendmore` says so.

	// This goroutine (#1) reads the UDP socket and puts the resulting packets on channel singlepackets.
	// They will be removed and queued by the other goroutine (#2).
	go func() {
		defer close(device.data)
		defer func() { device.conn = nil }()

		message := make([]byte, UDPPacketSize)

		const initialQueueCapacity = 4000 // = 32 MB worth of full-size packets
		queue := make([]*packets.Packet, 0, initialQueueCapacity)

		// This 100ms timeout for reading the UDP socket will allow the goroutine to wake up that often
		// and check for requests on the `device.sendmore` channel to send the queued data to `device.data`.
		// That's a timeout for the unusual case of the socket having no packets, of course. Normal
		// behavior is to get messages almost every time.
		const delay = 100 * time.Millisecond
		device.conn.SetReadDeadline(time.Now().Add(delay))

		for {
			select {
			case _, ok := <-device.sendmore:
				device.data <- queue
				if !ok {
					return
				}
				queue = make([]*packets.Packet, 0, initialQueueCapacity)
			default:
				_, _, err := device.conn.ReadFrom(message)
				// If error, was it a timeout?
				if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
					device.conn.SetReadDeadline(time.Now().Add(delay))
					continue
				}
				if err != nil {
					// Getting an error in ReadFrom is the normal way to detect closed connection.
					return
				}

				if pack, err := packets.ReadPacket(bytes.NewReader(message)); err == nil {
					queue = append(queue, pack)
				} else if err == io.EOF {
					return
				} else {
					fmt.Printf("Error converting UDP to packet: err %v, packet %v\n", err, pack)
					return
				}
			}
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

// AbacoSource represents all AbacoUDPReceiver objects that can potentially supply data,
// as well as all AbacoGroups that are discovered in the SampleData phase.
type AbacoSource struct {
	// These items have to do with UDP data sources
	udpReceivers []*AbacoUDPReceiver

	// Below here are independent of exact nature of the data sources.
	producers    []PacketProducer
	groups       map[GroupIndex]*AbacoGroup
	readPeriod   time.Duration
	buffersChan  chan AbacoBuffersType
	eTrigPackets []*packets.Packet // Unprocessed packets with external trigger info

	unwrapOpts       AbacoUnwrapOptions
	minUDPBufferSize int
	AnySource
}

// NewAbacoSource creates a new AbacoSource.
func NewAbacoSource() (*AbacoSource, error) {
	source := new(AbacoSource)
	source.name = "Abaco"
	source.udpReceivers = make([]*AbacoUDPReceiver, 0)
	source.producers = make([]PacketProducer, 0)
	source.groups = make(map[GroupIndex]*AbacoGroup)
	source.eTrigPackets = make([]*packets.Packet, 0)
	source.channelsPerPixel = 1
	source.subframeDivisions = abacoSubframeDivisions
	source.minUDPBufferSize = minimum_buffer_size

	return source, nil
}

// Delete terminates any input data producers.
func (as *AbacoSource) Delete() {
	as.closeDevices()
}

// VoltsPerArb returns a per-channel value scaling raw into volts.
// For Abaco, the default is 1 Phi0 per 4096 arb units
func (as *AbacoSource) VoltsPerArb() []float32 {
	if as.voltsPerArb == nil || len(as.voltsPerArb) != as.nchan {
		as.voltsPerArb = make([]float32, as.nchan)
		vpa := float32(1.0 / 65536.0) // default if not rescaling raw data
		if as.unwrapOpts.RescaleRaw {
			vpa = float32(1. / (65536 >> abacoBitsToDrop))
		}
		for i := 0; i < as.nchan; i++ {
			as.voltsPerArb[i] = vpa
		}
	}
	return as.voltsPerArb
}

// AbacoSourceConfig holds the arguments needed to call AbacoSource.Configure by RPC.
type AbacoSourceConfig struct {
	// For the time being, leave ActiveCards, AvailableCards in the config, but ignore them.
	// This leaves Dastard compatible with Dcom versions that still send these fields.
	ActiveCards    []int    // Unused: relic of when we had shared memory spaces per card
	AvailableCards []int    // Unused: relic of when we had shared memory spaces per card
	HostPortUDP    []string // host:port pairs to listen for UDP packets
	AbacoUnwrapOptions
}

// Configure sets up the internal buffers with given size, speed, and min/max.
func (as *AbacoSource) Configure(config *AbacoSourceConfig) (err error) {
	if err := config.AbacoUnwrapOptions.isvalid(); err != nil {
		return err
	}

	// Make sure entries in the HostPortUDP slice are unique and sorted
	hpseen := make(map[string]bool)
	i := 0
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

	as.unwrapOpts = config.AbacoUnwrapOptions

	// Bind servers at each host:port pair in the list of requested UDP receivers
	as.producers = make([]PacketProducer, 0)
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
	frameTimeUpdated := false
	for _, p := range allpackets {
		if p.IsExternalTrigger() {
			as.eTrigPackets = append(as.eTrigPackets, p)
			continue
		}

		cidx := gIndex(p)
		grp := as.groups[cidx]
		grp.enqueuePacket(p, now)
		if !frameTimeUpdated {
			grp.updateFrameTiming(p, as.nextFrameNum)
			frameTimeUpdated = true
		}
	}
}

// Sample determines key data facts by sampling some initial data.
func (as *AbacoSource) Sample() error {
	if len(as.producers) <= 0 {
		return fmt.Errorf("no Abaco data producers (UDP receivers) are active")
	}

	// Panic if there are UDP receivers, and the UDP receive buffer isn't somewhat larger than the 200 kB default.
	// This is to stop people who ignore the README info about sysctl, and then complain that "Dastard doesn't work".
	if len(as.udpReceivers) > 0 {
		if err := verifyLargeUDPBuffer(as.minUDPBufferSize); err != nil {
			msg := fmt.Sprintf("Could not verify large UDP receive buffer.\n%v", err)
			panic(msg)
		}
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
			if err := pp.start(); err != nil {
				var nopackets []*packets.Packet
				sampleResults <- SampleResult{allpackets: nopackets, err: err}
				return
			}
			p, err := pp.samplePackets(timeout)
			sampleResults <- SampleResult{allpackets: p, err: err}
		}(pp)
	}

	// Now sort the packets received into the right AbacoGroups
	as.nchan = 0
	as.groups = make(map[GroupIndex]*AbacoGroup)
	allRates := []float64{}
	for range as.producers {
		results := <-sampleResults
		now := time.Now()
		if results.err != nil {
			return results.err
		}
		// Create new AbacoGroup for each GroupIndex seen
		for _, p := range results.allpackets {
			if p.IsExternalTrigger() {
				continue
			}
			cidx := gIndex(p)
			if _, ok := as.groups[cidx]; !ok {
				as.groups[cidx] = NewAbacoGroup(cidx, as.unwrapOpts)
				as.nchan += cidx.Nchan
				allRates = append(allRates, as.groups[cidx].sampleRate)
			}
		}
		as.distributePackets(results.allpackets, now)
	}

	// Verify that no two groups have unequal sample rates
	if len(allRates) > 1 {
		for _, r := range allRates {
			if math.Abs(r-allRates[0]) > 1 {
				fmt.Printf("Channel groups have unequal sample rates: %v\n", allRates)
				panic("Two channel groups have unequal sample rates. Dastard does not support this.")
			}
		}
	}

	// Verify that no channel # appears in 2 groups.
	known := make(map[int]bool)
	for _, g := range as.groups {
		cinit := g.index.Firstchan
		cend := cinit + g.index.Nchan
		for cnum := cinit; cnum < cend; cnum++ {
			if known[cnum] {
				return fmt.Errorf("channel group %v sees channel %d, which was in another group", g.index, cnum)
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
		if diff/as.sampleRate > 1e-6 {
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
	as.subframeOffsets = make([]int, as.nchan) // all zeros for Abaco sources
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
	defer timeout.Stop()
	ticker := time.NewTicker(as.readPeriod)
	defer ticker.Stop()
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
			// read from the UDP port
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

func (as *AbacoSource) extractExternalTriggers() []int64 {
	externalTriggers := make([]int64, 0)
	for _, p := range as.eTrigPackets {
		// These packets have form (u32, u32, u64) repeating, but we don't care about the first 2.
		// So it's simplest to treat AS IF they were (u64, 64) repeating.
		// fmt.Printf("\nExternal trigger packet found:\n")
		// spew.Dump(*p)
		// fmt.Printf("Packet sequence: %d   Num frames: %d\n", p.SequenceNumber(), p.Frames())

		key0 := as.groupKeysSorted[0]
		grp0 := as.groups[key0]
		outlength := p.Frames()
		switch d := p.Data.(type) {
		case []byte:
			u64data := unsafe.Slice((*uint64)(unsafe.Pointer(&d[0])), 2*outlength)
			packets.ByteSwap(u64data)
			for i := 0; i < 2*outlength; i += 2 {
				// At the moment, the first two int32 values are ignored. (Don't even know what they mean.)
				// v := (u64data[i] >> 32) & 0xffffffff
				// a := u64data[i] & 0xffffffff
				t := u64data[i+1]
				fc := int64(grp0.subframeCountFromTimestamp(t))
				// fmt.Printf("val 0x%8x  active 0x%8x  T 0x%16x    frameIndex %7d\n", v, a, t, fc)
				externalTriggers = append(externalTriggers, fc)
			}
		default:
			fmt.Println("Oh crap, wrong type in extractExternalTriggers")
		}
	}
	// Remove all queued eTrigPackets
	as.eTrigPackets = as.eTrigPackets[:0]
	return externalTriggers
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

	// Here we find external triggers from the queue of relevant packets
	externalTriggers := as.extractExternalTriggers()

	// TODO: we should loop over devices here, matching devices to channels.
	var wg sync.WaitGroup
	for channelIndex := range nchan {
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
				signed:          false,
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
	block.externalTriggerRowcounts = externalTriggers

	return block
}

// closeDevices stops and closes all UDP servers.
func (as *AbacoSource) closeDevices() error {
	for _, pp := range as.producers {
		pp.stop()
	}
	as.producers = as.producers[:0]
	return nil
}
