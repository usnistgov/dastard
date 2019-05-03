package dastard

import (
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"time"

	"github.com/usnistgov/dastard/lancero"
	"github.com/usnistgov/dastard/ringbuffer"
)

// AbacoDevice represents a single Abaco device-special file and the ring buffer
// that stores its data.
type AbacoDevice struct {
	cardnum   int
	devnum    int
	nrows     int
	ncols     int
	nchan     int
	frameSize int // frame size, in bytes
	ring      *ringbuffer.RingBuffer
}

const maxAbacoCards = 4    // Don't allow more than this many cards.
const maxAbacoChannels = 8 // Don't allow more than this many channels per card.

// NewAbacoDevice creates a new AbacoDevice and opens the underlying file for reading.
func NewAbacoDevice(cardnum, devnum int) (dev *AbacoDevice, err error) {
	if cardnum >= maxAbacoCards || cardnum < 0 {
		return nil, fmt.Errorf("NewAbacoDevice() got cardnum=%d, want [0,%d]",
			cardnum, maxAbacoCards-1)
	}
	if devnum >= maxAbacoChannels || devnum < 0 {
		return nil, fmt.Errorf("NewAbacoDevice() got devnum=%d, want [0,%d]",
			devnum, maxAbacoChannels-1)
	}
	dev = new(AbacoDevice)
	dev.cardnum = cardnum
	dev.devnum = devnum

	shmNameBuffer := fmt.Sprintf("xdma%d_c2h_%d_buffer", dev.cardnum, dev.devnum)
	shmNameDesc := fmt.Sprintf("xdma%d_c2h_%d_description", dev.cardnum, dev.devnum)
	if dev.ring, err = ringbuffer.NewRingBuffer(shmNameBuffer, shmNameDesc); err != nil {
		return nil, err
	}
	return dev, nil
}

// abacoFBOffset gives the location of the frame bits: in bytes 0, 4, 8...
// (In the TDM system, this value is 2, meaning frame bits are in bytes 2, 6, 10....)
const abacoFBOffset int = 0

// sampleCard samples the data from a single card to scan for frame bits.
// For Abaco µMUX systems, we need to discard one DMA buffer worth of stale data,
// then check the data after that for frame bit info.
// 1. Discard all data currently in the ring buffer.
// 2. Discard 2 FIFOs worth of new data (John says sometimes needed).
// 3. Read 32k: enough data to discern frame bits; scan for frame bits
// 4. Store info that we've learned in AbacoSource or per device.
func (device *AbacoDevice) sampleCard() error {
	// Open the device and discard whatever is in the buffer
	if err := device.ring.Open(); err != nil {
		return err
	}
	fmt.Printf("Device ring.Open returns no error.\n")
	if err := device.ring.DiscardAll(); err != nil {
		return err
	}

	// Then discard the first 2 FIFOs worth of data, to be safe
	for bytesToDiscard := 128 * 1024; bytesToDiscard > 0; {
		data, err := device.ring.ReadMinimum(bytesToDiscard)
		if err != nil {
			return err
		}
		bytesToDiscard -= len(data)
	}

	// Now get the data we actually want
	data, err := device.ring.ReadMinimum(32768)
	if err != nil {
		return err
	}
	log.Print("Abaco bytes read for FindFrameBits: ", len(data))

	q, p, n, err3 := lancero.FindFrameBits(data, abacoFBOffset)
	if err3 == nil {
		device.ncols = n
		device.nrows = (p - q) / n
		device.nchan = device.ncols * device.nrows
		device.frameSize = device.ncols * device.nrows * 4 // For now: 4 bytes/sample
	} else {
		fmt.Printf("Error in FindFrameBits: %v", err3)
		return err3
	}
	return device.DiscardPartialFrame(data)
}

