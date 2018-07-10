package dastard

import (
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"sort"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/usnistgov/dastard/lancero"
)

// LanceroDevice represents one lancero device.
type LanceroDevice struct {
	devnum      int
	nrows       int
	ncols       int
	lsync       int
	fiberMask   uint32
	cardDelay   int
	clockMhz    int
	frameSize   int // frame size, in bytes
	adapRunning bool
	collRunning bool
	card        lancero.Lanceroer
}

// LanceroSource is a DataSource that handles 1 or more lancero devices.
type LanceroSource struct {
	devices           map[int]*LanceroDevice
	ncards            int
	clockMhz          int
	nsamp             int
	active            []*LanceroDevice
	chan2readoutOrder []int
	Mix               []*Mix
	AnySource
}

// NewLanceroSource creates a new LanceroSource.
func NewLanceroSource() (*LanceroSource, error) {
	source := new(LanceroSource)
	source.name = "Lancero"
	source.nsamp = 1
	source.devices = make(map[int]*LanceroDevice)
	devnums, err := lancero.EnumerateLanceroDevices()
	if err != nil {
		return source, err
	}

	for _, dnum := range devnums {
		ld := LanceroDevice{devnum: dnum}
		lan, err := lancero.NewLancero(dnum)
		if err != nil {
			log.Printf("warning: failed to create /dev/lancero_user%d", dnum)
			continue
		}
		ld.card = lan
		source.devices[dnum] = &ld
		source.ncards++
	}
	return source, nil
}

// Delete closes all Lancero cards
func (ls *LanceroSource) Delete() {
	for _, device := range ls.devices {
		if device != nil && device.card != nil {
			device.card.Close()
		}
	}
}

// used to make sure the same device isn't used twice
func contains(s []*LanceroDevice, e *LanceroDevice) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// LanceroSourceConfig holds the arguments needed to call LanceroSource.Configure by RPC.
// For now, we'll make the FiberMask equal for all cards. That need not
// be permanent, but I do think ClockMhz is necessarily the same for all cards.
type LanceroSourceConfig struct {
	FiberMask      uint32
	ClockMhz       int
	Nsamp          int
	CardDelay      []int
	ActiveCards    []int
	AvailableCards []int
}

// Configure sets up the internal buffers with given size, speed, and min/max.
// FiberMask must be identical across all cards, 0xFFFF uses all fibers, 0x0001 uses only fiber 0
// ClockMhz must be identical arcross all cards, as of June 2018 it's always 125
// CardDelay can have one value, which is shared across all cards, or must be one entry per card
// ActiveCards is a slice of indicies into ls.devices to activate
// AvailableCards is an output, contains a sorted slice of valid indicies for use in ActiveCards
func (ls *LanceroSource) Configure(config *LanceroSourceConfig) error {
	ls.runMutex.Lock()
	defer ls.runMutex.Unlock()

	ls.active = make([]*LanceroDevice, 0)
	ls.clockMhz = config.ClockMhz
	for i, c := range config.ActiveCards {
		dev := ls.devices[c]
		if dev == nil {
			return fmt.Errorf("i=%v, c=%v, device == nil", i, c)
		}
		if contains(ls.active, dev) {
			return fmt.Errorf("attempt to use same device two times: i=%v, c=%v, config.ActiveCards=%v", i, c, config.ActiveCards)
		}
		ls.active = append(ls.active, dev)
		if len(config.CardDelay) >= 1+i {
			dev.cardDelay = config.CardDelay[i]
		}
		dev.fiberMask = config.FiberMask
		dev.clockMhz = config.ClockMhz
	}
	config.AvailableCards = make([]int, 0)
	for k := range ls.devices {
		config.AvailableCards = append(config.AvailableCards, k)
	}
	sort.Ints(config.AvailableCards)

	// Error if Nsamp not in [1,16].
	if config.Nsamp > 16 || config.Nsamp < 1 {
		return fmt.Errorf("LanceroSourceConfig.Nsamp=%d but requires 1<=NSAMP<=16", config.Nsamp)
	}
	ls.nsamp = config.Nsamp
	return nil
}

