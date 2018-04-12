package main

import (
	"fmt"
	"sync"
	"time"
)

// RawType holds raw signal data.
type RawType uint16

// FrameIndex is used for counting raw data frames.
type FrameIndex int64

// DataSource is the interface for hardware or simulated data sources that
// produce data.
type DataSource interface {
	Sample() error
	PrepareRun() error
	Run() error
	Stop() error
	Running() bool
	BlockingRead() error
	Outputs() []chan DataSegment
	Nchan() int
}

// AnySource implements features common to any object that implements
// DataSource, including the output channels and the abort channel.
type AnySource struct {
	nchan    int // how many channels to provide
	lastread time.Time
	output   []chan DataSegment
	abort    chan struct{} // This can signal all goroutines to stop
	broker   *TriggerBroker
	runMutex sync.Mutex
}

// Start will start the given DataSource, including sampling its data for # channels.
func Start(ds DataSource) error {
	if err := ds.Sample(); err != nil {
		return err
	}
	if err := ds.PrepareRun(); err != nil {
		return err
	}
	if err := ds.Run(); err != nil {
		return err
	}
	return nil
}

func (ds *AnySource) Nchan() int {
	return ds.nchan
}

// Running tells whether the source is actively running
func (ds *AnySource) Running() bool {
	if ds.abort == nil {
		return false
	}
	select {
	case <-ds.abort:
		return false
	default:
		return true
	}
}

// PrepareRun configures an AnySource by initializing all data that cannot
// be prepared until we know the number of channels. It's an error for
// ds.nchan to be less than 1.
func (ds *AnySource) PrepareRun() error {
	if ds.nchan <= 0 {
		return fmt.Errorf("PrepareRun could not run with %d channel (expect > 0)", ds.nchan)
	}
	ds.abort = make(chan struct{})

	// Start a TriggerBroker to handle secondary triggering
	ds.broker = NewTriggerBroker(ds.nchan)
	go ds.broker.Run(ds.abort)

	// Start a data publishing goroutine
	const publishChannelDepth = 500
	dataToPub := make(chan []*DataRecord, publishChannelDepth)
	go PublishRecords(dataToPub, ds.abort, PortTrigs)

	// Launch goroutines to drain the data produced by this source
	allOutputs := ds.Outputs()
	for chnum, ch := range allOutputs {
		dsp := NewDataStreamProcessor(chnum, ds.abort, dataToPub, ds.broker)
		// TODO: don't just set these to arbitrary values
		dsp.Decimate = false
		dsp.DecimateLevel = 3
		dsp.DecimateAvgMode = true
		dsp.LevelTrigger = true
		dsp.LevelLevel = 4000
		dsp.NPresamples = 200
		dsp.NSamples = 1000
		go dsp.ProcessData(ch)
	}
	ds.lastread = time.Now()
	return nil
}

// Stop ends the data supply.
func (ds *AnySource) Stop() error {
	ds.runMutex.Lock()
	defer ds.runMutex.Unlock()

	if ds.Running() {
		close(ds.abort)
	}
	return nil
}

// Outputs returns the slice of channels that carry buffers of data for downstream processing.
func (ds *AnySource) Outputs() []chan DataSegment {
	result := make([]chan DataSegment, ds.nchan)
	copy(result, ds.output)
	return result
}

// DataSegment is a continuous, single-channel raw data buffer, plus info about (e.g.)
// raw-physical units, first sample’s frame number and sample time. Not yet triggered.
type DataSegment struct {
	rawData         []RawType
	framesPerSample int // Normally 1, but can be larger if decimated
	firstFramenum   FrameIndex
	firstTime       time.Time
	framePeriod     time.Duration
	// something about raw-physical conversion???
	// facts about the data source?
}

// NewDataSegment generates a pointer to a new, initialized DataSegment object.
func NewDataSegment(data []RawType, framesPerSample int, firstFrame FrameIndex,
	firstTime time.Time, period time.Duration) *DataSegment {
	seg := DataSegment{rawData: data, framesPerSample: framesPerSample,
		firstFramenum: firstFrame, firstTime: firstTime, framePeriod: period}
	return &seg
}

// TimeOf returns the absolute time of sample # sampleNum within the segment.
func (seg *DataSegment) TimeOf(sampleNum int) time.Time {
	return seg.firstTime.Add(time.Duration(sampleNum*seg.framesPerSample) * seg.framePeriod)
}

// DataStream models a continuous stream of data, though we have only a finite
// amount at any time. For now, it's semantically different from a DataSegment,
// yet they need the same information.
type DataStream struct {
	DataSegment
	samplesSeen int
}

// NewDataStream generates a pointer to a new, initialized DataStream object.
func NewDataStream(data []RawType, framesPerSample int, firstFrame FrameIndex,
	firstTime time.Time, period time.Duration) *DataStream {
	ds := DataStream{DataSegment: DataSegment{rawData: data, framesPerSample: framesPerSample,
		firstFramenum: firstFrame, firstTime: firstTime, framePeriod: period},
		samplesSeen: len(data)}
	return &ds
}

// AppendSegment will append the data in segment to the DataStream.
// It will update the frame/time counters to be consistent with the appended
// segment, not necessarily with the previous values.
func (stream *DataStream) AppendSegment(segment *DataSegment) {
	oldFrameCount := FrameIndex(len(stream.rawData) * segment.framesPerSample)
	stream.framesPerSample = segment.framesPerSample
	stream.framePeriod = segment.framePeriod
	stream.rawData = append(stream.rawData, segment.rawData...)
	stream.firstFramenum = segment.firstFramenum - oldFrameCount
	stream.firstTime = segment.firstTime.Add(-time.Duration(oldFrameCount) * stream.framePeriod)
	stream.samplesSeen += len(segment.rawData)
}

// TrimKeepingN will trim (remove) all but the last N values in the DataStream
func (stream *DataStream) TrimKeepingN(N int) {
	L := len(stream.rawData)
	if N >= L {
		return
	}
	copy(stream.rawData[:N], stream.rawData[L-N:L])
	stream.rawData = stream.rawData[:N]
	stream.firstFramenum += FrameIndex(L - N)
	stream.firstTime = stream.firstTime.Add(time.Duration(L-N) * stream.framePeriod)
}

// DataRecord contains a single triggered pulse record.
type DataRecord struct {
	data      []RawType
	trigFrame FrameIndex
	trigTime  time.Time
	channum   int

	// trigger type?

	// Analyzed quantities
	pretrigMean  float64
	pulseAverage float64
	pulseRMS     float64
	peakValue    float64
}
