package dastard

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"gonum.org/v1/gonum/mat"
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
	ComputeFullTriggerState() []FullTriggerState
	ComputeWritingState() WritingState
	ChannelNames() []string
	ConfigurePulseLengths(int, int) error
	ConfigureProjectorsBases(int, mat.Dense, mat.Dense) error
	ChangeTriggerState(*FullTriggerState) error
	ConfigureMixFraction(int, float64) error
	WriteControl(*WriteControlConfig) error
}

// ConfigureMixFraction provides a default implementation for all non-lancero sources that
// don't need the mix
func (ds *AnySource) ConfigureMixFraction(processorIndex int, mixFraction float64) error {
	return fmt.Errorf("this source does not support Mix") // how to best provide name of source here?
}

// Start will start the given DataSource, including sampling its data for # channels.
// Steps are:
// 1. Sample: a per-source method that determines the # of channels and other internal
//    facts that we need to know.
// 2. PrepareRun: an AnySource method to do the actions that any source needs before
//    starting the actual acquisition phase.
// 3. StartRun: a per-source method to begin data acquisition, if relevant.
// 4. Loop over calls to ds.blockingRead(), a per-source method that waits for data.
//    When done with the loop, close all channels to DataStreamProcessor objects.
func Start(ds DataSource) error {
	if ds.Running() {
		return fmt.Errorf("cannot Start() a source that's already Running()")
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
				log.Printf("blockingRead returns Error: %s\n", err.Error())
				// break
			}
		}
		ds.CloseOutputs()
	}()
	return nil
}

// AnySource implements features common to any object that implements
// DataSource, including the output channels and the abort channel.
type AnySource struct {
	nchan        int      // how many channels to provide
	chanNames    []string // one name per channel
	sampleRate   float64  // samples per second
	lastread     time.Time
	nextFrameNum FrameIndex // frame number for the next frame we will receive
	output       []chan DataSegment
	processors   []*DataStreamProcessor
	abortSelf    chan struct{} // This can signal the Run() goroutine to stop
	broker       *TriggerBroker
	noProcess    bool // Set true only for testing.
	heartbeats   chan Heartbeat
	writingState WritingState
	runMutex     sync.Mutex
	runDone      sync.WaitGroup
}

// StartRun tells the hardware to switch into data streaming mode.
// It's a no-op for simulated (software) sources
func (ds *AnySource) StartRun() error {
	return nil
}

// makeDirectory creates directory of the form basepath/20060102/000 where
// the 3-digit subdirectory counts separate file-writing occasions.
// It also returns the formatting code for use in an Sprintf call
// basepath/20060102/000/20060102_run000_%s.ljh and an error, if any.
func makeDirectory(basepath string) (string, error) {
	today := time.Now().Format("20060102")
	todayDir := fmt.Sprintf("%s/%s", basepath, today)
	if err := os.MkdirAll(todayDir, 0755); err != nil {
		return "", err
	}
	for i := 0; i < 10000; i++ {
		thisDir := fmt.Sprintf("%s/%4.4d", todayDir, i)
		_, err := os.Stat(thisDir)
		if os.IsNotExist(err) {
			if err2 := os.MkdirAll(thisDir, 0755); err2 != nil {
				return "", err
			}
			return fmt.Sprintf("%s/%s_run%4.4d_%%s.ljh", thisDir, today, i), nil
		}
	}
	return "", fmt.Errorf("out of 4-digit ID numbers for today in %s", todayDir)
}

// WriteControl changes the data writing start/stop/pause/unpause state
func (ds *AnySource) WriteControl(config *WriteControlConfig) error {
	if strings.HasPrefix(config.Request, "Pause") {
		for _, dsp := range ds.processors {
			dsp.DataPublisher.SetPause(true)
		}
		ds.writingState.Paused = true
	} else if strings.HasPrefix(config.Request, "Unpause") {
		for _, dsp := range ds.processors {
			dsp.DataPublisher.SetPause(false)
		}
		ds.writingState.Paused = false
	} else if strings.HasPrefix(config.Request, "Stop") {
		for _, dsp := range ds.processors {
			dsp.DataPublisher.RemoveLJH22()
			dsp.DataPublisher.RemoveLJH3()
		}
		ds.writingState.Active = false
		ds.writingState.Paused = false
		ds.writingState.BasePath = config.Path
		ds.writingState.Filename = ""
	} else if strings.HasPrefix(config.Request, "Start") {
		if config.FileType != "LJH2.2" {
			return fmt.Errorf("WriteControl FileType=%q, needs to be %q",
				config.FileType, "LJH2.2")
		}
		filenamePattern, err := makeDirectory(config.Path)
		if err != nil {
			return fmt.Errorf("Could not make directory: %s", err.Error())
		}
		for i, dsp := range ds.processors {
			if dsp.DataPublisher.HasLJH22() {
				return fmt.Errorf("WriteControl Request:Start is not yet implemented when writing already running")
			}
			timebase := 1.0 / dsp.SampleRate
			var nrows, ncols int // default to 0 x 0 array
			// TODO: update nrows, ncols for a Lancero source
			filename := fmt.Sprintf(filenamePattern, dsp.Name)
			var timestampOffset float64 // TODO: figure this out
			dsp.DataPublisher.SetLJH22(i, dsp.NPresamples, dsp.NSamples, timebase, timestampOffset, nrows, ncols, filename)
		}
		ds.writingState.Active = true
		ds.writingState.Paused = false
		ds.writingState.BasePath = config.Path
		ds.writingState.Filename = fmt.Sprintf(filenamePattern, "chan*")
	} else {
		return fmt.Errorf("WriteControl config.Request=%q, need (Start,Stop,Pause,Unpause)",
			config.Request)
	}

	return nil
}

