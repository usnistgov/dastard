package dastard

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
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
	CloseOutputs()
	Nchan() int
	Signed() []bool
	VoltsPerArb() []float32
	ComputeFullTriggerState() []FullTriggerState
	ComputeWritingState() WritingState
	ChannelNames() []string
	ConfigurePulseLengths(int, int) error
	ConfigureProjectorsBases(int, mat.Dense, mat.Dense) error
	ChangeTriggerState(*FullTriggerState) error
	ConfigureMixFraction(int, float64) error
	WriteControl(*WriteControlConfig) error
	SetCoupling(CouplingStatus) error
}

// ConfigureMixFraction provides a default implementation for all non-lancero sources that
// don't need the mix
func (ds *AnySource) ConfigureMixFraction(channelIndex int, mixFraction float64) error {
	return fmt.Errorf("source type %s does not support Mix", ds.name)
}

// Start will start the given DataSource, including sampling its data for # channels.
// Steps are: 1) Sample: a per-source method that determines the # of channels and other
// internal facts that we need to know.  2) PrepareRun: an AnySource method to do the
// actions that any source needs before starting the actual acquisition phase.
// 3) StartRun: a per-source method to begin data acquisition, if relevant.
// 4) Loop over calls to ds.blockingRead(), a per-source method that waits for data.
// When done with the loop, close all channels to DataStreamProcessor objects.
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
				// EOF error should occur after abortSelf has been closed
				// ds.CloseOutputs() // why is this here, ds.Stop also calls CloseOutputs
				return
			} else if err != nil {
				// other errors indicate a problem with source, need to close down
				fmt.Printf("blockingRead returns Error, stopping source: %s\n", err.Error())
				ds.Stop() // will cause next call to ds.blockingRead to return io.EOF
			}
		}

	}()
	return nil
}

// RowColCode holds an 8-byte summary of the row-column geometry
type RowColCode uint64

func (c RowColCode) row() int {
	return int((uint64(c) >> 0) & 0xffff)
}
func (c RowColCode) col() int {
	return int((uint64(c) >> 16) & 0xffff)
}
func (c RowColCode) rows() int {
	return int((uint64(c) >> 32) & 0xffff)
}
func (c RowColCode) cols() int {
	return int((uint64(c) >> 48) & 0xffff)
}
func rcCode(row, col, rows, cols int) RowColCode {
	code := cols & 0xffff
	code = code<<16 | (rows & 0xffff)
	code = code<<16 | (col & 0xffff)
	code = code<<16 | (row & 0xffff)
	return RowColCode(code)
}

// AnySource implements features common to any object that implements
// DataSource, including the output channels and the abort channel.
type AnySource struct {
	nchan        int           // how many channels to provide
	name         string        // what kind of source is this?
	chanNames    []string      // one name per channel
	chanNumbers  []int         // names have format "prefixNumber", this is the number
	rowColCodes  []RowColCode  // one RowColCode per channel
	signed       []bool        // is the raw data signed, one per channel
	voltsPerArb  []float32     // the physical units per arb, one per channel
	sampleRate   float64       // samples per second
	samplePeriod time.Duration // time per sample
	lastread     time.Time
	nextFrameNum FrameIndex // frame number for the next frame we will receive
	output       []chan DataSegment
	processors   []*DataStreamProcessor
	abortSelf    chan struct{} // This can signal the Run() goroutine to stop
	broker       *TriggerBroker
	publishSync  *PublishSync
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
// basepath/20060102/000/20060102_run000_%s.%s and an error, if any.
func makeDirectory(basepath string) (string, error) {
	if len(basepath) == 0 {
		return "", fmt.Errorf("BasePath is the empty string")
	}
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
			return fmt.Sprintf("%s/%s_run%4.4d_%%s.%%s", thisDir, today, i), nil
		}
	}
	return "", fmt.Errorf("out of 4-digit ID numbers for today in %s", todayDir)
}

