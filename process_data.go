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
	stream      DataStream
	DecimateState
	TriggerState
}

// NewDataChannel creates and initializes a new DataChannel.
func NewDataChannel(channum int, abort <-chan struct{}) *DataChannel {
	dc := DataChannel{Channum: channum, Abort: abort}
	return &dc
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

	LevelTrigger bool
	LevelRising  bool
	LevelLevel   RawType
	// Also Level, Edge, and Noise info.
	// Also group source/rx info.
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
			dc.DecimateData(&segment)
			data = segment.rawData
			fmt.Printf("Chan %d after decimate: %d values starting with %v\n", dc.Channum, len(data), data[:10])
			records := dc.TriggerData(&segment)
			fmt.Printf("Chan %d Found %d triggered records %v\n", dc.Channum, len(records), records[0].data[:10])
			dc.AnalyzeData(records) // add analyzed info in-place
			// dc.WriteData(records)
			// dc.PublishData(records)
		}
	}
}

// DecimateData decimates data in-place.
func (dc *DataChannel) DecimateData(segment *DataSegment) {
	if !dc.Decimate || dc.DecimateLevel <= 1 {
		return
	}
	data := segment.rawData
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
		val := float64(data[(Nout-1)*level])
		count := 1.0
		for j := (Nout-1)*level + 1; j < Nin; j++ {
			val += float64(data[j])
			count++
		}
		data[Nout-1] = RawType(val/count + 0.5)
	} else {
		for i := 0; i < Nout; i++ {
			data[i] = data[i*dc.DecimateLevel]
		}
	}
	segment.rawData = data[:Nout]
	segment.framesPerSample *= dc.DecimateLevel
	return
}

func (dc *DataChannel) AnalyzeData(records []DataRecord) {
	for _, rec := range records {
		var val float64
		for i := 0; i < dc.NPresamples; i++ {
			val += float64(rec.data[i])
		}
		rec.pretrigMean = val / float64(dc.NPresamples)

		max := rec.data[dc.NPresamples]
		val = 0
		for i := dc.NPresamples; i < dc.NSamples; i++ {
			val += float64(rec.data[i])
			if rec.data[i] > max {
				max = rec.data[i]
			}
		}
		rec.pulseAverage = val/float64(dc.NSamples-dc.NPresamples) - rec.pretrigMean
		rec.peakValue = float64(max) - rec.pretrigMean
	}
}
