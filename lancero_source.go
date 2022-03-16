package dastard

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
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
	clockMHz    int
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
	datacopies       [][]RawType
	lastSampleTime   time.Time
	timeDiff         time.Duration
	totalBytes       int
	dataDropDetected bool
}

// LanceroSource is a DataSource that handles 1 or more lancero devices.
type LanceroSource struct {
	devices                  map[int]*LanceroDevice
	ncards                   int
	clockMHz                 int
	nsamp                    int
	active                   []*LanceroDevice
	chan2readoutOrder        []int
	Mix                      []*Mix
	dataBlockCount           int
	buffersChan              chan BuffersChanType
	readPeriod               time.Duration
	mixRequests              chan *MixFractionObject
	firstRowChanNum          int            // Channel number of the 1st row (default 1)
	chanSepCards             int            // Channel separation between cards (or 0 to indicate number sequentially)
	chanSepColumns           int            // Channel separation between columns (or 0 to indicate number sequentially)
	currentMix               chan []float64 // allows ConfigureMixFraction to return the currentMix race free
	externalTriggerLastState bool
	previousLastSampleTime   time.Time
	AnySource
}

// NewLanceroSource creates a new LanceroSource.
func NewLanceroSource() (*LanceroSource, error) {
	source := new(LanceroSource)
	source.name = "Lancero"
	source.nsamp = 1
	source.devices = make(map[int]*LanceroDevice)
	source.channelsPerPixel = 2

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

// contains is used to make sure the same device isn't used twice
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
	CardDelay         []int
	ActiveCards       []int
	ShouldAutoRestart bool
	FirstRow          int // Channel number of the 1st row (default 1)
	ChanSepCards      int // Channel separation between cards (or 0 to indicate number sequentially)
	ChanSepColumns    int // Channel separation between columns (or 0 to indicate number sequentially)
	DastardOutput     LanceroDastardOutputJSON
}

// LanceroDastardOutputJSON is used to return values over the JSON RPC which cannot be set over the JSON rpc
type LanceroDastardOutputJSON struct {
	Nsamp            int
	ClockMHz         int
	AvailableCards   []int
	Lsync            int
	Settle           int
	SequenceLength   int
	PropagationDelay int
	BAD16CardDelay   int
}

// Configure sets up the internal buffers with given size, speed, and min/max.
// FiberMask must be identical across all cards, 0xFFFF uses all fibers, 0x0001 uses only fiber 0
// ClockMhz must be identical arcross all cards, as of June 2018 it's always 125
// CardDelay can have one value, which is shared across all cards, or must be one entry per card
// ActiveCards is a slice of indices into ls.devices to activate
// AvailableCards is an output, contains a sorted slice of valid indices for use in ActiveCards
func (ls *LanceroSource) Configure(config *LanceroSourceConfig) (err error) {
	ls.sourceStateLock.Lock()
	defer ls.sourceStateLock.Unlock()

	// populate AvailableCards before any possible errors
	config.DastardOutput.AvailableCards = make([]int, 0)
	for k := range ls.devices {
		config.DastardOutput.AvailableCards = append(config.DastardOutput.AvailableCards, k)
	}
	sort.Ints(config.DastardOutput.AvailableCards)

	var cg cringeGlobals
	cg, err = cringeGlobalsRead(cringeGlobalsPath)
	if err != nil {
		return err
	}
	//fmt.Println("read cringeGlobals in LanceroSource.Configure")
	//spew.Dump(cg)

	if ls.sourceState != Inactive {
		return fmt.Errorf("cannot Configure a LanceroSource if it's not Inactive")
	}

	// Error if Nsamp not in [1,16].
	if cg.Nsamp > 16 || cg.Nsamp < 1 {
		return fmt.Errorf("LanceroSourceConfig.Nsamp=%d but requires 1<=NSAMP<=16", cg.Nsamp)
	}

	ls.active = make([]*LanceroDevice, 0)
	ls.clockMHz = cg.ClockMHz
	ls.shouldAutoRestart = config.ShouldAutoRestart
	ls.firstRowChanNum = config.FirstRow
	ls.chanSepColumns = config.ChanSepColumns
	ls.chanSepCards = config.ChanSepCards
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
		dev.clockMHz = cg.ClockMHz
		dev.nrows = cg.SequenceLength
		dev.lsync = cg.Lsync
	}

	ls.nsamp = cg.Nsamp
	config.DastardOutput.Nsamp = cg.Nsamp
	config.DastardOutput.ClockMHz = cg.ClockMHz
	config.DastardOutput.Lsync = cg.Lsync
	config.DastardOutput.Settle = cg.Settle
	config.DastardOutput.SequenceLength = cg.SequenceLength
	config.DastardOutput.PropagationDelay = cg.PropagationDelay
	config.DastardOutput.BAD16CardDelay = cg.BAD16CardDelay

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
	// Cannot start this process if configuration failed.
	if ls.configError != nil {
		return ls.configError
	}
	ls.dataBlockCount = 0
	ls.nchan = 0
	for _, device := range ls.active {

		err := device.sampleCard()
		if err != nil {
			return err
		}
		ls.nchan += device.ncols * device.nrows * 2
		ls.sampleRate = float64(device.clockMHz) * 1e6 / float64(device.lsync*device.nrows)
	}

	ls.samplePeriod = time.Duration(roundint(1e9 / ls.sampleRate))
	ls.updateChanOrderMap()

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
	return nil
}