// DiscardPartialFrame reads and discards between [0, device.frameSize-1]
// bytes from the device's ring buffer, so that future reads will be aligned
// to the start of a frame.
func (device *AbacoDevice) DiscardPartialFrame(lastdata []byte) error {
	if len(lastdata) < device.frameSize {
		return fmt.Errorf("DiscardPartialFrame failed: given %d bytes, need %d",
			len(lastdata), device.frameSize)
	}
	// Can ignore all but the last 1 frame of data
	lastdata = lastdata[len(lastdata)-device.frameSize:]
	indexrow0 := -1
	fmt.Printf("Raw lastdata: %v\n", lastdata)
	for i := abacoFBOffset; i < device.frameSize; i += 4 {
		if lastdata[i]&0x1 != 0 {
			indexrow0 = i / 4
			break
		}
	}
	if indexrow0 < 0 {
		return fmt.Errorf("DiscardPartialFrame found no frame bits")
	}
	indexrow1 := -1
	for i := 4*indexrow0 + 4 + abacoFBOffset; i < device.frameSize; i += 4 {
		if lastdata[i]&0x1 == 0 {
			indexrow1 = i / 4
			break
		}
	}
	bytesToRead := 0
	if indexrow0 > 0 { // started checking after row 0
		bytesToRead = indexrow0 * 4
	} else { // started checking during (or at start of) row 0
		if indexrow1-indexrow0 == device.ncols {
			bytesToRead = 0
		} else {
			nrow0missed := device.ncols - (indexrow1 - indexrow0)
			bytesToRead = device.frameSize - 4*nrow0missed
		}

	}
	_, err := device.ring.ReadMinimum(bytesToRead)
	return err
}

// enumerateAbacoDevices returns a list of abaco device numbers that exist
// in the devfs. If /dev/xdma0_c2h_X exists, then X is added to the list.
// Does not yet handle cards other than xdma0.
func enumerateAbacoDevices() (devices []int, err error) {
	for cnum := 0; cnum < maxAbacoCards; cnum++ {
		for id := 0; id < maxAbacoChannels; id++ {
			name := fmt.Sprintf("/dev/xdma%d_c2h_%d", cnum, id)
			info, err := os.Stat(name)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				} else {
					return devices, err
				}
			}
			if (info.Mode() & os.ModeDevice) != 0 {
				devices = append(devices, 10*cnum+id)
			}
		}
	}
	return devices, nil
}

// AbacoSource represents all Abaco devices that can potentially supply data.
type AbacoSource struct {
	devices     map[int]*AbacoDevice
	Ndevices    int
	active      []*AbacoDevice
	readPeriod  time.Duration
	buffersChan chan AbacoBuffersType
	AnySource
}

// NewAbacoSource creates a new AbacoSource.
func NewAbacoSource() (*AbacoSource, error) {
	source := new(AbacoSource)
	source.name = "Abaco"
	source.devices = make(map[int]*AbacoDevice)

	deviceCodes, err := enumerateAbacoDevices()
	if err != nil {
		return source, err
	}

	for _, code := range deviceCodes {
		cnum := code / 10
		dnum := code % 10
		ad, err := NewAbacoDevice(cnum, dnum)
		if err != nil {
			log.Printf("warning: failed to create ring buffer for /dev/xdma%d_c2h_%d, though it should exist", cnum, dnum)
			continue
		}
		source.devices[code] = ad
		source.Ndevices++
	}
	if source.Ndevices == 0 && len(deviceCodes) > 0 {
		return source, fmt.Errorf("could not create ring buffer for any of /dev/xdma*_c2h_*, though deviceCodes %v exist", deviceCodes)
	}
	return source, nil
}