// updateChanOrderMap updates the map chan2readoutOrder based on the number
// of columns and rows in each active device
// also initializes errorScale and lastFbData to correct length with all zeros
func (ls *LanceroSource) updateChanOrderMap() {
	nChannelsAllCards := int(0)
	for _, dev := range ls.active {
		nChannelsAllCards += dev.ncols * dev.nrows * 2
	}
	ls.Mix = make([]*Mix, nChannelsAllCards)
	for i := 0; i < nChannelsAllCards; i++ {
		ls.Mix[i] = &Mix{}
	}
	ls.chan2readoutOrder = make([]int, nChannelsAllCards)
	nchanPrevDevices := 0
	for _, dev := range ls.active {
		nchan := dev.ncols * dev.nrows * 2
		for readIdx := 0; readIdx < nchan; readIdx++ {
			rownum := (readIdx / 2) / dev.ncols
			colnum := (readIdx / 2) % dev.ncols
			channum := (readIdx % 2) + rownum*2 + (colnum*dev.nrows)*2
			ls.chan2readoutOrder[channum+nchanPrevDevices] = readIdx + nchanPrevDevices
		}
		nchanPrevDevices += nchan
	}
}

// ConfigureMixFraction sets the MixFraction for the channel associated with ProcessorIndex
// mix = fb + errorScale*err
func (ls *LanceroSource) ConfigureMixFraction(processorIndex int, mixFraction float64) error {
	if processorIndex >= len(ls.Mix) || processorIndex < 0 {
		return fmt.Errorf("processorIndex %v out of bounds", processorIndex)
	}
	if processorIndex%2 == 0 {
		return fmt.Errorf("proccesorIndex %v is even, only odd channels (feedback) allowed", processorIndex)
	}
	// Make this a goroutine so it can grab the lock whenever convenient, but after
	// any possible errors have already been sent back to the RPC server. This lock
	// is normally held most of the time by LanceroSource.blockingRead.
	go func() {
		ls.runMutex.Lock()
		defer ls.runMutex.Unlock()
		ls.Mix[processorIndex].errorScale = mixFraction / float64(ls.nsamp)
	}()
	return nil
}

// Sample determines key data facts by sampling some initial data.
func (ls *LanceroSource) Sample() error {
	ls.nchan = 0
	for _, device := range ls.active {
		err := device.sampleCard()
		if err != nil {
			return err
		}
		ls.nchan += device.ncols * device.nrows * 2
		ls.sampleRate = float64(device.clockMhz) * 1e6 / float64(device.lsync*device.nrows)
	}
	ls.samplePeriod = time.Duration(roundint(1e9 / ls.sampleRate))
	ls.updateChanOrderMap()

	ls.signed = make([]bool, ls.nchan)
	for i := 0; i < ls.nchan; i += 2 {
		ls.signed[i] = true
	}
	ls.voltsPerArb = make([]float32, ls.nchan)
	for i := 0; i < ls.nchan; i += 2 {
		ls.voltsPerArb[i] = 1.0 / (4096. * float32(ls.nsamp))
	}
	for i := 1; i < ls.nchan; i += 2 {
		ls.voltsPerArb[i] = 1. / 65535.0
	}

	ls.rowColCodes = make([]RowColCode, ls.nchan)
	i := 0
	for _, device := range ls.active {
		cardNchan := device.ncols * device.nrows * 2
		for j := 0; j < cardNchan; j += 2 {
			col := j / (2 * device.nrows)
			row := (j % (2 * device.nrows)) / 2
			ls.rowColCodes[i+j] = rcCode(row, col, device.nrows, device.ncols)
			ls.rowColCodes[i+j+1] = ls.rowColCodes[i+j]
		}
		i += cardNchan
	}
	ls.chanNames = make([]string, ls.nchan)
	ls.chanNumbers = make([]int, ls.nchan)
	for i := 1; i < ls.nchan; i += 2 {
		ls.chanNames[i-1] = fmt.Sprintf("err%d", 1+i/2)
		ls.chanNames[i] = fmt.Sprintf("chan%d", 1+i/2)
		ls.chanNumbers[i-1] = 1 + i/2
		ls.chanNumbers[i] = 1 + i/2
	}
	return nil
}

