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
	clockMhz  int
	card      *lancero.Lancero
}

// LanceroSource is a DataSource that reads 1 or more lancero devices.
type LanceroSource struct {
	devices []LanceroDevice
	ncards  int
	AnySource
}

// NewLanceroSource creates a new LanceroSource.
func NewLanceroSource() (*LanceroSource, error) {
	source := new(LanceroSource)
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
		source.devices = append(source.devices, ld)
		source.ncards++
	}
	return source, nil
}

// LanceroSourceConfig holds the arguments needed to call LanceroSource.Configure by RPC
type LanceroSourceConfig struct {
	FiberMask uint32
	CardDelay int
	ClockMhz  int
}

// Configure sets up the internal buffers with given size, speed, and min/max.
func (ts *LanceroSource) Configure(config *LanceroSourceConfig) error {
	// if config.Nchan < 1 {
	// 	return fmt.Errorf("LanceroSource.Configure() asked for %d channels, should be > 0", config.Nchan)
	// }
	// ts.nchan = config.Nchan
	// ts.sampleRate = config.SampleRate
	// ts.minval = config.Min
	// ts.maxval = config.Max
	//
	// ts.output = make([]chan DataSegment, ts.nchan)
	// for i := 0; i < ts.nchan; i++ {
	// 	ts.output[i] = make(chan DataSegment, 1)
	// }
	//
	// nrise := ts.maxval - ts.minval
	// ts.cycleLen = 2 * int(nrise)
	// ts.onecycle = make([]RawType, ts.cycleLen)
	// var i RawType
	// for i = 0; i < nrise; i++ {
	// 	ts.onecycle[i] = ts.minval + i
	// 	ts.onecycle[int(i)+int(nrise)] = ts.maxval - i
	// }
	// cycleTime := float64(ts.cycleLen) / ts.sampleRate
	// ts.timeperbuf = time.Duration(float64(time.Second) * cycleTime)
	return nil
}

// Sample determines key data facts by sampling some initial data.
// It's a no-op for simulated (software) sources
func (ts *LanceroSource) Sample() error {
	return nil
}

// StartRun tells the hardware to switch into data streaming mode.
// It's a no-op for simulated (software) sources
func (ts *LanceroSource) StartRun() error {
	return nil
}

// blockingRead blocks and then reads data when "enough" is ready.
func (ts *LanceroSource) blockingRead() error {
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
