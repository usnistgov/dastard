package dastard

import (
	"fmt"
	"io"
	"time"
)

// TriangleSource is a DataSource that synthesizes triangle waves.
type TriangleSource struct {
	sampleRate float64 // samples per second
	nchan      int     // how many channels to provide
	minval     RawType
	maxval     RawType
	lastread   time.Time
	timeperbuf time.Duration
	output     []chan DataSegment
	onecycle   []RawType
	cycleLen   int
}

// NewTriangleSource creates a new TriangleSource with given size, speed, and min/max.
func NewTriangleSource(nchan int, rate float64, min, max RawType) *TriangleSource {
	ts := new(TriangleSource)
	ts.sampleRate = rate
	ts.nchan = nchan
	ts.minval = min
	ts.maxval = max
	return ts
}

// Configure sets up the internal buffers.
func (ts *TriangleSource) Configure() error {
	ts.output = make([]chan DataSegment, ts.nchan)
	for i := 0; i < ts.nchan; i++ {
		ts.output[i] = make(chan DataSegment, 1)
	}

	nrise := ts.maxval - ts.minval
	ts.cycleLen = 2 * int(nrise)
	ts.onecycle = make([]RawType, ts.cycleLen)
	var i RawType
	for i = 0; i < nrise; i++ {
		ts.onecycle[i] = ts.minval + i
		ts.onecycle[int(i)+int(nrise)] = ts.maxval - i
	}
	cycleTime := float64(ts.cycleLen) / ts.sampleRate
	ts.timeperbuf = time.Duration(float64(time.Second) * cycleTime)
	return nil
}

// Start begins the data supply.
func (ts *TriangleSource) Start() error {
	ts.lastread = time.Now()
	return nil
}

// Stop ends the data supply.
func (ts *TriangleSource) Stop() error {
	return nil
}

// Sample pre-samples the hardware data to see what's in it.
// Does nothing for a TriangleSource.
func (ts *TriangleSource) Sample() error {
	return nil
}

// BlockingRead blocks and then reads data when "enough" is ready.
func (ts *TriangleSource) BlockingRead(abort <-chan struct{}) error {
	nextread := ts.lastread.Add(ts.timeperbuf)
	waittime := time.Until(nextread)
	if waittime > 0 {
		select {
		case <-abort:
			return io.EOF
		case <-time.After(waittime):
			ts.lastread = time.Now()
		}
	}

	for _, ch := range ts.output {
		datacopy := make([]RawType, ts.cycleLen)
		copy(datacopy, ts.onecycle)
		seg := DataSegment{rawData: datacopy, framesPerSample: 1}
		ch <- seg
	}

	return nil
}

// Outputs returns the slice of channels that carry buffers of data for downstream processing.
func (ts *TriangleSource) Outputs() []chan DataSegment {
	result := make([]chan DataSegment, ts.nchan)
	copy(result, ts.output)
	return result
}

// SimPulseSource simulates simple pulsed sources
type SimPulseSource struct {
	nchan      int     // how many channels to provide
	sampleRate float64 // samples per second
	lastread   time.Time
	timeperbuf time.Duration
	output     []chan DataSegment
	onecycle   []RawType
	cycleLen   int

	// regular bool // whether pulses are regular or Poisson-distributed

}

// NewSimPulseSource creates a new SimPulseSource with given size, speed.
func NewSimPulseSource() *SimPulseSource {
	ps := new(SimPulseSource)
	// At this point, no invariants to enforce
	return ps
}

// Configure sets up the internal buffers with given size, speed, and pedestal and amplitude.
func (sps *SimPulseSource) Configure(nchan int, rate, pedestal, amplitude float64, nsamp int) error {
	sps.sampleRate = rate
	sps.nchan = nchan

	sps.output = make([]chan DataSegment, sps.nchan)
	for i := 0; i < sps.nchan; i++ {
		sps.output[i] = make(chan DataSegment, 1)
	}

	sps.cycleLen = nsamp
	firstIdx := 4096
	// nrise := sps.maxval - sps.minval
	// sps.cycleLen = 2 * int(nrise)
	sps.onecycle = make([]RawType, sps.cycleLen)

	ampl := []float64{amplitude, -amplitude}
	exprate := []float64{.999, .98}
	value := pedestal
	for i := 0; i < sps.cycleLen; i++ {
		if i >= firstIdx {
			value = pedestal + ampl[0] + ampl[1]
			ampl[0] *= exprate[0]
			ampl[1] *= exprate[1]
		}
		sps.onecycle[i] = RawType(value + 0.5)
	}

	cycleTime := float64(sps.cycleLen) / sps.sampleRate
	sps.timeperbuf = time.Duration(float64(time.Second) * cycleTime)
	fmt.Printf("made a simulated pulse source for %d channels.\n", nchan)
	fmt.Printf("configured with wait time of %v\n", sps.timeperbuf)
	return nil
}

// Start begins the data supply.
func (sps *SimPulseSource) Start() error {
	sps.lastread = time.Now()
	return nil
}

// Stop ends the data supply.
func (sps *SimPulseSource) Stop() error {
	return nil
}

// Sample pre-samples the hardware data to see what's in it.
// Does nothing for a SimPulseSource.
func (sps *SimPulseSource) Sample() error {
	return nil
}

// Outputs returns the slice of channels that carry buffers of data for downstream processing.
func (sps *SimPulseSource) Outputs() []chan DataSegment {
	result := make([]chan DataSegment, sps.nchan)
	copy(result, sps.output)
	return result
}

// BlockingRead blocks and then reads data when "enough" is ready.
func (sps *SimPulseSource) BlockingRead(abort <-chan struct{}) error {
	nextread := sps.lastread.Add(sps.timeperbuf)
	waittime := time.Until(nextread)
	if waittime > 0 {
		// fmt.Printf("Waiting %v to read\n", waittime)
		select {
		case <-abort:
			fmt.Println("aborted the read")
			return io.EOF
		case <-time.After(waittime):
			sps.lastread = time.Now()
		}
	}

	for _, ch := range sps.output {
		datacopy := make([]RawType, sps.cycleLen)
		copy(datacopy, sps.onecycle)
		seg := DataSegment{rawData: datacopy, framesPerSample: 1}
		ch <- seg
	}

	return nil
}
