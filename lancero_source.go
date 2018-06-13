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
	devices  map[int]*LanceroDevice
	ncards   int
	clockMhz int
	active   []*LanceroDevice
	AnySource
}

// NewLanceroSource creates a new LanceroSource.
func NewLanceroSource() (*LanceroSource, error) {
	source := new(LanceroSource)
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

// LanceroSourceConfig holds the arguments needed to call LanceroSource.Configure by RPC.
// For now, we'll make the mask and card delay equal for all cards. That need not
// be permanent, but I do think ClockMhz is necessarily the same for all cards.
type LanceroSourceConfig struct {
	FiberMask      uint32
	CardDelay      int
	ClockMhz       int
	ActiveCards    []int
	AvailableCards []int
}

// Configure sets up the internal buffers with given size, speed, and min/max.
func (ls *LanceroSource) Configure(config *LanceroSourceConfig) error {
	ls.active = make([]*LanceroDevice, 0)
	ls.clockMhz = config.ClockMhz
	for _, c := range config.ActiveCards {
		dev := ls.devices[c]
		if dev == nil {
			continue
		}
		ls.active = append(ls.active, dev)
		dev.cardDelay = config.CardDelay
		dev.fiberMask = config.FiberMask
		dev.clockMhz = config.ClockMhz
	}
	config.AvailableCards = make([]int, 0)
	for k := range ls.devices {
		config.AvailableCards = append(config.AvailableCards, k)
	}
	return nil
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
				device.lsync = int(math.Round(float64(periodNS) * 1e-3 * float64(device.clockMhz) / float64(device.nrows)))
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

// StartRun tells the hardware to switch into data streaming mode.
// For lancero TDM systems, we need to consume any initial data that constitutes
// a fraction of a frame.
func (ls *LanceroSource) StartRun() error {

	// Starting the source for all active cards has 3 steps per card.
	for _, device := range ls.active {
		lan := device.card

		// 1. Resize the ring buffer to hold 16,384 frames
		if device.frameSize <= 0 {
			device.frameSize = 128 // a random guess
		}
		thresh := 8192 * device.frameSize
		if err := lan.ChangeRingBuffer(8*thresh, thresh); err != nil {
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

		// 3. Consume possibly fractional frame info
		if _, _, err := lan.Wait(); err != nil {
			return fmt.Errorf("error in Wait: %v", err)
		}
		bytes, err := lan.AvailableBuffers()
		if err != nil {
			return fmt.Errorf("error in AvailableBuffers: %v", err)
		}

		// 3a. Consume any words with frame bit set (in case you missed some)
		firstWord, _, _, _ := lancero.FindFrameBits(bytes)
		bytesToRelease := 4 * firstWord
		// bytesToRelease += ((len(bytes) - 4*firstWord) / device.frameSize) * device.frameSize
		fmt.Printf("First frame bit at word %d, so release %d of %d bytes\n", firstWord, bytesToRelease, len(bytes))
		lan.ReleaseBytes(bytesToRelease)
	}
	return nil
}

// blockingRead blocks and then reads data when "enough" is ready.
// This will need to somehow work across multiple cards???
func (ls *LanceroSource) blockingRead() error {
	period := 100 * time.Millisecond
	nextread := ls.lastread.Add(period)
	waittime := time.Until(nextread)

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
	case <-time.After(waittime):
		ls.lastread = time.Now()
	case result := <-done:
		if result.err != nil {
			return result.err
		}
		fmt.Printf("time to distribute data after wait of %v\n", result.duration)
		ls.distributeData(result.timestamp)
	}
	return nil
}

func (ls *LanceroSource) distributeData(timestamp time.Time) {

	// Get 1 buffer per card, and compute which contains the fewest frames
	minframes := math.MaxInt64
	var buffers [][]RawType
	for _, dev := range ls.active {
		b, err := dev.card.AvailableBuffers()
		if err != nil {
			log.Printf("Warning: AvailableBuffers failed")
			return
		}
		buffers = append(buffers, bytesToRawType(b))
		fmt.Printf("new buffer of length %d bytes\n", len(b))
		lb := 128
		if len(b) < lb {
			lb = len(b)
		}
		for j := 0; j < lb; j += 4 {
			p := uint64(b[j+3])
			for i := 2; i >= 0; i-- {
				p = 256*p + uint64(b[j+i])
			}
			fmt.Printf("%8.8x ", p)
			if j%32 == 28 {
				fmt.Println()
			}
		}

		bframes := len(b) / dev.frameSize
		if bframes < minframes {
			minframes = bframes
		}
	}
	if minframes <= 0 {
		fmt.Printf("Nothing to consume, buffer[0] size: %d samples\n", len(buffers[0]))
		return
	}

	// Consume minframes frames of data from each channel
	ch0num := 0
	datacopies := make([][]RawType, len(ls.output))
	for i := range ls.output {
		datacopies[i] = make([]RawType, minframes)
	}

	for ibuf, dev := range ls.active {
		nchan := dev.ncols * dev.nrows * 2
		buffer := buffers[ibuf]
		idx := 0
		for j := 0; j < minframes; j++ {
			for i := 0; i < nchan; i++ {
				datacopies[i+ch0num][j] = buffer[idx]
				idx++
			}
		}
		ch0num += nchan
	}

	// Now send these data downstream
	for i, ch := range ls.output {
		// mask out frame and extern trigger bits from FB channels
		// if i%2 == 1 {
		// 	const mask = ^RawType(0x3)
		// 	for j := 0; j < len(datacopies[i]); j++ {
		// 		datacopies[i][j] &= mask
		// 	}
		// I think you'd do err->FB mixing here??
		// }
		if i <= 1 && len(datacopies[i]) > 31 {
			fmt.Printf("datacopies[%d][ 0: 8]: %v\n", i, datacopies[i][0:8])
			fmt.Printf("datacopies[%d][ 8:16]: %v\n", i, datacopies[i][8:16])
			fmt.Printf("datacopies[%d][16:24]: %v\n", i, datacopies[i][16:24])
			fmt.Printf("datacopies[%d][24:32]: %v\n", i, datacopies[i][24:32])
		}
		seg := DataSegment{rawData: datacopies[i], framesPerSample: 1}
		ch <- seg
	}

	// Inform the driver to release the data we just consumed
	for _, dev := range ls.active {
		dev.card.ReleaseBytes(minframes * dev.frameSize)
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