func (device *LanceroDevice) sampleCard() error {
	lan := device.card

	if err := lan.ChangeRingBuffer(1200000, 400000); err != nil {
		return fmt.Errorf("failed to change ring buffer size (driver problem): %v", err)
	}
	if err := lan.StartAdapter(2); err != nil {
		return fmt.Errorf("failed to start lancero (driver problem): %v", err)
	}
	defer lan.StopAdapter()
	log.Println("sampling card:")
	log.Println(spew.Sdump(lan))
	lan.InspectAdapter()

	linePeriod := 1 // use dummy values for things we will learn by sampling data
	frameLength := 1
	dataDelay := device.cardDelay
	channelMask := device.fiberMask
	err := lan.CollectorConfigure(linePeriod, dataDelay, channelMask, frameLength)
	if err != nil {
		return fmt.Errorf("error in CollectorConfigure: %v", err)
	}

	const simulate bool = false
	err = lan.StartCollector(simulate)
	defer lan.StopCollector()
	if err != nil {
		return fmt.Errorf("error in StartCollector: %v", err)
	}

	interruptCatcher := make(chan os.Signal, 1)
	signal.Notify(interruptCatcher, os.Interrupt)
	defer signal.Stop(interruptCatcher)
	var bytesRead int
	const tooManyBytes int = 1000000  // shouldn't need this many bytes to SampleData
	const tooManyIterations int = 100 // nor this many reads of the lancero
	for i := 0; i < tooManyIterations; i++ {
		if bytesRead >= tooManyBytes && i > 1 { // we ignore first buffer, so make sure we've seen 2
			return fmt.Errorf("LanceroDevice.sampleCard read %d bytes, failed to find nrow*ncol",
				bytesRead)
		}

		var waittime time.Duration
		var buffer []byte
		select {
		case <-interruptCatcher:
			return fmt.Errorf("LanceroDevice.sampleCard was interrupted")
		default:
			_, waittime, err = lan.Wait()
			if err != nil {
				return err
			}
			buffer, _, err = lan.AvailableBuffers()

			if err != nil {
				return err
			}
		}

		// if dastard is getting lsync wrong, consider requireing a minimum buffer size
		// or possibly appending a few buffers
		totalBytes := len(buffer)
		if i == 0 {
			lan.ReleaseBytes(totalBytes)
			bytesRead += totalBytes
			continue
		}
		log.Printf("waittime: %v\n", waittime)
		log.Printf("Found buffer with %9d total bytes, bytes read previously=%10d\n", totalBytes, bytesRead)
		log.Println(lancero.OdDashTX(buffer, 10))
		q, p, n, err := lancero.FindFrameBits(buffer)
		bytesPerFrame := 4 * (p - q)
		if err != nil {
			return fmt.Errorf("Error in findFrameBits: %v", err)
		}
		device.ncols = n
		device.nrows = (p - q) / n
		periodNS := waittime.Nanoseconds() / (int64(totalBytes) / int64(bytesPerFrame))
		device.lsync = roundint((float64(periodNS) / 1000) * float64(device.clockMhz) / float64(device.nrows))
		device.frameSize = device.ncols * device.nrows * 4

		log.Printf("cols=%d  rows=%d  frame period %5d ns, lsync=%d\n", device.ncols,
			device.nrows, periodNS, device.lsync)

		lan.ReleaseBytes(totalBytes)
		return nil
	}
	return fmt.Errorf("After %d reads, found no valid buffers in Lancero device %d", tooManyIterations, device.devnum)
}

// Imperfect round to nearest integer
func roundint(x float64) int {
	return int(x + math.Copysign(0.5, x))
}

