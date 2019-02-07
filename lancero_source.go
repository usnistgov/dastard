package dastard

import (
	"fmt"
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

// lanceroFBOffset gives the location of the frame bit is in bytes 2, 6, 10...
const lanceroFBOffset int = 2

// BuffersChanType is an internal message type used to allow
// a goroutine to read from the Lancero card and put data on a buffered channel
type BuffersChanType struct {
	datacopies     [][]RawType
	lastSampleTime time.Time
	timeDiff       time.Duration
	totalBytes     int
}

// LanceroSource is a DataSource that handles 1 or more lancero devices.
type LanceroSource struct {
	devices                  map[int]*LanceroDevice
	ncards                   int
	clockMhz                 int
	nsamp                    int
	active                   []*LanceroDevice
	chan2readoutOrder        []int
	Mix                      []*Mix
	dataBlockCount           int
	buffersChan              chan BuffersChanType
	readPeriod               time.Duration
	mixRequests              chan *MixFractionObject
	currentMix               chan []float64 // allows ConfigureMixFraction to return the currentMix race free
	externalTriggerLastState bool
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
			log.Printf("warning: failed to open /dev/lancero_user%d and companion devices", dnum)
			continue
		}
		ld.card = lan
		source.devices[dnum] = &ld
		source.ncards++
	}
	if source.ncards == 0 && len(devnums) > 0 {
		return source, fmt.Errorf("could not open any of /dev/lancero_user*, though devnums %v exist", devnums)
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
	FiberMask         uint32
	ClockMhz          int
	Nsamp             int
	CardDelay         []int
	ActiveCards       []int
	AvailableCards    []int
	ShouldAutoRestart bool
}

