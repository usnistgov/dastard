package dastard

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
	GenerateOutputs() []chan DataSegment
	Nchan() int
	ConfigurePulseLengths(int, int) error
}

// AnySource implements features common to any object that implements
// DataSource, including the output channels and the abort channel.
type AnySource struct {
	nchan      int     // how many channels to provide
	sampleRate float64 // samples per second
	lastread   time.Time
	output     []chan DataSegment
	processors []*DataStreamProcessor
	abort      chan struct{} // This can signal all goroutines to stop
	broker     *TriggerBroker
	runMutex   sync.Mutex
	runDone    sync.WaitGroup
}

// Start will start the given DataSource, including sampling its data for # channels.
// Steps are:
// 1. Sample: per-source method that determines the # of channels and other internal
//    facts that we need to know.
// 2. PrepareRun: an AnySource method to do the actions that any source needs before
//    starting the actual acquisition phase.
// 3. Run: a per-source method that starts the data acquisition.
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

// Nchan returns the current number of valid channels in the data source.
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

// PrepareRun configures an AnySource by initializing all data structures that
// cannot be prepared until we know the number of channels. It's an error for
// ds.nchan to be less than 1.
func (ds *AnySource) PrepareRun() error {
	ds.runMutex.Lock()
	defer ds.runMutex.Unlock()
	if ds.nchan <= 0 {
		return fmt.Errorf("PrepareRun could not run with %d channels (expect > 0)", ds.nchan)
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
	ds.processors = make([]*DataStreamProcessor, ds.nchan)
	ds.runDone.Add(ds.nchan)
	allOutputs := ds.GenerateOutputs()
	for chnum, ch := range allOutputs {
		dsp := NewDataStreamProcessor(chnum, ds.abort, dataToPub, ds.broker)
		dsp.SampleRate = ds.sampleRate
		ds.processors[chnum] = dsp

		// TODO: don't just set these to arbitrary values
		dsp.Decimate = false
		dsp.DecimateLevel = 3
		dsp.DecimateAvgMode = true
		dsp.LevelTrigger = false
		dsp.LevelLevel = 4000
		dsp.EdgeTrigger = true
		dsp.EdgeLevel = 100
		dsp.EdgeRising = true
		dsp.NPresamples = 200
		dsp.NSamples = 1000

		// This goroutine will run until the ds.abort channel is closed
		go func(ch <-chan DataSegment) {
			defer ds.runDone.Done()
			dsp.ProcessData(ch)
		}(ch)
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
	ds.runDone.Wait()
	return nil
}

// GenerateOutputs generates the slice of channels that carry buffers of data for downstream processing.
func (ds *AnySource) GenerateOutputs() []chan DataSegment {
	result := make([]chan DataSegment, ds.nchan)
	copy(result, ds.output)
	return result
}

func (ds *AnySource) ConfigurePulseLengths(nsamp, npre int) error {
	if npre < 0 || nsamp < 1 || nsamp < npre+1 {
		return fmt.Errorf("ConfigurePulseLengths arguments are invalid")
	}
	for _, dsp := range ds.processors {
		go dsp.ConfigurePulseLengths(nsamp, npre)
	}
	return nil
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
	seg := NewDataSegment(data, framesPerSample, firstFrame, firstTime, period)
	ds := DataStream{DataSegment: *seg, samplesSeen: len(data)}
	return &ds
}

// AppendSegment will append the data in segment to the DataStream.
// It will update the frame/time counters to be consistent with the appended
// segment, not necessarily with the previous values.
func (stream *DataStream) AppendSegment(segment *DataSegment) {
	framesNowInStream := FrameIndex(len(stream.rawData) * segment.framesPerSample)
	stream.framesPerSample = segment.framesPerSample
	stream.framePeriod = segment.framePeriod
	stream.firstFramenum = segment.firstFramenum - framesNowInStream
	stream.firstTime = segment.firstTime.Add(-time.Duration(framesNowInStream) * stream.framePeriod)
	stream.rawData = append(stream.rawData, segment.rawData...)
	stream.samplesSeen += len(segment.rawData)
}

// TrimKeepingN will trim (discard) all but the last N values in the DataStream.
// Returns the number of values in the stream after trimming (should be <= N).
func (stream *DataStream) TrimKeepingN(N int) int {
	L := len(stream.rawData)
	if N >= L {
		return L
	}
	copy(stream.rawData[:N], stream.rawData[L-N:L])
	stream.rawData = stream.rawData[:N]
	stream.firstFramenum += FrameIndex(L - N)
	stream.firstTime = stream.firstTime.Add(time.Duration(L-N) * stream.framePeriod)
	return N
}

// DataRecord contains a single triggered pulse record.
type DataRecord struct {
	data       []RawType
	trigFrame  FrameIndex
	trigTime   time.Time
	channum    int
	presamples int
	sampPeriod float32

	// trigger type?

	// Analyzed quantities
	pretrigMean  float64
	pulseAverage float64
	pulseRMS     float64
	peakValue    float64
}