// StartRun tells the hardware to switch into data streaming mode.
// For lancero TDM systems, we need to consume any initial data that constitutes
// a fraction of a frame.
func (ls *LanceroSource) StartRun() error {

	// Starting the source for all active cards has 3 steps per card.
	for _, device := range ls.active {
		// 1. Resize the ring buffer to hold up to 16,384 frames
		if device.frameSize <= 0 {
			device.frameSize = 128 // a random guess
		}
		const threshBufferRatio = 4
		thresh := 16384 * device.frameSize
		bufsize := threshBufferRatio * thresh
		if bufsize > int(lancero.HardMaxBufSize) {
			bufsize = int(lancero.HardMaxBufSize)
			thresh = bufsize / threshBufferRatio
		}
		lan := device.card
		if lan == nil {
			continue
		}
		if err := lan.ChangeRingBuffer(bufsize, thresh); err != nil {
			return fmt.Errorf("failed to change ring buffer size (driver problem): %v", err)
		}
		// 2. Start the adapter and collector components in firmware
		const Timeout int = 2 // seconds
		if err := lan.StartAdapter(Timeout); err != nil {
			return fmt.Errorf("failed to start lancero (driver problem): %v", err)
		}
		device.adapRunning = true

		linePeriod := 1 // use dummy values for things we will learn by sampling data
		frameLength := 1
		dataDelay := device.cardDelay
		channelMask := device.fiberMask
		if err := lan.CollectorConfigure(linePeriod, dataDelay,
			channelMask, frameLength); err != nil {
			return fmt.Errorf("error in CollectorConfigure: %v", err)
		}

		const simulate bool = false
		if err := lan.StartCollector(simulate); err != nil {
			return fmt.Errorf("error in StartCollector: %v", err)
		}
		device.collRunning = true
		// 3. Consume any possible fractional frames at the start of the buffer
		const tooManyBytes int = 1000000  // shouldn't need this many bytes to SampleData
		const tooManyIterations int = 100 // nor this many reads of the lancero
		var bytesRead int
		var success bool
		var i int
		for i = 0; i < tooManyIterations; i++ {
			if bytesRead >= tooManyBytes {
				return fmt.Errorf("LanceroDevice.sampleCard read %d bytes, failed to find nrow*ncol",
					bytesRead)
			}

			if _, _, err := lan.Wait(); err != nil {
				return fmt.Errorf("error in Wait: %v", err)
			}
			bytes, _, err := lan.AvailableBuffers()
			bytesRead += len(bytes)
			if err != nil {
				return fmt.Errorf("error in AvailableBuffers: %v", err)
			}
			if len(bytes) <= 0 {
				continue
			}
			firstWord, _, _, err := lancero.FindFrameBits(bytes)
			// should check for correct number of columns/rows again?
			if err == nil {
				if firstWord > 0 {
					bytesToRelease := 4 * firstWord
					log.Printf("First frame bit at word %d, so release %d of %d bytes\n", firstWord, bytesToRelease, len(bytes))
					lan.ReleaseBytes(bytesToRelease)
				}
				success = true
				break
			}

		}
		if !success {
			return fmt.Errorf("read %v bytes, did %v iterations", bytesRead, i)
		}
	}
	return nil
}

// blockingRead blocks and then reads data when "enough" is ready.
// This will need to somehow work across multiple cards???
func (ls *LanceroSource) blockingRead() error {
	ls.runMutex.Lock()
	defer ls.runMutex.Unlock()

	// Wait on only one device (the first) to have enough data.
	// Method distributeData will then read from all devices.
	done := make(chan error)
	dev := ls.active[0]
	go func() {
		_, _, err := dev.card.Wait()
		done <- err
	}()

	select {
	case <-ls.abortSelf:
		if err := ls.stop(); err != nil {
			return err
		}
		return io.EOF
	case err := <-done:
		if err != nil {
			return err
		}
		ls.distributeData()
	}
	return nil
}

