package dastard

import (
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"time"

	"github.com/usnistgov/dastard/lancero"
)

// AbacoDevice represents a single Abaco device-special file.
type AbacoDevice struct {
	cardnum    int
	devnum     int
	nrows      int
	ncols      int
	nchan      int
	frameSize  int // frame size, in bytes
	File       *os.File
	buffersize int
	buffer     []byte
}

// NewAbacoDevice creates a new AbacoDevice and opens the underlying file for reading
// For now, assume card 0 with channels 0,1,2,... We can add a second card later.
func NewAbacoDevice(devnum int) (dev *AbacoDevice, err error) {
	dev = new(AbacoDevice)
	dev.cardnum = 0 // Force card zero for now
	dev.devnum = devnum
	if devnum != 0 {
		return nil, fmt.Errorf("NewAbacoDevice only supports devnum=0")
	}

	// fname := fmt.Sprintf("/dev/xdma%d_c2h_%d", dev.cardnum, devnum)
	fname := fmt.Sprintf("/var/tmp/abacopipe")
	if dev.File, err = os.OpenFile(fname, os.O_RDONLY, 0666); err != nil {
		return nil, err
	}
	dev.buffersize = 1024 * 1024 // How to choose the size of the read buffer???
	dev.buffer = make([]byte, dev.buffersize)
	return dev, nil
}

// abacoFBOffset gives the location of the frame bit is in bytes 0, 4, 8...
const abacoFBOffset int = 0

// sampleCard samples the data from a single card to scan for frame bits.
// For Abaco µMUX systems, we need to discard one DMA buffer worth of stale data,
// then check the data after that for frame bit info.
// 1. Open the user file and query it for the data rate, number of channels (?),
//    firmware FIFO size, and firmware version.
// 2. Read at least 2 FIFOs plus enough data to discern frame bits
// 3. Discard 2 FIFOs
// 4. Scan for frame bits
// 5. Store info that we've learned in AbacoSource or per device.
func (device *AbacoDevice) sampleCard() error {
	// Read the device: two full buffers AND ignore the leading 2 FIFOs in the 2nd
	for i := 0; i < 2; i++ {
		nb, err := device.File.Read(device.buffer)
		if err != nil {
			return err
		}
		log.Print("Abaco bytes read: ", nb)
	}

	q, p, n, err3 := lancero.FindFrameBits(device.buffer[128*1024:], abacoFBOffset)
	if err3 == nil {
		device.ncols = n
		device.nrows = (p - q) / n
		device.nchan = device.ncols * device.nrows
		device.frameSize = device.ncols * device.nrows * 4
	} else {
		fmt.Printf("Error in FindFrameBits: %v", err3)
	}
	return nil
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

	devnums, err := enumerateAbacoDevices()
	fmt.Printf("enumerateAbacoDevices returns %v\n", devnums)
	if err != nil {
		return source, err
	}

	for _, dnum := range devnums {
		ad, err := NewAbacoDevice(dnum)
		if err != nil {
			log.Printf("warning: failed to open /dev/xdma0_c2h_%d", dnum)
			continue
		}
		// cardnum = 0 // For now, only card 0 is allowed.
		source.devices[dnum] = ad
		source.Ndevices++
	}
	if source.Ndevices == 0 && len(devnums) > 0 {
		return source, fmt.Errorf("could not open any of /dev/xdma0_c2h_*, though devnums %v exist", devnums)
	}
	fmt.Printf("NewAbacoSource has %d devices\n", source.Ndevices)
	return source, nil
}

// Delete closes all Abaco cards
func (as *AbacoSource) Delete() {
	for i := range as.devices {
		fmt.Printf("Closing device %d.\n", i)
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
		if c < 0 || c >= len(as.devices) {
			log.Printf("Warning: could not activate device %d, as there are only 0-%d\n",
				c, len(as.devices)-1)
			continue
		}
		dev := as.devices[c]
		if dev == nil {
			err = fmt.Errorf("i=%v, c=%v, device == nil", i, c)
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
	if len(as.active) <= 0 {
		return fmt.Errorf("No Abaco devices are active")
	}
	for _, device := range as.active {
		err := device.sampleCard()
		if err != nil {
			return err
		}
		as.nchan += device.ncols * device.nrows
	}

	return nil
}

// StartRun tells the hardware to switch into data streaming mode.
// For Abaco µMUX systems, we need to consume any initial data that constitutes
// a fraction of a frame. Then launch a goroutine to consume data.
func (as *AbacoSource) StartRun() error {
	// There's no data streaming mode on Abaco, so no need to start it?
	// TODO: Consume data until start of a frame is found.
	as.launchAbacoReader()
	return nil
}

// AbacoBuffersType is an internal message type used to allow
// a goroutine to read from the Abaco card and put data on a buffered channel
type AbacoBuffersType struct {
	datacopies [][]RawType
	// lastSampleTime time.Time
	// timeDiff       time.Duration
	totalBytes int
}

//
func (as *AbacoSource) launchAbacoReader() {
	timeoutPeriod := 2 * time.Second
	go func() {
		as.buffersChan = make(chan AbacoBuffersType, 100)
		defer close(as.buffersChan)
		timeout := time.NewTicker(timeoutPeriod)

		for {
			go func() {
				// read from the data pipe
				// send filled buffer (and bytes actually read) on a channel
				framesUsed := math.MaxInt64
				totalBytes := 0
				datacopies := make([][]RawType, as.nchan)
				nchanPrevDevices := 0
				for _, dev := range as.active {
					nb, err := dev.File.Read(dev.buffer)
					if err != nil {
						panic("AbacoDevice.File.Read failed")
					}
					bframes := nb / dev.frameSize
					if bframes < framesUsed {
						framesUsed = bframes
					}

					localBuffer := make([]byte, nb)
					copy(localBuffer, dev.buffer[:nb])
					rawBuffer := bytesToRawType(localBuffer)
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
				if len(as.buffersChan) == cap(as.buffersChan) {
					panic(fmt.Sprintf("internal buffersChan full, len %v, capacity %v", len(as.buffersChan), cap(as.buffersChan)))
				}
				as.buffersChan <- AbacoBuffersType{
					datacopies: datacopies,
					// lastSampleTime: lastSampleTime,
					// timeDiff: timeDiff,
					totalBytes: totalBytes}
			}()
			select {
			case <-as.abortSelf:
				// close anything that needs closing
				return

			case <-timeout.C:
				// Handle failure to return
				log.Printf("Abaco read timed out")
				return

			case <-as.buffersChan:
				// Send the data on for further processing
			}
		}
	}()
}

// stop ends the data streaming on all active abaco devices.
func (as *AbacoSource) stop() error {
	// loop over as.active and do any needed stopping functions.
	return nil
}

// enumerateAbacoDevices returns a list of abaco device numbers that exist
// in the devfs. If /dev/xdma0_c2h_0 exists, then 0 is added to the list.
func enumerateAbacoDevices() (devices []int, err error) {
	MAXDEVICES := 8
	for id := 0; id < MAXDEVICES; id++ {
		name := fmt.Sprintf("/dev/xdma0_c2h_%d", id)
		info, err := os.Stat(name)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			} else {
				return devices, err
			}
		}
		if (info.Mode() & os.ModeDevice) != 0 {
			devices = append(devices, id)
		}
	}
	return devices, nil
}
