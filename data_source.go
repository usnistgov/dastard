package dastard

import (
	"fmt"
	"io"
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
	StartRun() error
	Stop() error
	Running() bool
	blockingRead() error
	Outputs() []chan DataSegment
	CloseOutputs()
	Nchan() int
	ConfigurePulseLengths(int, int) error
}

// Start will start the given DataSource, including sampling its data for # channels.
// Steps are:
// 1. Sample: per-source method that determines the # of channels and other internal
//    facts that we need to know.
// 2. PrepareRun: an AnySource method to do the actions that any source needs before
//    starting the actual acquisition phase.
// 3. StartRun: per-source method to begin data acquisition.
// 4. Loop over calls to ds.blockingRead(), a per-source method that waits for data.
func Start(ds DataSource) error {
	if ds.Running() {
		return fmt.Errorf("Cannot Start() a source that's already Running().")
	}
	if err := ds.Sample(); err != nil {
		return err
	}
	if err := ds.PrepareRun(); err != nil {
		return err
	}
	if err := ds.StartRun(); err != nil {
		return err
	}
	// Have the DataSource produce data until graceful stop.
	go func() {
		for {
			if err := ds.blockingRead(); err == io.EOF {
				break
			} else if err != nil {
				fmt.Printf("blockingRead returns Error\n")
				break
			}
		}
		ds.CloseOutputs()
	}()
	return nil
}

// AnySource implements features common to any object that implements
// DataSource, including the output channels and the abort channel.
type AnySource struct {
	nchan      int     // how many channels to provide
	sampleRate float64 // samples per second
	lastread   time.Time
	output     []chan DataSegment
	processors []*DataStreamProcessor
	abortSelf  chan struct{} // This can signal the Run() goroutine to stop
	broker     *TriggerBroker
	runMutex   sync.Mutex
	runDone    sync.WaitGroup
}

// Nchan returns the current number of valid channels in the data source.
func (ds *AnySource) Nchan() int {
	return ds.nchan
}

// Running tells whether the source is actively running
func (ds *AnySource) Running() bool {
	if ds.abortSelf == nil {
		return false
	}
	select {
	case <-ds.abortSelf:
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
	ds.abortSelf = make(chan struct{})

	// Start a TriggerBroker to handle secondary triggering
	ds.broker = NewTriggerBroker(ds.nchan)
	go ds.broker.Run()

	// Start a data publishing goroutine
	const publishChannelDepth = 500
	dataToPub := make(chan []*DataRecord, publishChannelDepth)
	go PublishRecords(dataToPub, ds.abortSelf, PortTrigs)

	// Channels onto which we'll put data produced by this source
	ds.output = make([]chan DataSegment, ds.nchan)
	for i := 0; i < ds.nchan; i++ {
		ds.output[i] = make(chan DataSegment)
	}

	// Launch goroutines to drain the data produced by this source
	ds.processors = make([]*DataStreamProcessor, ds.nchan)
	ds.runDone.Add(ds.nchan)
	for chnum, ch := range ds.output {
		dsp := NewDataStreamProcessor(chnum, dataToPub, ds.broker)
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

		// This goroutine will run until the ch==ds.output[chnum] channel is closed
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
		close(ds.abortSelf)
	}
	ds.runDone.Wait()
	return nil
}

// Outputs returns the slice of channels that carry buffers of data for downstream processing.
func (ds *AnySource) Outputs() []chan DataSegment {
	return ds.output
}

// CloseOutputs closes all channels that carry buffers of data for downstream processing.
func (ds *AnySource) CloseOutputs() {
	for _, ch := range ds.output {
		close(ch)
	}
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
