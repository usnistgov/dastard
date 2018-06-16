package dastard

import (
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"time"

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
	card        *lancero.Lancero
}

// LanceroSource is a DataSource that handles 1 or more lancero devices.
type LanceroSource struct {
	devices           map[int]*LanceroDevice
	ncards            int
	clockMhz          int
	active            []*LanceroDevice
	chan2readoutOrder map[int]int
	AnySource
}

// NewLanceroSource creates a new LanceroSource.
func NewLanceroSource() (*LanceroSource, error) {
	source := new(LanceroSource)
	source.devices = make(map[int]*LanceroDevice)
	source.chan2readoutOrder = make(map[int]int)
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

// LanceroSourceConfig holds the arguments needed to call LanceroSource.Configure by RPC.
// For now, we'll make the mask and card delay equal for all cards. That need not
// be permanent, but I do think ClockMhz is necessarily the same for all cards.
type LanceroSourceConfig struct {
	FiberMask      uint32
	ClockMhz       int
	CardDelay      []int
	ActiveCards    []int
	AvailableCards []int
}

// Configure sets up the internal buffers with given size, speed, and min/max.
func (ls *LanceroSource) Configure(config *LanceroSourceConfig) error {
	ls.active = make([]*LanceroDevice, 0)
	ls.clockMhz = config.ClockMhz
	for i, c := range config.ActiveCards {
		dev := ls.devices[c]
		if dev == nil {
			continue
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
	return nil
}

// updateChanOrderMap updates the map chan2readoutOrder based on the number
// of columns and rows in each active device
func (ls *LanceroSource) updateChanOrderMap() {
	ls.chan2readoutOrder = make(map[int]int)
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

// Sample determines key data facts by sampling some initial data.
// It's a no-op for simulated (software) sources
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
	ls.updateChanOrderMap()

	ls.chanNames = make([]string, ls.nchan)
	for i := 1; i < ls.nchan; i += 2 {
		ls.chanNames[i-1] = fmt.Sprintf("err%d", 1+i/2)
		ls.chanNames[i] = fmt.Sprintf("chan%d", 1+i/2)
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
	const tooManyBytes int = 1000000 // shouldn't need this many bytes to SampleData
	for {
		if bytesRead >= tooManyBytes {
			return fmt.Errorf("LanceroDevice.sampleCard read %d bytes, failed to find nrow*ncol",
				bytesRead)
		}
		select {
		case <-interruptCatcher:
			return fmt.Errorf("LanceroDevice.sampleCard was interrupted")
		default:
			_, waittime, err := lan.Wait()
			if err != nil {
				return err
			}
			buffer, err := lan.AvailableBuffers()
			if err != nil {
				return err
			}
			totalBytes := len(buffer)
			if totalBytes > 45000 {
				fmt.Printf("waittime: %v\n", waittime)
				fmt.Printf("Found buffers with %9d total bytes, bytes read previously=%10d\n", totalBytes, bytesRead)
				q, p, n, err := lancero.FindFrameBits(buffer)
				bytesPerFrame := 4 * (p - q)
				if err != nil {
					fmt.Println("Error in findFrameBits:", err)
					break
				}
				device.ncols = n
				device.nrows = (p - q) / n
				periodNS := waittime.Nanoseconds() / (int64(totalBytes) / int64(bytesPerFrame))
				device.lsync = roundint((float64(periodNS) / 1000) * float64(device.clockMhz) / float64(device.nrows))
				device.frameSize = device.ncols * device.nrows * 4

				fmt.Printf("cols=%d  rows=%d  frame period %5d ns, lsync=%d\n", device.ncols,
					device.nrows, periodNS, device.lsync)

				lan.ReleaseBytes(totalBytes)
				return nil
			}
			lan.ReleaseBytes(totalBytes)
			bytesRead += totalBytes
		}
	}
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
		for {
			if _, _, err := lan.Wait(); err != nil {
				return fmt.Errorf("error in Wait: %v", err)
			}
			bytes, err := lan.AvailableBuffers()
			if err != nil {
				return fmt.Errorf("error in AvailableBuffers: %v", err)
			}
			if len(bytes) <= 0 {
				continue
			}

			firstWord, _, _, err := lancero.FindFrameBits(bytes)
			if err == nil && firstWord > 0 {
				bytesToRelease := 4 * firstWord
				// bytesToRelease += ((len(bytes) - 4*firstWord) / device.frameSize) * device.frameSize
				fmt.Printf("First frame bit at word %d, so release %d of %d bytes\n", firstWord, bytesToRelease, len(bytes))
				lan.ReleaseBytes(bytesToRelease)
				break
			}
		}
	}
	return nil
}

// blockingRead blocks and then reads data when "enough" is ready.
// This will need to somehow work across multiple cards???
func (ls *LanceroSource) blockingRead() error {
	type waiter struct {
		timestamp time.Time
		duration  time.Duration
		err       error
	}
	done := make(chan waiter)
	dev := ls.active[0]
	go func() {
		timestamp, duration, err := dev.card.Wait()
		done <- waiter{timestamp, duration, err}
	}()

	select {
	case <-ls.abortSelf:
		if err := ls.stop(); err != nil {
			return err
		}
		return io.EOF
	case result := <-done:
		if result.err != nil {
			return result.err
		}
		timediff := result.timestamp.Sub(ls.lastread)
		ls.lastread = result.timestamp
		ls.distributeData(result.timestamp, timediff)
	}
	return nil
}

func (ls *LanceroSource) distributeData(timestamp time.Time, wait time.Duration) {

	// Get 1 buffer per card, and compute which contains the fewest frames
	framesUsed := math.MaxInt64
	var buffers [][]RawType
	for _, dev := range ls.active {
		b, err := dev.card.AvailableBuffers()
		if err != nil {
			log.Printf("Warning: AvailableBuffers failed")
			return
		}
		buffers = append(buffers, bytesToRawType(b))

		bframes := len(b) / dev.frameSize
		if bframes < framesUsed {
			framesUsed = bframes
		}
		// rate := 0.0
		// if wait > 0 {
		// 	rate = float64(len(b)) * 1e3 / float64(wait)
		// }
		// fmt.Printf("new buffer of length %8d b after wait %6.2f ms for %8.2f Mb/s\n", len(b), .001*float64(wait/time.Microsecond), rate)
	}
	if framesUsed <= 0 {
		fmt.Printf("Nothing to consume, buffer[0] size: %d samples\n", len(buffers[0]))
		return
	}

	// Consume framesUsed frames of data from each channel
	datacopies := make([][]RawType, len(ls.output))

	// Careful! This slice of slices will be in lancero READOUT order:
	// r0c0, r0c1, r0c2, etc.
	for i := range ls.output {
		datacopies[i] = make([]RawType, framesUsed)
	}

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

	// Now send these data downstream. Here we permute data into the expected
	// channel ordering: r0c0, r1c0, r2c0, etc via the chan2readoutOrder map.
	// Backtrack to find the time associated with the first sample.
	segDuration := time.Duration(framesUsed * roundint(1e9/ls.sampleRate))
	firstTime := ls.lastread.Add(-segDuration)
	for channum, ch := range ls.output {
		data := datacopies[ls.chan2readoutOrder[channum]]
		// mask out frame and extern trigger bits from FB channels
		if channum%2 == 1 {
			const mask = ^RawType(0x3)
			for j := 0; j < len(data); j++ {
				data[j] &= mask
			}
			// TODO: I think err->FB mixing goes here??
		}

		// TODO: replace framesPerSample=1 with the actual decimation level
		seg := DataSegment{
			rawData:         data,
			framesPerSample: 1,
			firstFramenum:   ls.nextFrameNum,
			firstTime:       firstTime,
		}
		ch <- seg
	}
	ls.nextFrameNum += FrameIndex(framesUsed)

	// Inform the driver to release the data we just consumed
	for _, dev := range ls.active {
		release := framesUsed * dev.frameSize
		dev.card.ReleaseBytes(release)
		// fmt.Printf("           releasing %8d b (%5d b remain)\n", release, 2*len(buffers[i])-release)
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