// distributeData reads the raw data buffers from all devices in the LanceroSource
// and distributes their data by copying into slices that go on channels, one
// channel per data stream.
func (ls *LanceroSource) distributeData() {
	// Get one buffer per card. Whichever contains the fewest frames will
	// dictate how much data to process and what the timestamp is.
	framesUsed := math.MaxInt64
	var buffers [][]RawType
	var lastSampleTime time.Time
	for _, dev := range ls.active {
		b, timeFix, err := dev.card.AvailableBuffers()
		if err != nil {
			log.Printf("Warning: AvailableBuffers failed")
			return
		}
		buffers = append(buffers, bytesToRawType(b))

		bframes := len(b) / dev.frameSize
		if bframes < framesUsed {
			framesUsed = bframes
			lastSampleTime = timeFix
		}
	}
	if framesUsed <= 0 {
		log.Printf("Nothing to consume, buffer[0] size: %d samples\n", len(buffers[0]))
		return
	}
	timediff := lastSampleTime.Sub(ls.lastread)
	ls.lastread = lastSampleTime

	// Consume framesUsed frames of data from each channel.
	// Careful! This slice of slices will be in lancero READOUT order:
	// r0c0, r0c1, r0c2, etc.
	datacopies := make([][]RawType, len(ls.output))
	for i := range ls.output {
		datacopies[i] = make([]RawType, framesUsed)
	}

	// TODO: Check for frame bits being correctly placed in the stream?

	// TODO: Look for external trigger bits here, either once per card or once per column

	// This loop is the demultiplexing step. Loop over devices, then frames,
	// then data streams.
	nchanPrevDevices := 0
	for ibuf, dev := range ls.active {
		buffer := buffers[ibuf]
		nchan := dev.ncols * dev.nrows * 2
		idx := 0
		for j := 0; j < framesUsed; j++ {
			for i := 0; i < nchan; i++ {
				datacopies[i+nchanPrevDevices][j] = buffer[idx]
				idx++
			}
		}
		nchanPrevDevices += nchan
	}

	// Inform the driver to release the data we just consumed
	totalBytes := 0
	for _, dev := range ls.active {
		release := framesUsed * dev.frameSize
		dev.card.ReleaseBytes(release)
		totalBytes += release
	}

	// Now send these data downstream. Here we permute data into the expected
	// channel ordering: r0c0, r1c0, r2c0, etc via the chan2readoutOrder map.
	// Backtrack to find the time associated with the first sample.
	segDuration := time.Duration(roundint((1e9 * float64(framesUsed-1)) / ls.sampleRate))
	firstTime := lastSampleTime.Add(-segDuration)
	for channum, ch := range ls.output {
		data := datacopies[ls.chan2readoutOrder[channum]]
		if channum%2 == 1 { // feedback channel needs more processing
			mix := ls.Mix[channum]
			errData := datacopies[ls.chan2readoutOrder[channum-1]]
			//	MixRetardFb alters data in place to mix some of errData in based on mix.errorScale
			mix.MixRetardFb(&data, &errData)
		}

		seg := DataSegment{
			rawData:         data,
			framesPerSample: 1, // This will be changed later if decimating
			framePeriod:     ls.samplePeriod,
			firstFramenum:   ls.nextFrameNum,
			firstTime:       firstTime,
		}
		ch <- seg
	}
	ls.nextFrameNum += FrameIndex(framesUsed)
	if ls.heartbeats != nil {
		ls.heartbeats <- Heartbeat{Running: true, DataMB: float64(totalBytes) / 1e6,
			Time: timediff.Seconds()}
	}
}

// stop ends the data streaming on all active lancero devices.
func (ls *LanceroSource) stop() error {
	for _, device := range ls.active {
		if device.collRunning {
			device.card.StopCollector()
			device.collRunning = false
		}
	}

	for _, device := range ls.active {
		if device.adapRunning {
			device.card.StopAdapter()
			device.adapRunning = false
		}
	}
	return nil
}

// SetCoupling set up the trigger broker to connect err->FB, FB->err, or neither
func (ls *LanceroSource) SetCoupling(status CouplingStatus) error {
	// Notice that status == NoCoupling will visit both else clauses in this
	// function and therefore delete the connections from either sort of coupling.
	// It is safe to call DeleteConnection on pairs that are unconnected.
	if status == ErrToFB {
		for i := 0; i < ls.nchan; i += 2 {
			ls.broker.AddConnection(i, i+1)
		}
	} else {
		for i := 0; i < ls.nchan; i += 2 {
			ls.broker.DeleteConnection(i, i+1)
		}
	}

	if status == FBToErr {
		for i := 0; i < ls.nchan; i += 2 {
			ls.broker.AddConnection(i+1, i)
		}
	} else {
		for i := 0; i < ls.nchan; i += 2 {
			ls.broker.DeleteConnection(i+1, i)
		}
	}
	return nil
}
