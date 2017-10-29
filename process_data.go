package dastard

import (
	"fmt"
	"math"
	"time"
)

// DataChannel contains all the state needed to decimate, trigger, write, and publish data.
type DataChannel struct {
	Channum     int
	Abort       <-chan struct{}
	Publisher   chan<- []*DataRecord
	Broker      *TriggerBroker
	NSamples    int
	NPresamples int
	SampleRate  float64
	stream      DataStream
	DecimateState
	TriggerState
}

// NewDataChannel creates and initializes a new DataChannel.
func NewDataChannel(channum int, abort <-chan struct{}, publisher chan<- []*DataRecord,
	broker *TriggerBroker) *DataChannel {
	dc := DataChannel{Channum: channum, Abort: abort, Publisher: publisher, Broker: broker}
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
	// TODO: Edge and Noise info.
	// TODO: group source/rx info.
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
			records, secondaries := dc.TriggerData(&segment)
			fmt.Printf("Chan %d Found %d triggered records %v\n", dc.Channum, len(records), records[0].data[:10])
			fmt.Printf("Chan %d Found %d secondary records\n", dc.Channum, len(secondaries))
			dc.AnalyzeData(records) // add analysis results to records in-place
			// dc.WriteData(records)
			dc.PublishData(records)
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

// AnalyzeData computes pulse-analysis values in-place for all elements of a
// slice of DataRecord values.
func (dc *DataChannel) AnalyzeData(records []*DataRecord) {
	for _, rec := range records {
		var val float64
		for i := 0; i < dc.NPresamples; i++ {
			val += float64(rec.data[i])
		}
		ptm := val / float64(dc.NPresamples)

		max := rec.data[dc.NPresamples]
		var sum, sum2 float64
		for i := dc.NPresamples; i < dc.NSamples; i++ {
			val = float64(rec.data[i])
			sum += val
			sum2 += val * val
			if rec.data[i] > max {
				max = rec.data[i]
			}
		}
		rec.pretrigMean = ptm
		rec.peakValue = float64(max) - rec.pretrigMean

		N := float64(dc.NSamples - dc.NPresamples)
		rec.pulseAverage = sum/N - ptm
		meanSquare := sum2/N - 2*ptm*(sum/N) + ptm*ptm
		rec.pulseRMS = math.Sqrt(meanSquare)
	}
}

// PublishData sends the slice of DataRecords to be published.
func (dc *DataChannel) PublishData(records []*DataRecord) {
	dc.Publisher <- records
}