// WriteControl changes the data writing start/stop/pause/unpause state
// For WriteLJH22 == true and/or WriteLJH3 == true all channels will have writing enabled
// For WriteOFF == true, only chanels with projectors set will have writing enabled
// the function first checks for errors, then launched a goroutine with a lock
// to actually change state, the last thing the goroutine does is close the returned channel
// (closed channels always return the zero value immediately, so it can be used for waiting until the writing state is changed)
func (ds *AnySource) WriteControl(config *WriteControlConfig) error {
	request := strings.ToUpper(config.Request)
	var filenamePattern, path string
	// ds.runMutex.Lock()
	// defer ds.runMutex.Unlock()
	// this seems like a good idea, but doesn't actually prevent race conditions, and leads to deadlocks

	// first check for possible errors, then take the lock and do the work
	if strings.HasPrefix(request, "START") {
		if !(config.WriteLJH22 || config.WriteOFF || config.WriteLJH3) {
			return fmt.Errorf("WriteLJH22 and WriteOFF and WriteLJH3 all false")
		}

		for _, dsp := range ds.processors {
			if dsp.DataPublisher.HasLJH22() || dsp.DataPublisher.HasOFF() || dsp.DataPublisher.HasLJH3() {
				return fmt.Errorf(
					"Writing already in progress, stop writing before starting again. Currently: LJH22 %v, OFF %v, LJH3 %v",
					dsp.DataPublisher.HasLJH22(), dsp.DataPublisher.HasOFF(), dsp.DataPublisher.HasLJH3())
			}
		}

		path = ds.writingState.BasePath
		if len(config.Path) > 0 {
			path = config.Path
		}
		var err error
		filenamePattern, err = makeDirectory(path)
		if err != nil {
			return fmt.Errorf("Could not make directory: %s", err.Error())
		}
		if config.WriteOFF {
			// throw an error if no channels have projectors set
			// only channels with projectors set will have OFF files enabled
			anyProjectorsSet := false
			for _, dsp := range ds.processors {
				if !(dsp.projectors.IsZero() || dsp.basis.IsZero()) {
					anyProjectorsSet = true
					break
				}
			}
			if !anyProjectorsSet {
				return fmt.Errorf("no projectors are loaded, OFF files require projectors")
			}
		}
	}
	if !(strings.HasPrefix(request, "START") || strings.HasPrefix(request, "STOP") ||
		strings.HasPrefix(request, "PAUSE") || strings.HasPrefix(request, "UNPAUSE")) {
		return fmt.Errorf("WriteControl config.Request=%q, need one of (START,STOP,PAUSE,UNPAUSE). Not case sensitive",
			config.Request)
	}

	// Hold the lock before doing actual changes

	if strings.HasPrefix(request, "PAUSE") {
		for _, dsp := range ds.processors {
			dsp.DataPublisher.SetPause(true)
		}
		ds.writingState.Paused = true

	} else if strings.HasPrefix(request, "UNPAUSE") {
		for _, dsp := range ds.processors {
			dsp.DataPublisher.SetPause(false)
		}
		ds.writingState.Paused = false

	} else if strings.HasPrefix(request, "STOP") {
		for _, dsp := range ds.processors {
			dsp.DataPublisher.RemoveLJH22()
			dsp.DataPublisher.RemoveOFF()
			dsp.DataPublisher.RemoveLJH3()
		}
		ds.writingState.Active = false
		ds.writingState.Paused = false
		ds.writingState.Filename = ""

	} else if strings.HasPrefix(request, "START") {
		channelsWithOff := 0
		for i, dsp := range ds.processors {
			timebase := 1.0 / dsp.SampleRate
			rccode := ds.rowColCodes[i]
			nrows := rccode.rows()
			ncols := rccode.cols()
			rowNum := rccode.row()
			colNum := rccode.col()
			fps := 1
			if dsp.Decimate {
				fps = dsp.DecimateLevel
			}
			if config.WriteLJH22 {
				filename := fmt.Sprintf(filenamePattern, dsp.Name, "ljh")
				dsp.DataPublisher.SetLJH22(i, dsp.NPresamples, dsp.NSamples, fps,
					timebase, Build.RunStart, nrows, ncols, ds.nchan, rowNum, colNum, filename,
					ds.name, ds.chanNames[i], ds.chanNumbers[i])
			}
			if config.WriteOFF && !dsp.projectors.IsZero() {
				filename := fmt.Sprintf(filenamePattern, dsp.Name, "off")
				dsp.DataPublisher.SetOFF(i, dsp.NPresamples, dsp.NSamples, fps,
					timebase, Build.RunStart, nrows, ncols, ds.nchan, rowNum, colNum, filename,
					ds.name, ds.chanNames[i], ds.chanNumbers[i], &dsp.projectors, &dsp.basis,
					"model description not implemented. but it should be mean, average pulse, derivative of average pulse")
				channelsWithOff++
			}
			if config.WriteLJH3 {
				filename := fmt.Sprintf(filenamePattern, dsp.Name, "ljh3")
				dsp.DataPublisher.SetLJH3(i, timebase, nrows, ncols, filename)
			}
		}
		ds.writingState.Active = true
		ds.writingState.Paused = false
		ds.writingState.BasePath = path
		ds.writingState.Filename = fmt.Sprintf(filenamePattern, "chan*", "ljh")
		log.Printf("%v/%v channels have OFF writing enabled\n", channelsWithOff, len(ds.processors))
	}
	if ds.publishSync.writingChan != nil {
		ds.publishSync.writingChan <- ds.writingState.Active && !ds.writingState.Paused
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

// ConfigureProjectorsBases calls SetProjectorsBasis on ds.processors[channelIndex]
func (ds *AnySource) ConfigureProjectorsBases(channelIndex int, projectors mat.Dense, basis mat.Dense) error {
	if channelIndex >= len(ds.processors) || channelIndex < 0 {
		return fmt.Errorf("channelIndex out of range, channelIndex=%v, len(ds.processors)=%v", channelIndex, len(ds.processors))
	}
	dsp := ds.processors[channelIndex]
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

// Signed returns a per-channel value: whether data are signed ints.
func (ds *AnySource) Signed() []bool {
	// Objects containing an AnySource can override this, but default is here:
	// all channels are unsigned.
	if ds.signed == nil {
		ds.signed = make([]bool, ds.nchan)
	}
	return ds.signed
}

// VoltsPerArb returns a per-channel value scaling raw into volts.
func (ds *AnySource) VoltsPerArb() []float32 {
	// Objects containing an AnySource can set this up, but here is the default
	if ds.voltsPerArb == nil || len(ds.voltsPerArb) != ds.nchan {
		ds.voltsPerArb = make([]float32, ds.nchan)
		for i := 0; i < ds.nchan; i++ {
			ds.voltsPerArb[i] = 1. / 65535.0
		}
	}
	return ds.voltsPerArb
}

// setDefaultChannelNames defensively sets channel names of the appropriate length.
// They should have been set in DataSource.Sample()
func (ds *AnySource) setDefaultChannelNames() {
	// If the number of channel names is correct, assume it was set in Sample, as expected.
	if len(ds.chanNames) == ds.nchan {
		return
	}
	ds.chanNames = make([]string, ds.nchan)
	ds.chanNumbers = make([]int, ds.nchan)
	for i := 0; i < ds.nchan; i++ {
		ds.chanNames[i] = fmt.Sprintf("chan%d", i)
		ds.chanNumbers[i] = i
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

	// Start a PublishSync to publish the # of records written
	ds.publishSync = NewPublishSync(ds.nchan)
	go ds.publishSync.Run()

	// Channels onto which we'll put data produced by this source
	ds.output = make([]chan DataSegment, ds.nchan)
	for i := 0; i < ds.nchan; i++ {
		ds.output[i] = make(chan DataSegment)
	}

	// Launch goroutines to drain the data produced by this source
	ds.processors = make([]*DataStreamProcessor, ds.nchan)
	ds.runDone.Add(ds.nchan)
	signed := ds.Signed()
	vpa := ds.VoltsPerArb()

	// Load last trigger state from config file
	var fts []FullTriggerState
	if err := viper.UnmarshalKey("trigger", &fts); err != nil {
		// could not read trigger state from config file.
		fts = []FullTriggerState{}
	}
	tsptrs := make([]*TriggerState, ds.nchan)
	for i, ts := range fts {
		for _, channelIndex := range ts.ChannelIndicies {
			if channelIndex < ds.nchan {
				tsptrs[channelIndex] = &(fts[i].TriggerState)
			}
		}
	}
	// Use defaultTS for any channels not in the stored state.
	// This will be needed any time you have more channels than in the
	// last saved configuration. All trigger types are disabled.
	defaultTS := TriggerState{
		AutoTrigger:  false,
		AutoDelay:    250 * time.Millisecond,
		EdgeTrigger:  false,
		EdgeLevel:    100,
		EdgeRising:   true,
		LevelTrigger: false,
		LevelLevel:   4000,
	}

	for channelIndex, dataSegmentChan := range ds.output {
		dsp := NewDataStreamProcessor(channelIndex, ds.broker, ds.publishSync.numberWrittenChans[channelIndex])
		dsp.Name = ds.chanNames[channelIndex]
		dsp.SampleRate = ds.sampleRate
		dsp.stream.signed = signed[channelIndex]
		dsp.stream.voltsPerArb = vpa[channelIndex]
		ds.processors[channelIndex] = dsp

		ts := tsptrs[channelIndex]
		if ts == nil {
			ts = &defaultTS
		}
		dsp.TriggerState = *ts

		// Publish Records and Summaries over ZMQ by default
		dsp.SetPubRecords()
		dsp.SetPubSummaries()

		// This goroutine will run until the ds.abortSelf channel or the ch==ds.output[channelIndex]
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
	if ds.abortSelf != nil {
		select {
		case <-ds.abortSelf:
		default:
			close(ds.abortSelf)
		}
	}
	ds.broker.Stop()
	ds.publishSync.Stop()
	ds.CloseOutputs()
	return nil
}

// CloseOutputs closes all channels that carry buffers of data for downstream processing.
func (ds *AnySource) CloseOutputs() {
	ds.runMutex.Lock()
	defer ds.runMutex.Unlock()
	for _, ch := range ds.output {
		close(ch)
	}
	ds.output = nil
}

// FullTriggerState used to collect channels that share the same TriggerState
type FullTriggerState struct {
	ChannelIndicies []int
	TriggerState
}

// ComputeFullTriggerState uses a map to collect channels with identical TriggerStates, so they
// can be sent all together as one unit.
func (ds *AnySource) ComputeFullTriggerState() []FullTriggerState {

	result := make(map[TriggerState][]int)
	for _, dsp := range ds.processors {
		chans, ok := result[dsp.TriggerState]
		if ok {
			result[dsp.TriggerState] = append(chans, dsp.channelIndex)
		} else {
			result[dsp.TriggerState] = []int{dsp.channelIndex}
		}
	}

	// Now "unroll" that map into a vector of FullTriggerState objects
	fts := []FullTriggerState{}
	for k, v := range result {
		fts = append(fts, FullTriggerState{ChannelIndicies: v, TriggerState: k})
	}
	return fts
}

// ChangeTriggerState changes the trigger state for 1 or more channels.
func (ds *AnySource) ChangeTriggerState(state *FullTriggerState) error {
	if state.ChannelIndicies == nil || len(state.ChannelIndicies) < 1 {
		return fmt.Errorf("got ConfigureTriggers with no valid ChannelIndicies")
	}
	for _, channelIndex := range state.ChannelIndicies {
		if channelIndex < ds.nchan { // Don't trust client to know this number!
			ds.processors[channelIndex].TriggerState = state.TriggerState
		}
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

// SetCoupling is not allowed for generic data sources
func (ds *AnySource) SetCoupling(status CouplingStatus) error {
	return fmt.Errorf("Generic data sources do not support FB/error coupling")
}

// DataSegment is a continuous, single-channel raw data buffer, plus info about (e.g.)
// raw-physical units, first sample’s frame number and sample time. Not yet triggered.
type DataSegment struct {
	rawData         []RawType
	signed          bool
	framesPerSample int // Normally 1, but can be larger if decimated
	firstFramenum   FrameIndex
	firstTime       time.Time
	framePeriod     time.Duration
	voltsPerArb     float32
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
	timeNowInStream := time.Duration(framesNowInStream) * stream.framePeriod
	stream.framesPerSample = segment.framesPerSample
	stream.framePeriod = segment.framePeriod
	stream.firstFramenum = segment.firstFramenum - framesNowInStream
	stream.firstTime = segment.firstTime.Add(-timeNowInStream)
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
	deltaFrames := (L - N) * stream.framesPerSample
	stream.firstFramenum += FrameIndex(deltaFrames)
	stream.firstTime = stream.firstTime.Add(time.Duration(deltaFrames) * stream.framePeriod)
	return N
}

// DataRecord contains a single triggered pulse record.
type DataRecord struct {
	data         []RawType
	trigFrame    FrameIndex
	trigTime     time.Time
	signed       bool // do we interpret the data as signed values?
	channelIndex int
	presamples   int
	voltsPerArb  float32 // "volts" or other physical unit per raw unit
	sampPeriod   float32

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
