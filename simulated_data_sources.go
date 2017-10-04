package dastard

import (
	"fmt"
	"io"
	"time"
)

// RawType holds raw signal data.
type RawType uint16

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
	fmt.Printf("made a source for %d channels.\n", nchan)
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
	fmt.Printf("configured with wait time of %v and cR: %f\n", ts.timeperbuf, cycleTime)
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
		fmt.Printf("Waiting %v to read\n", waittime)
		select {
		case <-abort:
			fmt.Println("aborted the read")
			return io.EOF
		case <-time.After(waittime):
			ts.lastread = time.Now()
		}
	}

	for _, ch := range ts.output {
		datacopy := make([]RawType, ts.cycleLen)
		copy(datacopy, ts.onecycle)
		seg := DataSegment{rawData: datacopy, prevValue: ts.minval + 1}
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