// Delete closes the ring buffers for all Abaco devices
func (as *AbacoSource) Delete() {
	for _, dev := range as.devices {
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
	if as.sourceState != Inactive {
		return fmt.Errorf("cannot Configure an AbacoSource if it's not Inactive")
	}

	// used to be sure the same device isn't listed twice in config.ActiveCards
	contains := func(s []*AbacoDevice, e *AbacoDevice) bool {
		for _, a := range s {
			if a == e {
				return true
			}
		}
		return false
	}

	// Activate the cards listed in the config request.
	as.active = make([]*AbacoDevice, 0)
	for i, c := range config.ActiveCards {
		dev := as.devices[c]
		if dev == nil {
			err = fmt.Errorf("i=%v, c=%v (card %d, dev %d), device == nil", i, c, c/10, c%10)
			break
		}
		if contains(as.active, dev) {
			err = fmt.Errorf("attempt to use same Abaco device two times: i=%v, c=%v, config.ActiveCards=%v", i, c, config.ActiveCards)
			break
		}
		as.active = append(as.active, dev)
	}
	config.AvailableCards = make([]int, 0)
	for k := range as.devices {
		config.AvailableCards = append(config.AvailableCards, k)
	}
	sort.Ints(config.AvailableCards)
	return nil
}

// Sample determines key data facts by sampling some initial data.
func (as *AbacoSource) Sample() error {
	as.nchan = 0
	if len(as.active) <= 0 {
		return fmt.Errorf("No Abaco devices are active")
	}
	for _, device := range as.active {
		if err := device.sampleCard(); err != nil {
			return err
		}
		as.nchan += device.ncols * device.nrows
	}
	as.sampleRate = 8125000.0 // HACK! For now, assume a value.
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
		if err := dev.ring.DiscardAll(); err != nil {
			panic("AbacoDevice.ring.DiscardAll failed")
		}
	}
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

//
func (as *AbacoSource) readerMainLoop() {
	timeoutPeriod := 5 * time.Second
	as.readPeriod = 50 * time.Millisecond

	as.buffersChan = make(chan AbacoBuffersType, 100)
	defer close(as.buffersChan)
	timeout := time.NewTimer(timeoutPeriod)
	ticker := time.NewTicker(as.readPeriod)
	defer ticker.Stop()
	defer timeout.Stop()

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
			framesUsed := math.MaxInt64
			totalBytes := 0
			datacopies := make([][]RawType, as.nchan)
			nchanPrevDevices := 0
			var lastSampleTime time.Time
			for _, dev := range as.active {
				bytesData, err := dev.ring.ReadAll()
				lastSampleTime = time.Now()
				if err != nil {
					fmt.Printf("AbacoDevice.ring.ReadAll failed with error: %v", err)
					panic("AbacoDevice.ring.ReadAll failed")
				}
				nb := len(bytesData)
				bframes := nb / dev.frameSize
				if bframes < framesUsed {
					framesUsed = bframes
				}
				// log.Printf("Read Abaco device #%d, total of %d bytes = %d frames + %d extra bytes",
				// 	devnum, nb, bframes, nb-bframes*dev.frameSize)

				// This is the demultiplexing step. Loops over channels,
				// then over frames.
				rawBuffer := bytesToRawType(bytesData)
				for i := 0; i < dev.nchan; i++ {
					datacopies[i+nchanPrevDevices] = make([]RawType, framesUsed)
					dc := datacopies[i+nchanPrevDevices]
					idx := i
					for j := 0; j < framesUsed; j++ {
						dc[j] = rawBuffer[idx]
						idx += dev.nchan
					}
				}
				totalBytes += nb
			}
			timeDiff := lastSampleTime.Sub(as.lastread)
			if timeDiff > 2*as.readPeriod {
				fmt.Println("timeDiff in abaco reader", timeDiff)
			}
			as.lastread = lastSampleTime

			if len(as.buffersChan) == cap(as.buffersChan) {
				panic(fmt.Sprintf("internal buffersChan full, len %v, capacity %v", len(as.buffersChan), cap(as.buffersChan)))
			}
			// log.Printf("About to send on buffersChan")
			as.buffersChan <- AbacoBuffersType{
				datacopies:     datacopies,
				lastSampleTime: lastSampleTime,
				timeDiff:       timeDiff,
				totalBytes:     totalBytes,
			}
			// log.Printf("Sent something on buffersChan (%d bytes)", totalBytes)
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
			// This select statement was formerly the ls.blockingRead method
			select {
			case <-time.After(panicTime):
				panic(fmt.Sprintf("timeout, no data from Abaco after %v / readPeriod is %v", panicTime, as.readPeriod))

			case buffersMsg, ok := <-as.buffersChan:
				//  Check is buffersChan closed? Recognize that by receiving zero values and/or being drained.
				if buffersMsg.datacopies == nil || !ok {
					block := new(dataBlock)
					if err := as.stop(); err != nil {
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

	for channelIndex := 0; channelIndex < nchan; channelIndex++ {
		data := datacopies[channelIndex]
		seg := DataSegment{
			rawData:         data,
			framesPerSample: 1, // This will be changed later if decimating
			framePeriod:     as.samplePeriod,
			firstFramenum:   as.nextFrameNum,
			firstTime:       firstTime,
		}
		block.segments[channelIndex] = seg
		block.nSamp = len(data)
	}
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

// stop ends the data streaming on all active abaco devices.
func (as *AbacoSource) stop() error {
	// loop over as.active and do any needed stopping functions.
	for _, dev := range as.active {
		dev.ring.Close()
	}
	return nil
}
