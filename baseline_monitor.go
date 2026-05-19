package dastard

import (
	"fmt"
	"slices"
	"time"

	"github.com/usnistgov/dastard/internal/dastarddb"
)

// BaselineMonitor is a structure to perform inexpensive monitoring of microcalorimeter "baseline" levels.
// It estimates the baseline by approximating the mode of the distribution of raw values, on the theory that
// the baseline is defined as the level to which the readout system returns in between energetic pulse events.
// Raw data are put into the monitor one at a time (`AddOneValue`) or in slices (`AddSliceValues`), and
// zero or more `dbMonitor.BaselineMonitorMessage` objects are produced. These contain a channel #, a timestamp,
// and the monitor value.
//
// The monitor works in 2 stages. At the fast stage, raw data are averaged over `nAverage` consecutive samples.
// The slow stage accumulates these averages until there are `nStore` stored. These are sorted, and the smallest
// range that contains `nPeak` values is used to define a region near the mode, or peak. That cluster is averaged,
// and the message produced uses that cluster average as the monitored value.
// See discussion at https://github.com/usnistgov/dastard/issues/392
type BaselineMonitor struct {
	chanNumber int
	nAverage   int // how many samples to average
	avgCounter int
	avgSum     uint64
	nStore     int // how many averages to queue for analysis
	nPeak      int // how many of the queued averages define the peak; should be 10-20% of nStore
	averages   []float32
}

// NewBaselineMonitor creates and initializes a new BaselineMonitor.
// Its behavior (average time, queue size, cluster size to define a "peak") is fixed at creation time.
func NewBaselineMonitor(chanNumber int, nAverage int, nStore int, nPeak int) *BaselineMonitor {
	if nPeak > nStore {
		fmt.Printf("BaselineMonitor requires nPeak < nStore, got %d, %d\n", nPeak, nStore)
		return nil
	}
	averages := make([]float32, 0, nStore)
	return &BaselineMonitor{
		chanNumber: chanNumber,
		nAverage:   nAverage,
		nStore:     nStore,
		nPeak:      nPeak,
		averages:   averages,
	}
}

// AddOneValue adds a single raw data value to the monitor.
// It returns a pointer to the result message, or (more often) nil if none is ready.
func (bmon *BaselineMonitor) AddOneValue(v RawType) *dastarddb.BaselineMonitorMessage {
	bmon.avgSum += uint64(v)
	bmon.avgCounter += 1
	if bmon.avgCounter >= bmon.nAverage {
		return bmon.performAverage()
	}
	return nil
}

// AddOneValue adds a slice of raw data values to the monitor.
// It returns a slice of pointers to the result messages, or nil if no results are ready.
func (bmon *BaselineMonitor) AddSliceValues(values []RawType) []*dastarddb.BaselineMonitorMessage {
	var msgs []*dastarddb.BaselineMonitorMessage

	idx := 0
	ndata := len(values)

	for idx < ndata {
		// Determine how many we can process in a "fast path"
		// We take the smaller of: the distance to the boundary OR the rest of the slice
		chunkSize := min(bmon.nAverage-bmon.avgCounter, ndata-idx)
		sum := uint64(0)
		for j := range chunkSize {
			sum += uint64(values[idx+j])
		}
		bmon.avgSum += sum
		bmon.avgCounter += chunkSize
		idx += chunkSize

		// Boundary check only happens once per nAverage samples in large slices
		if bmon.avgCounter >= bmon.nAverage {
			if thismsg := bmon.performAverage(); thismsg != nil {
				if msgs == nil {
					msgs = make([]*dastarddb.BaselineMonitorMessage, 0, 16)
				}
				msgs = append(msgs, thismsg)
			}
		}
	}
	return msgs
}

// performAverage averages and resets the short-time averaging of raw values. It appends the result
// to the slower `bmon.averages` queue, and analyzes that queue if full.
// It returns the single message that comes from queue analysis, or nil otherwise.
func (bmon *BaselineMonitor) performAverage() *dastarddb.BaselineMonitorMessage {
	avg := float32(bmon.avgSum) / float32(bmon.avgCounter)
	bmon.avgSum = 0.0
	bmon.avgCounter = 0
	bmon.averages = append(bmon.averages, avg)
	if len(bmon.averages) == bmon.nStore {
		return bmon.analyzeQueue()
	}
	return nil
}

// analyzeQueue finds a weighted average of values near the mode of the distribution, and it
// returns a single message containing the result, suitable for sending to a dastarddb.
// It also resets the queue for re-use.
//
// This analysis of the queue defines the mode (the monitored value) by averaging the most
// closely clustered values for a cluster of size bmon.nPeak.
func (bmon *BaselineMonitor) analyzeQueue() *dastarddb.BaselineMonitorMessage {
	slices.Sort(bmon.averages)
	bestRange := bmon.averages[bmon.nPeak-1] - bmon.averages[0]
	bestIdx := 0
	for i := 0; i < bmon.nStore-bmon.nPeak; i++ {
		thisrange := bmon.averages[bmon.nPeak+i] - bmon.averages[i+1]
		if thisrange < bestRange {
			bestRange = thisrange
			bestIdx = i + 1
		}
	}
	var sum float32
	clusterValues := bmon.averages[bestIdx : bestIdx+bmon.nPeak]
	for _, v := range clusterValues {
		sum += v
	}
	baseline := sum / float32(bmon.nPeak)
	bmon.averages = bmon.averages[:0]
	return &dastarddb.BaselineMonitorMessage{
		ChanNum:   bmon.chanNumber,
		Timestamp: time.Now(),
		Value:     baseline,
	}
}
