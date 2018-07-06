package dastard

import (
	"fmt"
	"io"
	"math/rand"
	"time"
)

// TriangleSource is a DataSource that synthesizes triangle waves.
type TriangleSource struct {
	minval     RawType
	maxval     RawType
	timeperbuf time.Duration
	onecycle   []RawType
	cycleLen   int
	AnySource
}

// NewTriangleSource creates a new TriangleSource.
func NewTriangleSource() *TriangleSource {
	ts := new(TriangleSource)
	ts.name = "Triangle"
	return ts
}

// TriangleSourceConfig holds the arguments needed to call TriangleSource.Configure by RPC
type TriangleSourceConfig struct {
	Nchan      int
	SampleRate float64
	Min, Max   RawType
}

// Configure sets up the internal buffers with given size, speed, and min/max.
func (ts *TriangleSource) Configure(config *TriangleSourceConfig) error {
	if config.Nchan < 1 {
		return fmt.Errorf("TriangleSource.Configure() asked for %d channels, should be > 0", config.Nchan)
	}
	ts.runMutex.Lock()
	defer ts.runMutex.Unlock()
	if config.Min > config.Max {
		return fmt.Errorf("have config.Min=%v > config.Max=%v, want Min<Max", config.Min, config.Max)
	}
	nrise := config.Max - config.Min
	if nrise > 0 {
		ts.cycleLen = 2 * int(nrise)
		ts.onecycle = make([]RawType, ts.cycleLen)
		var i RawType
		for i = 0; i < nrise; i++ {
			ts.onecycle[i] = config.Min + i
			ts.onecycle[int(i)+int(nrise)] = config.Max - i
		}
	} else if nrise == 0 {
		ts.cycleLen = roundint(ts.sampleRate/10) + 1 // aim for 10 cycles/second
		ts.onecycle = make([]RawType, ts.cycleLen)
		for i := range ts.onecycle {
			ts.onecycle[i] = config.Max
		}
	}
	ts.nchan = config.Nchan
	ts.sampleRate = config.SampleRate
	ts.minval = config.Min
	ts.maxval = config.Max
	cycleTime := float64(ts.cycleLen) / ts.sampleRate
	ts.timeperbuf = time.Duration(float64(time.Second) * cycleTime)
	return nil
}

// Sample determines key data facts by sampling some initial data.
// It's a no-op for simulated (software) sources
func (ts *TriangleSource) Sample() error {
	ts.chanNames = make([]string, ts.nchan)
	ts.signed = make([]bool, ts.nchan)
	ts.rowColCodes = make([]RowColCode, ts.nchan)
	for i := 0; i < ts.nchan; i++ {
		ts.chanNames[i] = fmt.Sprintf("chan%d", i+1)
	}
	return nil
}

// blockingRead blocks and then reads data when "enough" is ready.
func (ts *TriangleSource) blockingRead() error {
	ts.runMutex.Lock()
	defer ts.runMutex.Unlock()
	nextread := ts.lastread.Add(ts.timeperbuf)
	waittime := time.Until(nextread)
	var now time.Time
	select {
	case <-ts.abortSelf:
		return io.EOF
	case <-time.After(waittime):
		now = time.Now()
		if ts.heartbeats != nil {
			dt := now.Sub(ts.lastread).Seconds()
			mb := float64(ts.cycleLen*2*len(ts.output)) / 1e6
			ts.heartbeats <- Heartbeat{Running: true, Time: dt, DataMB: mb}
		}
		ts.lastread = nextread // ensure average cycle time is correct, using now would allow error to build up
	}

	// Backtrack to find the time associated with the first sample.
	firstTime := now.Add(-ts.timeperbuf) // use now here, this should acutually correspond to the time the data was read
	for _, ch := range ts.output {
		datacopy := make([]RawType, ts.cycleLen)
		copy(datacopy, ts.onecycle)
		seg := DataSegment{
			rawData:         datacopy,
			framesPerSample: 1,
			firstFramenum:   ts.nextFrameNum,
			firstTime:       firstTime,
		}
		ch <- seg
	}
	ts.nextFrameNum += FrameIndex(ts.cycleLen)

	return nil
}

