package dastard

import (
	"log"

	"github.com/usnistgov/dastard/lancero"
)

// LanceroDevice represents one lancero device.
type LanceroDevice struct {
	devnum    int
	nrows     int
	ncols     int
	fiberMask uint32
	cardDelay int
	card      *lancero.Lancero
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
	ls.clockMhz = config.ClockMhz
	ls.active = make([]*LanceroDevice, 0)
	for _, c := range config.ActiveCards {
		dev := ls.devices[c]
		if dev == nil {
			continue
		}
		ls.active = append(ls.active, dev)
		dev.cardDelay = config.CardDelay
		dev.fiberMask = config.FiberMask
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
	return nil
}

// StartRun tells the hardware to switch into data streaming mode.
// It's a no-op for simulated (software) sources
func (ls *LanceroSource) StartRun() error {
	return nil
}

// blockingRead blocks and then reads data when "enough" is ready.
func (ls *LanceroSource) blockingRead() error {
	// nextread := ts.lastread.Add(ts.timeperbuf)
	// waittime := time.Until(nextread)
	// select {
	// case <-ts.abortSelf:
	// 	return io.EOF
	// case <-time.After(waittime):
	// 	ts.lastread = time.Now()
	// }
	//
	// for _, ch := range ts.output {
	// 	datacopy := make([]RawType, ts.cycleLen)
	// 	copy(datacopy, ts.onecycle)
	// 	seg := DataSegment{rawData: datacopy, framesPerSample: 1}
	// 	ch <- seg
	// }
	//
	return nil
}
