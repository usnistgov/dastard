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

// AbacoDevice represents a single shared-memory ring buffer
// that stores an Abaco card's data.
type AbacoDevice struct {
	cardnum    int
	nchan      int
	firstchan  int
	packetSize int // packet size, in bytes
	ring       *ringbuffer.RingBuffer
	unwrap     []*PhaseUnwrapper
}

const maxAbacoCards = 4 // Don't allow more than this many cards.
const abacoBitsToDrop = 1

// NewAbacoDevice creates a new AbacoDevice and opens the underlying file for reading.
func NewAbacoDevice(cardnum int) (dev *AbacoDevice, err error) {
	// Allow negative cardnum values, but for testing only!
	if cardnum >= maxAbacoCards {
		return nil, fmt.Errorf("NewAbacoDevice() got cardnum=%d, want [0,%d]",
			cardnum, maxAbacoCards-1)
	}
	dev = new(AbacoDevice)
	dev.cardnum = cardnum

	shmNameBuffer := fmt.Sprintf("xdma%d_c2h_0_buffer", dev.cardnum)
	shmNameDesc := fmt.Sprintf("xdma%d_c2h_0_description", dev.cardnum)
	if dev.ring, err = ringbuffer.NewRingBuffer(shmNameBuffer, shmNameDesc); err != nil {
		return nil, err
	}
	return dev, nil
}

// ReadAllPackets returns an array of *packet.Packet, as read from the device's RingBuffer.
func (device *AbacoDevice) ReadAllPackets() ([]*packets.Packet, error) {
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

// sampleCard samples the data from a single card to scan enough packets to
// know the number of channels, data rate, etc.
// Although it slows things down, it's best to discard all data in the ring
// at the time we open it, because we have no idea how old the data are.
func (device *AbacoDevice) sampleCard() error {
	// Open the device and discard whatever is in the buffer
	if err := device.ring.Open(); err != nil {
		return err
	}
	if err := device.ring.DiscardAll(); err != nil {
		return err
	}

	// Now get the data we actually want. Run for at least a minimum time
	// or a minimum number of packets.
	device.packetSize = int(device.ring.PacketSize())
	const minPacketsToRead = 100 // Not sure this is a good minimum
	maxDelay := time.Duration(200 * time.Millisecond)
	timeOut := time.NewTimer(maxDelay)
	for packetsRead := 0; packetsRead < minPacketsToRead; {
		select {
		case <-timeOut.C:
			fmt.Println("AbacoDevice.sampleCard() timer expired")
			break

		default:
			time.Sleep(5 * time.Millisecond)
			allPackets, err := device.ReadAllPackets()
			if err != nil {
				return nil
			}
			packetsRead += len(allPackets)

			// Do something with Packet.ChannelInfo() here: set device.nchan and
			// firstchan based on the values here, if they are larger/smaller than
			// any previously seen.
			device.nchan = 0
			device.firstchan = 99999999
			for _, p := range allPackets {
				nchan, offset := p.ChannelInfo()
				if offset < device.firstchan {
					device.firstchan = offset
				}
				if nchan+offset > device.nchan {
					device.nchan = nchan + offset
				}
			}
		}
	}

	device.unwrap = make([]*PhaseUnwrapper, device.nchan)
	for i := range device.unwrap {
		device.unwrap[i] = NewPhaseUnwrapper(abacoBitsToDrop)
	}
	return nil
}

// enumerateAbacoDevices returns a list of abaco device numbers that exist
// in the devfs. If /dev/xdma0_c2h_X exists, then X is added to the list.
// Does not yet handle cards other than xdma0.
func enumerateAbacoDevices() (devices []int, err error) {
	for cnum := 0; cnum < maxAbacoCards; cnum++ {
		name := fmt.Sprintf("xdma%d_c2h_0_description", cnum)
		if region, err := shm.Open(name, os.O_RDONLY, 0600); err == nil {
			region.Close()
			devices = append(devices, cnum)
		}
	}
	fmt.Printf("Abaco devices: %v\n", devices)
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

	for _, cnum := range deviceCodes {
		ad, err := NewAbacoDevice(cnum)
		if err != nil {
			log.Printf("warning: failed to create ring buffer for shm:xdma%d_c2h_0, though it should exist", cnum)
			continue
		}
		source.devices[cnum] = ad
		source.Ndevices++
	}
	if source.Ndevices == 0 && len(deviceCodes) > 0 {
		return source, fmt.Errorf("could not create ring buffer for any of shm:xdma*_c2h_0, though deviceCodes %v exist", deviceCodes)
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
	// Update the slice AvailableCards.
	config.AvailableCards = make([]int, 0)
	for k := range as.devices {
		config.AvailableCards = append(config.AvailableCards, k)
	}
	sort.Ints(config.AvailableCards)

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
		return fmt.Errorf("No Abaco devices are active")
	}
	for _, device := range as.active {
		if err := device.sampleCard(); err != nil {
			return err
		}
		as.nchan += device.nchan
	}
	as.sampleRate = 125000.0 // HACK! For now, assume a value.
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
					fmt.Printf("AbacoDevice.ReadAllPackets failed with error: %v\n", err)
					panic("AbacoDevice.ReadAllPackets failed")
				}
				log.Printf("Read Abaco device %v, total of %d packets", dev, len(allPackets))

				for _, p := range allPackets {
					switch d := p.Data.(type) {
					case []int32:
						framesUsed += len(d) / dev.nchan

					case []int16:
						framesUsed += len(d) / dev.nchan
						// TODO: this will break if multiple offsets are in the data.
						// fmt.Printf("Found [%d]int16 payload = %d frames: %v\n",
						// 	len(d), framesUsed, d[:5])

					default:
						panic("Cannot parse packets that aren't of type []int16 or []int32")
					}
				}

				// This is the demultiplexing step. Loops over channels,
				// then over frames.

				// Encoding note May 7, 2019:
				// Abaco raw data are 32 bit data in the form of binary number:
				// (MSB) wwwwwwf ffffffff ffffffff fffffssr (LSB)
				// w = whole number of phi0 (7)
				// f = fractions of a phi0 (22)
				// s = sync bits (2)
				// r = frame bit (1)
				// Our plan: omit the 12 lowest bits, giving us wwwfffff ffffffff.
				// Reserving 3 upper whole-nuber bits lets us phase unwrap 8 full times.
				// int32Buffer := bytesToInt32(bytesData)
				for i := 0; i < dev.nchan; i++ {
					datacopies[i+nchanPrevDevices] = make([]RawType, 0, framesUsed)
				}

				for _, p := range allPackets {
					nchan, offset := p.ChannelInfo()
					if offset < 0 || offset+nchan > dev.nchan {
						//panic?
						continue
					}

					switch d := p.Data.(type) {
					case []int16:
						// This loop order was faster than the reverse, before packets:
						for j, val := range d {
							idx := j%nchan + offset + nchanPrevDevices
							// dc[j] = RawType(int32Buffer[idx] >> 12)
							datacopies[idx] = append(datacopies[idx], RawType(val))
						}
						totalBytes += 2 * len(d)

					default:
						panic("Cannot parse packets that aren't of type []int16")
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

// closeDevices ends closes the ring buffers of all active AbacoDevice objects.
func (as *AbacoSource) closeDevices() error {
	// loop over as.active and do any needed stopping functions.
	for _, dev := range as.active {
		dev.ring.Close()
	}
	return nil
}
