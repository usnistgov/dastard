package main

import (
	"fmt"
	"io"
	"time"
)

// TriangleSource is a DataSource that synthesizes triangle waves.
type TriangleSource struct {
	sampleRate float64 // samples per second
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
	ts.sampleRate = config.SampleRate
	ts.nchan = config.Nchan
	ts.minval = config.Min
	ts.maxval = config.Max

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

// Sample determines key data facts by sampling some initial data.
// It's a no-op for simulated (software) sources
func (ts *TriangleSource) Sample() error {
	return nil
}

// Run begins the data supply.
func (ts *TriangleSource) Run() error {
	ts.runMutex.Lock()
	defer ts.runMutex.Unlock()

	// Have the DataSource produce data until graceful stop.
	go func() {
		for {
			if err := ts.BlockingRead(); err == io.EOF {
				fmt.Printf("BlockingRead returns EOF\n")
				return
			} else if err != nil {
				fmt.Printf("BlockingRead returns Error\n")
				return
			}
		}
	}()

	return nil
}

// BlockingRead blocks and then reads data when "enough" is ready.
func (ts *TriangleSource) BlockingRead() error {
	nextread := ts.lastread.Add(ts.timeperbuf)
	waittime := time.Until(nextread)
	if waittime > 0 {
		select {
		case <-ts.abort:
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

// SimPulseSource simulates simple pulsed sources
type SimPulseSource struct {
	sampleRate float64 // samples per second
	timeperbuf time.Duration
	onecycle   []RawType
	cycleLen   int
	AnySource

	// regular bool // whether pulses are regular or Poisson-distributed
}

// NewSimPulseSource creates a new SimPulseSource with given size, speed.
func NewSimPulseSource() *SimPulseSource {
	ps := new(SimPulseSource)

	// At this point, there are no invariants to enforce
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
	sps.nchan = config.Nchan
	sps.sampleRate = config.SampleRate

	sps.output = make([]chan DataSegment, sps.nchan)
	for i := 0; i < sps.nchan; i++ {
		sps.output[i] = make(chan DataSegment, 1)
	}

	sps.cycleLen = config.Nsamp
	firstIdx := 3
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
	// fmt.Printf("made a simulated pulse source for %d channels.\n", nchan)
	// fmt.Printf("configured with wait time of %v\n", sps.timeperbuf)
	return nil
}

// Sample determines key data facts by sampling some initial data.
// It's a no-op for simulated (software) sources
func (sps *SimPulseSource) Sample() error {
	return nil
}

// Run begins the data supply.
func (sps *SimPulseSource) Run() error {
	sps.runMutex.Lock()
	defer sps.runMutex.Unlock()

	// Have the DataSource produce data until graceful stop.
	go func() {
		for {
			if err := sps.BlockingRead(); err == io.EOF {
				fmt.Printf("BlockingRead returns EOF\n")
				return
			} else if err != nil {
				fmt.Printf("BlockingRead returns Error\n")
				return
			}
		}
	}()

	return nil
}

// BlockingRead blocks and then reads data when "enough" is ready.
func (sps *SimPulseSource) BlockingRead() error {
	nextread := sps.lastread.Add(sps.timeperbuf)
	waittime := time.Until(nextread)
	if waittime > 0 {
		select {
		case <-sps.abort:
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
