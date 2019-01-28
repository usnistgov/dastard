package dastard

import (
	"fmt"
	"log"
	"os"
)

// AbacoDevice represents a single Abaco device-special file.
type AbacoDevice struct {
	devnum int
}

// AbacoSource represents all Abaco devices that supply data.
type AbacoSource struct {
	devices map[int]*AbacoDevice
	ncards  int
	active  []*AbacoDevice
	AnySource
}

// NewAbacoSource creates a new AbacoSource.
func NewAbacoSource() (*AbacoSource, error) {
	source := new(AbacoSource)
	source.name = "Abaco"
	source.devices = make(map[int]*AbacoDevice)

	devnums, err := enumerateAbacoDevices()
	if err != nil {
		return source, err
	}

	for _, dnum := range devnums {
		ad := AbacoDevice{devnum: dnum}
		// Need to
		// lan, err := lancero.NewLancero(dnum)
		err := error(nil)
		if err != nil {
			log.Printf("warning: failed to open /dev/xdma0_c2h_%d", dnum)
			continue
		}
		// ad.card = lan
		source.devices[dnum] = &ad
		source.ncards++
	}
	if source.ncards == 0 && len(devnums) > 0 {
		return source, fmt.Errorf("could not open any of /dev/xdma0_c2h_*, though devnums %v exist", devnums)
	}
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
	return nil
}

// Sample determines key data facts by sampling some initial data.
func (as *AbacoSource) Sample() error {
	return nil
}

// StartRun tells the hardware to switch into data streaming mode.
// For Abaco µMUX systems, we need to consume any initial data that constitutes
// a fraction of a frame.
func (as *AbacoSource) StartRun() error {
	return nil
}

// stop ends the data streaming on all active lancero devices.
func (as *AbacoSource) stop() error {
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