// PrepareChannels configures a LanceroSource by initializing all data structures that
// have to do with channels and their naming/numbering.
func (ls *LanceroSource) PrepareChannels() error {
	ls.channelsPerPixel = 2

	// Check that ls.chanSepColumns and chanSepCards are appropriate,
	// i.e. large enough to avoid channel number collisions.
	if ls.chanSepCards < 0 {
		return fmt.Errorf("Lancero configured with ChanSepCards=%d, need non-negative", ls.chanSepCards)
	}
	if ls.chanSepColumns < 0 {
		return fmt.Errorf("Lancero configured with chanSepColumns=%d, need non-negative", ls.chanSepColumns)
	}
	if ls.chanSepColumns > 0 {
		for _, device := range ls.active {
			if device.nrows > ls.chanSepColumns {
				err := fmt.Errorf("/dev/lancero%d has %d rows, which exceeds ChanSepColumns (%d). Setting latter to 0",
					device.devnum, device.nrows, ls.chanSepColumns)
				log.Printf("%v", err)
				ls.chanSepColumns = 0
				return err
			}
		}
	}
	if ls.chanSepCards > 0 {
		for _, device := range ls.active {
			colsep := device.nrows
			if ls.chanSepColumns > 0 {
				colsep = ls.chanSepColumns
			}
			if colsep*device.ncols > ls.chanSepCards {
				err := fmt.Errorf("/dev/lancero%d needs %d channels, which exceeds ChanSepCards (%d). Setting latter to 0",
					device.devnum, colsep*device.ncols, ls.chanSepCards)
				log.Printf("%v", err)
				ls.chanSepColumns = 0
				return err
			}
		}
	}

	ls.rowColCodes = make([]RowColCode, ls.nchan)
	ls.chanNames = make([]string, ls.nchan)
	ls.chanNumbers = make([]int, ls.nchan)
	index := 0
	cnum := ls.firstRowChanNum
	thisColFirstCnum := cnum - ls.chanSepColumns
	ls.groupKeysSorted = make([]GroupIndex, 0)
	for _, device := range ls.active {
		if ls.chanSepCards > 0 {
			cnum = device.devnum*ls.chanSepCards + ls.firstRowChanNum
			thisColFirstCnum = cnum - ls.chanSepColumns
		}
		for col := 0; col < device.ncols; col++ {
			if ls.chanSepColumns > 0 {
				cnum = thisColFirstCnum + ls.chanSepColumns
			}
			thisColFirstCnum = cnum
			cg := GroupIndex{Firstchan: cnum, Nchan: device.nrows}
			ls.groupKeysSorted = append(ls.groupKeysSorted, cg)
			for row := 0; row < device.nrows; row++ {
				ls.chanNames[index] = fmt.Sprintf("err%d", cnum)
				ls.chanNumbers[index] = cnum
				ls.rowColCodes[index] = rcCode(row, col, device.nrows, device.ncols)
				index++
				ls.chanNames[index] = fmt.Sprintf("chan%d", cnum)
				ls.chanNumbers[index] = cnum
				ls.rowColCodes[index] = rcCode(row, col, device.nrows, device.ncols)
				index++
				cnum++
			}
		}
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
	var calculatedNrows int
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
					calculatedNrows = (p - q) / n
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
		calculatedLsync := roundint((float64(periodNS) / 1000) * float64(device.clockMHz) / float64(device.nrows))
		if math.Abs(float64(calculatedLsync)/float64(device.lsync)-1) > 0.02 {
			fmt.Printf("WARNING: calculated lsync=%d, but have lsync=%d\n", calculatedLsync, device.lsync)
		}
		if calculatedNrows != device.nrows {
			return fmt.Errorf("calculatedNrows=%d does not match nrows=%d", calculatedNrows, device.nrows)
		}
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

type cringeGlobals struct {
	Settle           int `json:"SETT"`
	SequenceLength   int `json:"seqln"`
	Lsync            int `json:"lsync"`
	TestPattern      int `json:"testpattern"`
	PropagationDelay int `json:"propagationdelay"`
	Nsamp            int `json:"NSAMP"`
	BAD16CardDelay   int `json:"carddelay"`
	XPT              int `json:"XPT"`
	ClockMHz         int
	// TODO. Someday Cringe will tell Dastard two more facts: the number of rows that exist
	// (which might be more than the number read out) and the first row # being read out now.
}

// cringeGlobalsPath calculate the path to ~/.cringe/cringeGlobals.json by expanding the ~
func cringeGlobalsCalculatePath() string {
	usr, err := user.Current()
	if err != nil {
		panic(err) // I don't have a plan to handle this error meaningfully, so just panic, we can always change it later if we figure out how to deal with it
	}
	dir := usr.HomeDir
	path := filepath.Join(dir, ".cringe", "cringeGlobals.json")
	// log.Println("cringeGlobalsPath", path)
	return path
}

var cringeGlobalsPath = cringeGlobalsCalculatePath()

// cringeGlobalsRead loads the cringeGlobals.json file into a cringeGlobals struct
func cringeGlobalsRead(jsonPath string) (cringeGlobals, error) {
	jsonFile, err := os.Open(jsonPath)
	defer jsonFile.Close()
	// if we os.Open returns an error then handle it
	if err != nil {
		return cringeGlobals{}, err
	}
	byteValue, _ := ioutil.ReadAll(jsonFile)

	var cg cringeGlobals
	// unmarshal our byteArray which contains our
	// jsonFile's content into 'cg'
	err = json.Unmarshal(byteValue, &cg)
	if err != nil {
		return cringeGlobals{}, err
	}
	cg.ClockMHz = 125 // Cringe should write this, but it's always 125 MHz for now
	return cg, nil
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
		lastSuccesfulRead := time.Now()
		for {
			select {
			case <-ls.abortSelf:
				close(ls.buffersChan)
				return

			case <-ticker.C:
				timeSinceLastSuccesfulRead := time.Since(lastSuccesfulRead)
				if timeSinceLastSuccesfulRead > 10*time.Second {
					panic("too long since last succesful read")
				}
				var buffers [][]RawType
				framesUsed := math.MaxInt64
				var lastSampleTime time.Time
				var dataDropDetected bool
				if len(ls.active) > 1 {
					panic("Handling multiple devices not yet implemented")
				}
				dev := ls.active[0]
				b, timeFix, err := dev.card.AvailableBuffer()
				if err != nil {
					panic("Warning: AvailableBuffer failed")
				}
				if len(b) < 3*dev.frameSize { // read is too small
					fmt.Println("lancero read too small")
					continue
				}
				q, p, ncols, err := lancero.FindFrameBits(b, lanceroFBOffset)
				nrows := (p - q) / ncols
				if ncols != dev.ncols || nrows != dev.nrows || err != nil {
					fmt.Printf("ncols have %v, want %v. nrows have %v, want %v, timeSinceLastSuccesfulRead %v\n",
						ncols, dev.ncols, nrows, dev.nrows, timeSinceLastSuccesfulRead)
					dev.card.ReleaseBytes(len(b))
					continue
				}
				firstWord := q
				// check for dataDrop
				if firstWord != dev.ncols*dev.nrows {
					// if data drop detected
					// alter b and timeFix in place
					dataDropDetected = true
					// align to next frame start
					dropFromStart := firstWord * 4
					if firstWord == 0 {
						panic("not sure what to do here, but it wont self fix")
					}
					dev.card.ReleaseBytes(dropFromStart) // we could instead remember dropFromStart and add it
					// to the later call to ReleaseBytes
					dropFromEnd := dev.frameSize - dropFromStart
					if dropFromEnd <= 0 {
						fmt.Printf("firstWord %v, dropFromStart %v, dropFromEnd %v\n", firstWord, dropFromEnd, dropFromStart)
						panic("expect dropFromEnd>0")
					}
					b = b[dropFromStart : len(b)-dropFromEnd]
					fractionOfSampledPeriod := float64(dropFromEnd) / float64(dev.frameSize)
					timeFix = timeFix.Add(-ls.samplePeriod * time.Duration(fractionOfSampledPeriod))
					log.Printf("DATA DROP, first word = %v\n", firstWord)

				}
				buffers = append(buffers, bytesToRawType(b))
				bframes := len(b) / dev.frameSize
				if bframes < framesUsed { // for multiple cards, take data amount equal to minimum across all cards
					framesUsed = bframes
					lastSampleTime = timeFix
				}

				timeDiff := lastSampleTime.Sub(ls.lastread)
				if timeDiff > 2*ls.readPeriod {
					fmt.Println("timeDiff in lancero reader", timeDiff)
				}
				ls.lastread = lastSampleTime

				if framesUsed == math.MaxInt64 || framesUsed == 0 {
					panic("should not get here")
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
					timeDiff: timeDiff, totalBytes: totalBytes, dataDropDetected: dataDropDetected}
				if !dataDropDetected {
					lastSuccesfulRead = time.Now()
				}
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
	panicTime := time.Duration(10 * time.Second)
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
	dataDropDetected := buffersMsg.dataDropDetected

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

	var droppedFrames int
	if dataDropDetected {
		droppedDuration := lastSampleTime.Sub(ls.previousLastSampleTime)
		droppedFrames = roundint(droppedDuration.Seconds() * ls.sampleRate)
		ProblemLogger.Printf("Dropped %d lancero frames over Δt=%v", droppedFrames, droppedDuration)
	}

	for channelIndex := 0; channelIndex < nchan; channelIndex++ {
		data := datacopies[ls.chan2readoutOrder[channelIndex]]
		isFeedbackChannel := (channelIndex%2 == 1)
		if isFeedbackChannel { // feedback channel needs more processing
			mix := ls.Mix[channelIndex]
			errData := datacopies[ls.chan2readoutOrder[channelIndex-1]]
			//	MixRetardFb alters data in place to mix some of errData in based on mix.errorScale
			mix.MixRetardFb(&data, &errData)
		}
		seg := DataSegment{
			rawData:         data,
			framesPerSample: 1, // This will be changed later if decimating
			framePeriod:     ls.samplePeriod,
			firstFrameIndex: ls.nextFrameNum + FrameIndex(droppedFrames),
			firstTime:       firstTime,
			signed:          !isFeedbackChannel,
			droppedFrames:   droppedFrames,
		}
		block.segments[channelIndex] = seg
		block.nSamp = len(data)
	}
	ls.nextFrameNum += FrameIndex(framesUsed)
	ls.previousLastSampleTime = lastSampleTime
	if ls.heartbeats != nil {
		mb := float64(totalBytes) / 1e6
		ls.heartbeats <- Heartbeat{Running: true, HWactualMB: mb, DataMB: mb,
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