// WritingState monitors the state of file writing.
type WritingState struct {
	Active   bool
	Paused   bool
	BasePath string
	Filename string
}

// ComputeWritingState doesn't need to compute, but just returns the writingState
func (ds *AnySource) ComputeWritingState() WritingState {
	return ds.writingState
}

// ConfigureProjectorsBases calls SetProjectorsBasis on ds.processors[processorsInd]
func (ds *AnySource) ConfigureProjectorsBases(processorInd int, projectors mat.Dense, basis mat.Dense) error {
	if processorInd >= len(ds.processors) || processorInd < 0 {
		return fmt.Errorf("processorInd out of range, processorInd=%v, len(ds.processors)=%v", processorInd, len(ds.processors))
	}
	dsp := ds.processors[processorInd]
	return dsp.SetProjectorsBasis(projectors, basis)
}

// Nchan returns the current number of valid channels in the data source.
func (ds *AnySource) Nchan() int {
	return ds.nchan
}

// Running tells whether the source is actively running.
// If there's no ds.abortSelf yet, or it's closed, then source is NOT running.
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

// setDefaultChannelNames defensively sets channel names of the appropriate length.
// They should have been set in DataSource.Sample()
func (ds *AnySource) setDefaultChannelNames() {
	// If the number of channel names is correct, assume it was set in Sample, as expected.
	if len(ds.chanNames) == ds.nchan {
		return
	}
	ds.chanNames = make([]string, ds.nchan)
	for i := 0; i < ds.nchan; i++ {
		ds.chanNames[i] = fmt.Sprintf("chan%d", i)
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
	ds.setDefaultChannelNames() // should be overwritten in ds.Sample()
	ds.abortSelf = make(chan struct{})

	// Start a TriggerBroker to handle secondary triggering
	ds.broker = NewTriggerBroker(ds.nchan)
	go ds.broker.Run()

	// Channels onto which we'll put data produced by this source
	ds.output = make([]chan DataSegment, ds.nchan)
	for i := 0; i < ds.nchan; i++ {
		ds.output[i] = make(chan DataSegment)
	}

	// Launch goroutines to drain the data produced by this source
	ds.processors = make([]*DataStreamProcessor, ds.nchan)
	ds.runDone.Add(ds.nchan)
	for channelNum, dataSegmentChan := range ds.output {
		dsp := NewDataStreamProcessor(channelNum, ds.broker)
		dsp.Name = ds.chanNames[channelNum]
		dsp.SampleRate = ds.sampleRate
		ds.processors[channelNum] = dsp

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
		dsp.AutoTrigger = true
		dsp.AutoDelay = 250 * time.Millisecond

		// TODO: don't automatically turn on all record publishing.
		dsp.SetPubRecords()
		dsp.SetPubSummaries()

		// This goroutine will run until the ds.abortSelf channel or the ch==ds.output[chnum]
		// channel is closed, depending on ds.noProcess (which is false except for testing)
		go func(ch <-chan DataSegment) {
			defer ds.runDone.Done()
			if ds.noProcess {
				<-ds.abortSelf
			} else {
				dsp.ProcessData(ch)
			}
		}(dataSegmentChan)
	}
	ds.lastread = time.Now()
	return nil
}

// Stop ends the data supply.
func (ds *AnySource) Stop() error {
	if ds.Running() {
		close(ds.abortSelf)
	}
	ds.runDone.Wait()
	return nil
}

// Outputs returns the slice of channels that carry buffers of data for downstream processing.
func (ds *AnySource) Outputs() []chan DataSegment {
	// Don't run this if PrepareRun or other sensitive sections are running
	ds.runMutex.Lock()
	defer ds.runMutex.Unlock()
	return ds.output
}

// CloseOutputs closes all channels that carry buffers of data for downstream processing.
func (ds *AnySource) CloseOutputs() {
	ds.runMutex.Lock()
	defer ds.runMutex.Unlock()

	for _, ch := range ds.output {
		close(ch)
	}
	// ds.output = make([]chan DataSegment, 0)
	ds.output = nil
}

// FullTriggerState used to collect channels that share the same TriggerState
type FullTriggerState struct {
	ChanNumbers []int
	TriggerState
}

// ComputeFullTriggerState uses a map to collect channels with identical TriggerStates, so they
// can be sent all together as one unit.
func (ds *AnySource) ComputeFullTriggerState() []FullTriggerState {

	result := make(map[TriggerState][]int)
	for _, dsp := range ds.processors {
		chans, ok := result[dsp.TriggerState]
		if ok {
			result[dsp.TriggerState] = append(chans, dsp.Channum)
		} else {
			result[dsp.TriggerState] = []int{dsp.Channum}
		}
	}

	// Now "unroll" that map into a vector of FullTriggerState objects
	fts := []FullTriggerState{}
	for k, v := range result {
		fts = append(fts, FullTriggerState{ChanNumbers: v, TriggerState: k})
	}
	return fts
}

// ChangeTriggerState changes the trigger state for 1 or more channels.
func (ds *AnySource) ChangeTriggerState(state *FullTriggerState) error {
	for _, chnum := range state.ChanNumbers {
		ds.processors[chnum].TriggerState = state.TriggerState
	}
	return nil
}

// ChannelNames returns a slice of the channel names
func (ds *AnySource) ChannelNames() []string {
	return ds.chanNames
}

// ConfigurePulseLengths set the pulse record length and pre-samples.
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

	// Real time Analysis quantities
	modelCoefs     []float64
	residualStdDev float64
}
