package dastard

import (
	"fmt"
	"time"
)

// DataChannel contains all the state needed to decimate, trigger, write, and publish data.
type DataChannel struct {
	Channum     int
	Abort       <-chan struct{}
	NSamples    int
	NPresamples int
	SampleRate  float64
	DecimateState
	TriggerState
}

// DecimateState contains all the state needed to decimate data.
type DecimateState struct {
	DecimateLevel   int
	Decimate        bool
	DecimateAvgMode bool
}

// TriggerState contains all the state that controls trigger logic
type TriggerState struct {
	AutoTrigger bool
	AutoDelay   time.Duration
	autoSamples int
	// Also Level, Edge, and Noise info.
	// Also group source/rx info.
}

// DataRecord contains a single triggered pulse record.
type DataRecord struct {
	data      []RawType
	trigFrame int64
	trigTime  time.Time

	// trigger type?

	// Analyzed quantities
	pretrigMean  float64
	pulseAverage float64
	pulseRMS     float64
	peakValue    float64
}

// ProcessData drains the data channel and processes whatever is found there.
func (dc *DataChannel) ProcessData(dataIn <-chan DataSegment) {
	for {
		select {
		case <-dc.Abort:
			return
		case segment := <-dataIn:
			data := segment.rawData
			fmt.Printf("Chan %d:          found %d values starting with %v\n", dc.Channum, len(data), data[:10])
			data = dc.DecimateData(data)
			fmt.Printf("Chan %d after decimate: %d values starting with %v\n", dc.Channum, len(data), data[:10])
			// records := dc.TriggerData(data)
			// dc.AnalyzeData(records) // add analyzed info in-place
			// dc.WriteData(records)
			// dc.PublishData(records)
		}
	}
}

// DecimateData decimates data in-place.
func (dc *DataChannel) DecimateData(data []RawType) []RawType {
	if !dc.Decimate || dc.DecimateLevel <= 1 {
		return data
	}
	Nin := len(data)
	Nout := (Nin - 1 + dc.DecimateLevel) / dc.DecimateLevel
	if dc.DecimateAvgMode {
		level := dc.DecimateLevel
		for i := 0; i < Nout-1; i++ {
			val := float64(data[i*level])
			for j := 1; j < level; j++ {
				val += float64(data[j+i*level])
			}
			data[i] = RawType(val/float64(level) + 0.5)
		}
		data[Nout-1] = data[(Nout-1)*level]
	} else {
		for i := 0; i < Nout; i++ {
			data[i] = data[i*dc.DecimateLevel]
		}
	}
	return data[:Nout]
}