// SimPulseSource simulates simple pulsed sources
type SimPulseSource struct {
	timeperbuf time.Duration
	onecycle   []RawType
	cycleLen   int
	AnySource

	// regular bool // whether pulses are regular or Poisson-distributed
}

// NewSimPulseSource creates a new SimPulseSource with given size, speed.
func NewSimPulseSource() *SimPulseSource {
	ps := new(SimPulseSource)
	ps.name = "SimPulse"
	return ps
}

// SimPulseSourceConfig holds the arguments needed to call SimPulseSource.Configure by RPC
type SimPulseSourceConfig struct {
	Nchan      int
	SampleRate float64
	Pedestal   float64
	Amplitude  float64
	Nsamp      int
}

// Configure sets up the internal buffers with given size, speed, and pedestal and amplitude.
func (sps *SimPulseSource) Configure(config *SimPulseSourceConfig) error {
	if config.Nchan < 1 {
		return fmt.Errorf("SimPulseSource.Configure() asked for %d channels, should be > 0", config.Nchan)
	}
	sps.nchan = config.Nchan
	sps.sampleRate = config.SampleRate

	sps.cycleLen = config.Nsamp
	firstIdx := 5
	sps.onecycle = make([]RawType, sps.cycleLen)

	ampl := []float64{config.Amplitude, -config.Amplitude}
	exprate := []float64{.999, .98}
	value := config.Pedestal
	for i := 0; i < sps.cycleLen; i++ {
		if i >= firstIdx {
			value = config.Pedestal + ampl[0] + ampl[1]
			ampl[0] *= exprate[0]
			ampl[1] *= exprate[1]
		}
		sps.onecycle[i] = RawType(value + 0.5)
	}

	cycleTime := float64(sps.cycleLen) / sps.sampleRate
	sps.timeperbuf = time.Duration(float64(time.Second) * cycleTime)
	// log.Printf("made a simulated pulse source for %d channels.\n", sps.nchan)
	// log.Printf("configured with wait time of %v\n", sps.timeperbuf)
	return nil
}

// Sample determines key data facts by sampling some initial data.
// It's a no-op for simulated (software) sources
func (sps *SimPulseSource) Sample() error {
	sps.chanNames = make([]string, sps.nchan)
	sps.signed = make([]bool, sps.nchan)
	sps.rowColCodes = make([]RowColCode, sps.nchan)
	for i := 0; i < sps.nchan; i++ {
		sps.chanNames[i] = fmt.Sprintf("chan%d", i+1)
	}
	return nil
}

// blockingRead blocks and then reads data when "enough" is ready.
func (sps *SimPulseSource) blockingRead() error {
	sps.runMutex.Lock()
	defer sps.runMutex.Unlock()
	nextread := sps.lastread.Add(sps.timeperbuf)
	waittime := time.Until(nextread)
	var now time.Time
	select {
	case <-sps.abortSelf:
		return io.EOF
	case <-time.After(waittime):
		now = time.Now()
		if sps.heartbeats != nil {
			dt := now.Sub(sps.lastread).Seconds()
			mb := float64(sps.cycleLen*2*len(sps.output)) / 1e6
			sps.heartbeats <- Heartbeat{Running: true, Time: dt, DataMB: mb}
		}
		sps.lastread = nextread // ensure average cycle time is correct, using now would allow error to build up
	}

	// Backtrack to find the time associated with the first sample.
	firstTime := now.Add(-sps.timeperbuf) // use now for accurate sample time
	for _, ch := range sps.output {
		datacopy := make([]RawType, sps.cycleLen)
		copy(datacopy, sps.onecycle)
		for i := 0; i < sps.cycleLen; i++ {
			datacopy[i] += RawType(rand.Intn(21) - 10)
		}
		seg := DataSegment{
			rawData:         datacopy,
			framesPerSample: 1,
			firstFramenum:   sps.nextFrameNum,
			firstTime:       firstTime,
		}
		ch <- seg
	}
	sps.nextFrameNum += FrameIndex(sps.cycleLen)
	return nil
}
