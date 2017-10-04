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
			// dc.AnalyzeData(records) // add analyzed info in-place
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
		data[Nout-1] = data[(Nout-1)*level]
	} else {
		for i := 0; i < Nout; i++ {
			data[i] = data[i*dc.DecimateLevel]
		}
	}
	segment.rawData = data[:Nout]
	return
}

func (dc *DataChannel) triggerAt(segment *DataSegment, i int) DataRecord {
	data := make([]RawType, dc.NSamples)
	copy(data, segment.rawData[i-dc.NPresamples:i+dc.NSamples-dc.NPresamples])
	record := DataRecord{data: data}
	return record
}

// TriggerData analyzes a DataSegment to find and generate triggered records
/* Consider a design where we find all possible triggers of one type, all of the
next, etc, and then merge all lists giving precedence to earlier over later triggers? */
func (dc *DataChannel) TriggerData(segment *DataSegment) (records []DataRecord) {
	nd := len(segment.rawData)
	raw := segment.rawData
	if dc.LevelTrigger {
		for i := dc.NPresamples; i < nd+dc.NPresamples-dc.NSamples; i++ {
			if raw[i] > dc.LevelLevel {
				records = append(records, dc.triggerAt(segment, i))
				i += dc.NSamples
			}
		}
	}
	return
}