// Configure sets up the internal buffers with given size, speed, and min/max.
// FiberMask must be identical across all cards, 0xFFFF uses all fibers, 0x0001 uses only fiber 0
// ClockMhz must be identical arcross all cards, as of June 2018 it's always 125
// CardDelay can have one value, which is shared across all cards, or must be one entry per card
// ActiveCards is a slice of indicies into ls.devices to activate
// AvailableCards is an output, contains a sorted slice of valid indicies for use in ActiveCards
func (ls *LanceroSource) Configure(config *LanceroSourceConfig) (err error) {
	ls.sourceStateLock.Lock()
	defer ls.sourceStateLock.Unlock()
	if ls.sourceState != Inactive {
		return fmt.Errorf("cannot Configure a LanceroSource if it's not Inactive")
	}

	// Error if Nsamp not in [1,16].
	if config.Nsamp > 16 || config.Nsamp < 1 {
		return fmt.Errorf("LanceroSourceConfig.Nsamp=%d but requires 1<=NSAMP<=16", config.Nsamp)
	}

	ls.active = make([]*LanceroDevice, 0)
	ls.clockMhz = config.ClockMhz
	ls.shouldAutoRestart = config.ShouldAutoRestart
	for i, c := range config.ActiveCards {
		dev := ls.devices[c]
		if dev == nil {
			err = fmt.Errorf("i=%v, c=%v, device == nil", i, c)
			break
		}
		if contains(ls.active, dev) {
			err = fmt.Errorf("attempt to use same device two times: i=%v, c=%v, config.ActiveCards=%v", i, c, config.ActiveCards)
			break
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

	ls.nsamp = config.Nsamp
	return err
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

// ConfigureMixFraction sets the MixFraction potentially for many channels, returns the list of current mix values
// mix = fb + errorScale*err
func (ls *LanceroSource) ConfigureMixFraction(mfo *MixFractionObject) ([]float64, error) {
	for _, channelIndex := range mfo.ChannelIndices {
		if channelIndex >= len(ls.Mix) || channelIndex < 0 {
			return nil, fmt.Errorf("channelIndex %v out of bounds", channelIndex)
		}
		if channelIndex%2 == 0 {
			return nil, fmt.Errorf("channelIndex %v is even, only odd channels (feedback) allowed", channelIndex)
		}
	}
	ls.mixRequests <- mfo
	current := <-ls.currentMix // retrieve current mix race-free
	return current, nil
}

// Sample determines key data facts by sampling some initial data.
func (ls *LanceroSource) Sample() error {
	ls.dataBlockCount = 0
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

	// Set up mix requests/replies to go on channels with a modest buffer size.
	// If this proves to be a problem, we can change it to ls.nchan later.
	const MIXDEPTH = 10 // How many active mix requests allowed before RPC backs up
	ls.mixRequests = make(chan *MixFractionObject, MIXDEPTH)
	ls.currentMix = make(chan []float64, MIXDEPTH)

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

	var timeFix0, timeFix time.Time
	var err0 error
	_, timeFix0, err0 = lan.AvailableBuffer()
	if err0 != nil {
		return err0
	}
	timeFix = timeFix0
	var buffer []byte
	minDuration := 200 * time.Millisecond // the NoHardware tests can fail if this is too long, since I test with multiple lancero devices, the first device has to wait for all other devices to finish
	var bytesReadSinceTimeFix0 int64
	frameBitsHandled := false
	for timeFix.Sub(timeFix0) < minDuration {
		// notice above we called AvailableBuffer and discarded data, noted timeFix0
		// here we read for at least minDuration, counting all bytes read (hopefully reading for this long will make lsync reliably correct)
		// we also append all bytes read to buffer to learn ncol and nrows
		// and stop appending once we have learned ncol and nrows
		select {
		case <-interruptCatcher:
			return fmt.Errorf("LanceroDevice.sampleCard was interrupted")
		default:
			if _, _, err2 := lan.Wait(); err2 != nil {
				return err2
			}
			var b []byte
			var err1 error
			b, timeFix, err1 = lan.AvailableBuffer()
			if err1 != nil {
				return err1
			}
			bytesReadSinceTimeFix0 += int64(len(b))
			if !frameBitsHandled {
				buffer = append(buffer, b...) // only append if framebits havent been handled, to reduce unneeded memory usage
				log.Println(lancero.OdDashTX(buffer, 10))
				q, p, n, err3 := lancero.FindFrameBits(buffer, lanceroFBOffset)
				if err3 == nil {
					device.ncols = n
					device.nrows = (p - q) / n
					device.frameSize = device.ncols * device.nrows * 4
					frameBitsHandled = true
				} else {
					fmt.Printf("Error in FindFrameBits: %v", err3)
				}
			}
			lan.ReleaseBytes(len(b))
		}
	}
	if frameBitsHandled {
		periodNS := timeFix.Sub(timeFix0).Nanoseconds() / (bytesReadSinceTimeFix0 / int64(device.frameSize))
		device.lsync = roundint((float64(periodNS) / 1000) * float64(device.clockMhz) / float64(device.nrows))
		log.Printf("cols=%d  rows=%d  frame period %5d ns, lsync=%d\n", device.ncols,
			device.nrows, periodNS, device.lsync)
		return nil
	}
	return fmt.Errorf("failed to SampleCard")
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
			bytes, _, err := lan.AvailableBuffer()
			bytesRead += len(bytes)
			if err != nil {
				return fmt.Errorf("error in AvailableBuffer: %v", err)
			}
			if len(bytes) <= 0 {
				continue
			}
			firstWord, _, _, err := lancero.FindFrameBits(bytes, lanceroFBOffset)
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
	ls.launchLanceroReader()
	return nil
}

// launchLanceroReader launches a goroutine that reads from the Lancero card
// whenever prompted by a ticker with a duration of ls.readPeriod.
// It then demuxes the data and puts it on ls.BuffersChan. A second goroutine
// receives data buffers on that channel. Because the channel is buffered with
// a large capacity, the Lancero can read with minimum potential for overflowing
// because of long latency in the analysis stages of Dastard.

func (ls *LanceroSource) launchLanceroReader() {
	ls.buffersChan = make(chan BuffersChanType, 100)
	ls.readPeriod = 50 * time.Millisecond
	go func() {
		ticker := time.NewTicker(ls.readPeriod)
		for {
			select {
			case <-ls.abortSelf:
				close(ls.buffersChan)
				return

			case <-ticker.C:
				var buffers [][]RawType
				framesUsed := math.MaxInt64
				var lastSampleTime time.Time

				for _, dev := range ls.active {

					b, timeFix, err := dev.card.AvailableBuffer()
					if err != nil {
						panic("Warning: AvailableBuffer failed")
					}
					buffers = append(buffers, bytesToRawType(b))
					bframes := len(b) / dev.frameSize
					if bframes < framesUsed {
						framesUsed = bframes
						lastSampleTime = timeFix
					}
				}
				timeDiff := lastSampleTime.Sub(ls.lastread)
				if timeDiff > 2*ls.readPeriod {
					fmt.Println("timeDiff in lancero reader", timeDiff)
				}
				ls.lastread = lastSampleTime
				// check for changes in nrow, ncol and lsync
				for ibuf, dev := range ls.active {
					buffer := buffers[ibuf]
					q, p, n, err := lancero.FindFrameBits(rawTypeToBytes(buffer), lanceroFBOffset)
					if err != nil {
						panic(fmt.Sprintf("Error in findFrameBits: %v", err))
					}
					qExpect := dev.ncols * dev.nrows
					// FindFrameBits q is the index of the first frame bit after a non frame bit
					ncols := n
					nrows := (p - q) / n
					periodNS := timeDiff.Nanoseconds() / int64(framesUsed)
					lsync := roundint((float64(periodNS) / 1000) * float64(dev.clockMhz) / float64(nrows))
					if q != qExpect || ncols != dev.ncols || nrows != dev.nrows || framesUsed <= 0 {
						fmt.Printf("(Not checking lsync) have ibuf %v, q %v, ncols %v, nrows %v, lsync %v, framesUsed %v\nwant q %v, ncols %v, nrows %v, lsync %v, dataBlockCount %v\n",
							ibuf, q, ncols, nrows, lsync, framesUsed, qExpect, dev.ncols, dev.nrows, dev.lsync, ls.dataBlockCount)
						panic("error reading from lancero, probably let buffer overfill")
					}
				}
				// Consume framesUsed frames of data from each channel.
				// Careful! This slice of slices will be in lancero READOUT order:
				// r0c0, r0c1, r0c2, etc.
				datacopies := make([][]RawType, ls.nchan)
				for i := range ls.processors {
					datacopies[i] = make([]RawType, framesUsed)
				}

				// NOTE: Galen reversed the inner loop order here, it was previously frames, then datastreams.
				// This loop is the demultiplexing step. Loop over devices, data streams, then frames.
				// For a single lancero 8x30 with linePeriod=20=160 ns this version handles:
				// this loop handles 10938 frames in 20.5 ms on 687horton, aka aka 1.9 us/frame
				// the previous loop handles 52000 frames in 253 ms, aka 4.8 us/frame
				// when running more than 2 lancero cards, even this version may not keep up reliably
				nchanPrevDevices := 0
				for ibuf, dev := range ls.active {
					buffer := buffers[ibuf]
					nchan := dev.ncols * dev.nrows * 2
					for i := 0; i < nchan; i++ {
						dc := datacopies[i+nchanPrevDevices]
						idx := i
						for j := 0; j < framesUsed; j++ {
							dc[j] = buffer[idx]
							idx += nchan
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
				if len(ls.buffersChan) == cap(ls.buffersChan) {
					panic(fmt.Sprintf("internal buffersChan full, len %v, capacity %v", len(ls.buffersChan), cap(ls.buffersChan)))
				}
				ls.buffersChan <- BuffersChanType{datacopies: datacopies, lastSampleTime: lastSampleTime,
					timeDiff: timeDiff, totalBytes: totalBytes}
			}
		}
	}()
}

// getNextBlock returns the channel on which data sources send data and any errors.
// More importantly, wait on this returned channel to await the source having a data block.
// This goroutine will end by putting a valid or error-ish dataBlock onto ls.nextBlock.
// If the block has a non-nil error, this goroutine will also close ls.nextBlock.
// The LanceroSource version also has to monitor the timeout channel, handle any possible
// mixRequests, and wait for the buffersChan to yield real, valid Lancero data.
// The idea here is to minimize the number of long-running goroutines, which are hard
// to reason about.
func (ls *LanceroSource) getNextBlock() chan *dataBlock {
	panicTime := time.Duration(cap(ls.buffersChan)) * ls.readPeriod
	go func() {
		for {
			// This select statement was formerly the ls.blockingRead method
			select {
			case <-time.After(panicTime):
				panic(fmt.Sprintf("timeout, no data from lancero after %v / %v", panicTime, ls.readPeriod))

			case mfo := <-ls.mixRequests:
				for i, index := range mfo.ChannelIndices {
					fraction := mfo.MixFractions[i]
					ls.Mix[index].errorScale = fraction / float64(ls.nsamp)
				}
				mixFrac := make([]float64, len(ls.Mix))
				for i, m := range ls.Mix {
					mixFrac[i] = m.errorScale * float64(ls.nsamp)
				}
				ls.currentMix <- mixFrac

			case buffersMsg, ok := <-ls.buffersChan:
				//  Check is buffersChan closed? Recognize that by receiving zero values and/or being drained.
				if buffersMsg.datacopies == nil || !ok {
					block := new(dataBlock)
					if err := ls.stop(); err != nil {
						block.err = err
						ls.nextBlock <- block
					}
					close(ls.nextBlock)
					return
				}
				// ls.buffersChan contained valid data, so act on it.
				block := ls.distributeData(buffersMsg)
				ls.dataBlockCount++ // set to 0 in SampleCard
				ls.nextBlock <- block
				if block.err != nil {
					close(ls.nextBlock)
				}
				return
			}
		}
	}()
	return ls.nextBlock
}

// distributeData reads the raw data buffers from all devices in the LanceroSource
// and distributes their data by copying into slices that go on channels, one
// channel per data stream.
func (ls *LanceroSource) distributeData(buffersMsg BuffersChanType) *dataBlock {
	datacopies := buffersMsg.datacopies
	lastSampleTime := buffersMsg.lastSampleTime
	timeDiff := buffersMsg.timeDiff
	totalBytes := buffersMsg.totalBytes
	framesUsed := len(datacopies[0])

	// Backtrack to find the time associated with the first sample.
	segDuration := time.Duration(roundint((1e9 * float64(framesUsed-1)) / ls.sampleRate))
	firstTime := lastSampleTime.Add(-segDuration)
	block := new(dataBlock)
	nchan := len(datacopies)
	block.segments = make([]DataSegment, nchan)

	// The external trigger is encoded in the second least significant bit of the feedback
	// The information is redundant across columns, so we should only scan a single column
	// The external trigger bit resolution is the row rate, eg for each row we get a 0 or a 1 representing
	// if the voltage at the external trigger input is 3.3 V or not
	// Here we will look for edge trigger, eg the bit is 1 but was 0 on the previous row
	// Then we record the "rowcounts", where rowcount = nrow*framecount+row
	// external trigger search must occur before Mix, since mix alters FB in place
	externalTriggerRowcounts := make([]int64, 0)
	nrows := ls.devices[0].nrows
	for frame := 0; frame < framesUsed; frame++ { // frame within this block, need to add ls.nextFrameNum for consistent timing across blocks
		for row := 0; row < nrows; row++ { // search the first column for frame bit level triggers
			channelIndex := row*2 + 1
			v := datacopies[channelIndex][frame]
			externalTriggerState := (v & 0x02) == 0x02 // external trigger bit is 2nd least significant bit in feedback (odd channelIndex)
			if externalTriggerState && !ls.externalTriggerLastState {
				externalTriggerRowcounts = append(externalTriggerRowcounts, (int64(frame)+int64(ls.nextFrameNum))*int64(nrows)+int64(row))
			}
			ls.externalTriggerLastState = externalTriggerState
		}
	}
	block.externalTriggerRowcounts = externalTriggerRowcounts

	for channelIndex := 0; channelIndex < nchan; channelIndex++ {
		data := datacopies[ls.chan2readoutOrder[channelIndex]]
		if channelIndex%2 == 1 { // feedback channel needs more processing
			mix := ls.Mix[channelIndex]
			errData := datacopies[ls.chan2readoutOrder[channelIndex-1]]
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
		block.segments[channelIndex] = seg
		block.nSamp = len(data)
	}
	ls.nextFrameNum += FrameIndex(framesUsed)
	if ls.heartbeats != nil {
		ls.heartbeats <- Heartbeat{Running: true, DataMB: float64(totalBytes) / 1e6,
			Time: timeDiff.Seconds()}
	}
	now := time.Now()
	delay := now.Sub(lastSampleTime)
	if delay > 100*time.Millisecond {
		log.Printf("Buffer %v/%v, now-firstTime %v\n", len(ls.buffersChan), cap(ls.buffersChan), now.Sub(firstTime))
	}

	return block
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
