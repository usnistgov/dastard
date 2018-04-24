package dastard

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// DataStreamProcessor contains all the state needed to decimate, trigger, write, and publish data.
type DataStreamProcessor struct {
	Channum     int
	Abort       <-chan struct{}
	Publisher   chan<- []*DataRecord
	Broker      *TriggerBroker
	NSamples    int
	NPresamples int
	SampleRate  float64
	LastTrigger FrameIndex
	stream      DataStream
	DecimateState
	TriggerState
	changeMutex sync.Mutex // Don't change key data without locking this.
}

// NewDataStreamProcessor creates and initializes a new DataStreamProcessor.
func NewDataStreamProcessor(channum int, abort <-chan struct{}, publisher chan<- []*DataRecord,
	broker *TriggerBroker) *DataStreamProcessor {
	data := make([]RawType, 0, 1024)
	framesPerSample := 1
	firstFrame := FrameIndex(0)
	firstTime := time.Now()
	period := time.Duration(1 * time.Millisecond) // TODO: figure out what this ought to be, or make an argument
	stream := NewDataStream(data, framesPerSample, firstFrame, firstTime, period)
	nsamp := 1024 // TODO: figure out what this ought to be, or make an argument
	npre := 256   // TODO: figure out what this ought to be, or make an argument
	dsp := DataStreamProcessor{Channum: channum, Abort: abort, Publisher: publisher, Broker: broker,
		stream: *stream, NSamples: nsamp, NPresamples: npre,
	}
	dsp.LastTrigger = math.MinInt64 / 4 // far in the past, but not so far we can't subtract from it.
	return &dsp
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

	LevelTrigger bool
	LevelRising  bool
	LevelLevel   RawType

	EdgeTrigger bool
	EdgeRising  bool
	EdgeFalling bool
	EdgeLevel   int32

	// TODO:  Noise info.
	// TODO: group source/rx info.
}

// ConfigurePulseLengths sets this stream's pulse length and # of presamples.
func (dsp *DataStreamProcessor) ConfigurePulseLengths(nsamp, npre int) {
	if nsamp <= npre+1 || npre < 0 {
		return
	}
	dsp.changeMutex.Lock()
	defer dsp.changeMutex.Unlock()

	dsp.NSamples = nsamp
	dsp.NPresamples = npre
}

// ProcessData drains the data channel and processes whatever is found there.
func (dsp *DataStreamProcessor) ProcessData(dataIn <-chan DataSegment) {
	for {
		select {
		case <-dsp.Abort:
			return
		case segment := <-dataIn:
			dsp.processSegment(&segment)
		}
	}
}

func (dsp *DataStreamProcessor) processSegment(segment *DataSegment) {
	dsp.changeMutex.Lock()
	defer dsp.changeMutex.Unlock()

	dsp.DecimateData(segment)
	dsp.stream.AppendSegment(segment)
	records, secondaries := dsp.TriggerData()
	if len(records)+len(secondaries) > 0 {
		fmt.Printf("Chan %d Found %d triggered records, %d secondary records.\n",
			dsp.Channum, len(records), len(secondaries))
	}
	dsp.AnalyzeData(records) // add analysis results to records in-place
	// TODO: dsp.WriteData(records)
	dsp.PublishData(records)
}

// DecimateData decimates data in-place.
func (dsp *DataStreamProcessor) DecimateData(segment *DataSegment) {
	if !dsp.Decimate || dsp.DecimateLevel <= 1 {
		return
	}
	data := segment.rawData
	Nin := len(data)
	Nout := (Nin - 1 + dsp.DecimateLevel) / dsp.DecimateLevel
	if dsp.DecimateAvgMode {
		level := dsp.DecimateLevel
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
			data[i] = data[i*dsp.DecimateLevel]
		}
	}
	segment.rawData = data[:Nout]
	segment.framesPerSample *= dsp.DecimateLevel
	return
}

// AnalyzeData computes pulse-analysis values in-place for all elements of a
// slice of DataRecord values.
func (dsp *DataStreamProcessor) AnalyzeData(records []*DataRecord) {
	for _, rec := range records {
		var val float64
		for i := 0; i < dsp.NPresamples; i++ {
			val += float64(rec.data[i])
		}
		ptm := val / float64(dsp.NPresamples)

		max := rec.data[dsp.NPresamples]
		var sum, sum2 float64
		for i := dsp.NPresamples; i < dsp.NSamples; i++ {
			val = float64(rec.data[i])
			sum += val
			sum2 += val * val
			if rec.data[i] > max {
				max = rec.data[i]
			}
		}
		rec.pretrigMean = ptm
		rec.peakValue = float64(max) - rec.pretrigMean

		N := float64(dsp.NSamples - dsp.NPresamples)
		rec.pulseAverage = sum/N - ptm
		meanSquare := sum2/N - 2*ptm*(sum/N) + ptm*ptm
		rec.pulseRMS = math.Sqrt(meanSquare)
	}
}

// PublishData sends the slice of DataRecords to be published.
func (dsp *DataStreamProcessor) PublishData(records []*DataRecord) {
	dsp.Publisher <- records
}
