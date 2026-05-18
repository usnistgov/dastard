package dastard

import (
	"fmt"
	"slices"
	"time"

	"github.com/usnistgov/dastard/internal/dastarddb"
)

type BaselineMonitor struct {
	chanNumber int
	nAverage   int // how many samples to average
	avgCounter int
	avgSum     uint32
	nStore     int // how many averages to queue for analysis
	nPeak      int // how many of the queued averages define the peak; should be 10-20% of nStore
	averages   []float32
}

// NewBaselineMonitor creates and initializes a new BaselineMonitor.
// Its behavior (average time, queue size) is fixed at creation time.
func NewBaselineMonitor(chanNumber int, nAverage int, nStore int, nPeak int) *BaselineMonitor {
	if nAverage > 65536 {
		fmt.Printf("BaselineMonitor requires nAverage <= 65536, got %d\n", nAverage)
		return nil
	}
	if nPeak > nStore {
		fmt.Printf("BaselineMonitor requires nPeak < nStore, got %d, %d\n", nPeak, nStore)
		return nil
	}
	averages := make([]float32, 0, nStore)
	bmon := BaselineMonitor{
		chanNumber: chanNumber,
		nAverage:   nAverage,
		nStore:     nStore,
		nPeak:      nPeak,
		averages:   averages,
	}
	return &bmon
}

func (bmon *BaselineMonitor) AddOneValue(v RawType) (msgs []*dastarddb.BaselineMonitorMessage) {
	bmon.avgSum += uint32(v)
	bmon.avgCounter += 1
	if bmon.avgCounter >= bmon.nAverage {
		msg := bmon.performAverage()
		if msg != nil {
			return []*dastarddb.BaselineMonitorMessage{msg}
		}
	}
	return nil
}

func (bmon *BaselineMonitor) AddSliceValues(values []RawType) (msgs []*dastarddb.BaselineMonitorMessage) {
	for _, v := range(values){
		bmon.avgSum += uint32(v)
		bmon.avgCounter += 1
		if bmon.avgCounter >= bmon.nAverage {
			thismsg := bmon.performAverage()
			if thismsg != nil {
				if msgs == nil {
					msgs = make([]*dastarddb.BaselineMonitorMessage, 0, 16)
				}
				msgs = append(msgs, thismsg)
			}
		}
	}
	return
}

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

func (bmon *BaselineMonitor) analyzeQueue() *dastarddb.BaselineMonitorMessage {
	slices.Sort(bmon.averages)
	bestRange := bmon.averages[bmon.nPeak] - bmon.averages[0]
	bestIdx := 0
	for i := 0; i < bmon.nStore-bmon.nPeak; i++ {
		thisrange := bmon.averages[bmon.nPeak+i] - bmon.averages[i]
		if thisrange < bestRange {
			bestRange = thisrange
			bestIdx = i
		}
	}
	var sum float32
	for i := bestIdx; i < bestIdx+bmon.nPeak; i++ {
		sum += bmon.averages[i]
	}
	baseline := sum / float32(bmon.nPeak)
	bmon.averages = bmon.averages[:0]
	return &dastarddb.BaselineMonitorMessage{
		ChanNum:   bmon.chanNumber,
		Timestamp: time.Now(),
		Value:     baseline,
	}
}
