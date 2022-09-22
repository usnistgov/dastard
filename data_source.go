package dastard

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/usnistgov/dastard/getbytes"

	"github.com/spf13/viper"
	"gonum.org/v1/gonum/mat"
)

// RawType holds raw signal data.
type RawType uint16

// FrameIndex is used for counting raw data frames.
type FrameIndex int64

// GroupIndex represents the specifics of a channel group.
// Channel numbers should be globally unique across a DataSource.
type GroupIndex struct {
	Firstchan int // first channel number in this group
	Nchan     int // how many channels in this group
}

// ByGroup implements sort.Interface for []GroupIndex so we can sort such slices.
type ByGroup []GroupIndex

func (g ByGroup) Len() int           { return len(g) }
func (g ByGroup) Swap(i, j int)      { g[i], g[j] = g[j], g[i] }
func (g ByGroup) Less(i, j int) bool { return g[i].Firstchan < g[j].Firstchan }

// SourceState is used to indicate the active/inactive/transition state of data sources
type SourceState int

// Names for the possible values of SourceState
const (
	Inactive SourceState = iota // Source is not active
	Starting                    // Source is in transition to Active state
	Active                      // Source is actively acquiring data
	Stopping                    // Source is in transition to Inactive state
)

// DataSource is the interface for hardware or simulated data sources that
// produce data.
type DataSource interface {
	Sample() error
	PrepareRun(int, int) error
	PrepareChannels() error
	StartRun() error
	Stop() error
	Running() bool
	GetState() SourceState
	SetStateStarting() error
	SetStateInactive() error
	getNextBlock() chan *dataBlock
	Nchan() int
	ChanGroups() []GroupIndex
	SamplePeriod() time.Duration
	VoltsPerArb() []float32
	ComputeGroupTriggerState() GroupTriggerState
	ComputeFullTriggerState() []FullTriggerState
	ComputeWritingState() *WritingState
	WritingIsActive() bool
	ChannelNames() []string
	ConfigurePulseLengths(int, int) error
	ConfigureProjectorsBases(int, *mat.Dense, *mat.Dense, string) error
	ChangeTriggerState(*FullTriggerState) error
	ConfigureMixFraction(*MixFractionObject) ([]float64, error)
	WriteControl(*WriteControlConfig) error
	SetCoupling(CouplingStatus) error
	ChangeGroupTrigger(turnon bool, gts *GroupTriggerState) error
	StopTriggerCoupling() error
	SetExperimentStateLabel(time.Time, string) error
	ChannelsWithProjectors() []int
	ProcessSegments(*dataBlock) error
	RunDoneActivate()
	RunDoneDeactivate()
	ShouldAutoRestart() bool
	getPulseLengths() (int, int, error)
}

// RunDoneActivate adds one to ds.runDone, this should only be called in Start
func (ds *AnySource) RunDoneActivate() {
	ds.sourceStateLock.Lock()
	defer ds.sourceStateLock.Unlock()
	ds.sourceState = Active
	ds.runDone.Add(1)
}

// RunDoneDeactivate calls Done on ds.runDone, this should only be called (by defer) in Start
func (ds *AnySource) RunDoneDeactivate() {
	ds.sourceStateLock.Lock()
	ds.sourceState = Inactive
	ds.runDone.Done()
	ds.sourceStateLock.Unlock()
}

// RunDoneWait returns when the source run is done, i.e., the source is stopped
func (ds *AnySource) RunDoneWait() {
	ds.runDone.Wait()
}

// ShouldAutoRestart true if source should be auto-restarted after an error
func (ds *AnySource) ShouldAutoRestart() bool {
	return ds.shouldAutoRestart
}

// ConfigureMixFraction provides a default implementation for all non-lancero sources that
// don't need the mix
func (ds *AnySource) ConfigureMixFraction(mfo *MixFractionObject) ([]float64, error) {
	return nil, fmt.Errorf("source type %s does not support Mix", ds.name)
}

// getNextBlock returns the channel on which data sources send data and any errors.
// Waiting on this channel = waiting on the source to produce a data block.
func (ds *AnySource) getNextBlock() chan *dataBlock {
	return ds.nextBlock
}

// Start will start the given DataSource, including sampling its data for # channels. Steps:
//  1. Sample: a method implemented per source that determines the # of channels and other
//     internal facts that we need to know.
//  2. PrepareChannels: an AnySource method (but overridden by certain other sources). Set up
//     the channel numbering and naming system.
//  3. PrepareRun: an AnySource method to do the actions that any source needs
//     before starting the actual acquisition phase.
//  4. StartRun: a per-source method to begin data acquisition, if relevant.
//  5. Loop over calls to ds.blockingRead(), a per-source method that waits for data.
//
// When done with the loop, close all channels to DataStreamProcessor objects.
func Start(ds DataSource, queuedRequests chan func(), Npresamp int, Nsamples int) error {
	if err := ds.SetStateStarting(); err != nil {
		return err
	}
	if err := ds.Sample(); err != nil {
		ds.SetStateInactive()
		return err
	}

	if err := ds.PrepareChannels(); err != nil {
		ds.SetStateInactive()
		return err
	}

	if err := ds.PrepareRun(Npresamp, Nsamples); err != nil {
		ds.SetStateInactive()
		return err
	}

	ds.RunDoneActivate() // Call RunDoneDeactivate inside CoreLoop when it returns.
	if err := ds.StartRun(); err != nil {
		ds.RunDoneDeactivate()
		return err
	}

	go CoreLoop(ds, queuedRequests)
	return nil
}

// CoreLoop has the DataSource produce data until graceful stop.
// This will be a long-running goroutine, as long as a source is active.
func CoreLoop(ds DataSource, queuedRequests chan func()) {
	defer ds.RunDoneDeactivate()
	nextBlock := ds.getNextBlock()

	for {
		// Use select to interleave 2 activities that should NOT be done concurrently:
		// 1. Handle RPC requests to change data processing parameters (e.g. trigger).
		// 2. Handle new data and process it.
		select {

		// Handle RPC requests
		case request := <-queuedRequests:
			request()

		// Handle data, or recognize the end of data
		case block, ok := <-nextBlock:
			if !ok {
				// nextBlock was closed in the data production loop when abortSelf was closed
				log.Println("nextBlock channel was closed; stopping the source normally")
				return

			} else if block.err != nil {
				// errors in block indicate a problem with source: need to close down
				log.Printf("nextBlock received Error; stopping source: %s\n", block.err.Error())
				return
			}
			if err := ds.ProcessSegments(block); err != nil {
				log.Printf("AnySource.ProcessSegments returns Error; stopping source: %s\n", err.Error())
				panic("Panic to stop source when processSegments errors. This seems to keep the Lancero working better than stopping the source")
			}
			// In some sources, ds.getNextBlock has to be called again to initiate the next
			// data acquisition step (Lancero, specifically).
			nextBlock = ds.getNextBlock()
		}
	}
}

// Stop tells the data supply to deactivate.
func (ds *AnySource) Stop() error {
	ds.sourceStateLock.Lock()
	switch ds.sourceState {
	case Inactive:
		ds.sourceStateLock.Unlock()
		return fmt.Errorf("AnySource not active, cannot stop")

	case Starting:
		panic("Called Stop on a Starting source; how to handle this??")

	case Active:
		log.Println("AnySource.Stop() was called to stop an active source")
		// This is the normal case: Stop on an Active source

	case Stopping:
		// Ignore Stop if source is already Stopping.
		ds.sourceStateLock.Unlock()
		return nil
	}
	ds.sourceState = Stopping
	closeIfOpen(ds.abortSelf)
	ds.sourceStateLock.Unlock()

	ds.RunDoneWait()
	ds.groupKeysSorted = make([]GroupIndex, 0)
	if ds.writingState.Active { // if writing, Stop writing
		wcc := WriteControlConfig{Request: "STOP"}
		ds.WriteControl(&wcc)
	}
	return nil
}

func closeIfOpen(c chan struct{}) {
	select {
	case <-c:
		log.Println("warning: you tried to close a channel twice, but Dastard outsmarted you")
	default:
		close(c)
	}
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

// dataBlock contains a block of data (one segment per data stream)
// This implies that dataBlock has synchronized data across all parts of a source (all Lancero
// cards, all Abaco ring buffers and channel groups, etc.).
type dataBlock struct {
	segments                 []DataSegment
	externalTriggerRowcounts []int64
	nSamp                    int
	err                      error
}

// AnySource implements features common to any object that implements
// DataSource, including the output channels and the abort channel.
type AnySource struct {
	nchan           int           // how many channels to provide
	name            string        // what kind of source is this?
	chanNames       []string      // one name per channel
	chanNumbers     []int         // names have format "prefixNumber", this is the number
	rowColCodes     []RowColCode  // one RowColCode per channel
	groupKeysSorted []GroupIndex  // sorted slice of channel group information
	voltsPerArb     []float32     // the physical units per arb, one per channel
	sampleRate      float64       // samples per second
	samplePeriod    time.Duration // time per sample
	lastread        time.Time
	nextFrameNum    FrameIndex // frame number for the next frame we will receive
	processors      []*DataStreamProcessor
	abortSelf       chan struct{}   // Signal to the core loop of active sources to stop
	nextBlock       chan *dataBlock // Signal from the core loop that a block is ready to process
	broker          *TriggerBroker
	configError     error // Any error that arose when configuring the source (before Start)

	shouldAutoRestart   bool // used to tell SourceControl to try to restart this source after an error
	noProcess           bool // Set true only for testing.
	heartbeats          chan Heartbeat
	writingState        WritingState
	numberWrittenTicker *time.Ticker
	sourceState         SourceState
	sourceStateLock     sync.Mutex // guards sourceState
	runDone             sync.WaitGroup
	readCounter         int
	channelsPerPixel    int
}

// SamplePeriod returns the sample period of the underlying source.
func (ds *AnySource) SamplePeriod() time.Duration {
	return ds.samplePeriod
}

// getPulseLengths returns (NPresamples, NSamples, err)
func (ds *AnySource) getPulseLengths() (int, int, error) {
	if len(ds.processors) < 1 {
		return 0, 0, fmt.Errorf("len(ds.processors)=%v, cannot getPulseLengths", len(ds.processors))
	}
	NPresamples := ds.processors[0].NPresamples
	NSamples := ds.processors[0].NSamples
	for _, dsp := range ds.processors {
		if dsp.NPresamples != NPresamples || dsp.NSamples != NSamples {
			return 0, 0, fmt.Errorf("not all processors have same record lengths, NPresamples %v, dsp.NPresample %v, NSamples %v, dsp.NSamples %v",
				NPresamples, dsp.NPresamples, NSamples, dsp.NSamples)
		}
	}
	return NPresamples, NSamples, nil
}

// ProcessSegments processes a single outstanding segment for each of ds.processors
// in parallel. Returns when all segments have been processed.
// It's more synchronous than our original plan of each dsp launching its own goroutine.
func (ds *AnySource) ProcessSegments(block *dataBlock) error {
	if len(ds.processors) != len(block.segments) {
		panic(fmt.Sprintf("Oh crap! dataBlock contains %d segments but Source has %d processors (channels)",
			len(block.segments), len(ds.processors)))
	}

	// We break processing data segments into 2 halves. Each half can proceed in parallel
	// across all data streams, but we have to synchronize in the middle in order for
	// secondary triggers to be computed and distributed.

	// Each processor (channel) ingests and analyzes/publishes its segment in parallel.
	// Use a WaitGroup to make all finish before secondary triggers can be computed.
	var wg sync.WaitGroup
	for idx, dsp := range ds.processors {
		segment := block.segments[idx]
		wg.Add(1)
		go func(dsp *DataStreamProcessor) {
			defer wg.Done()
			dsp.processSegment(&segment)
		}(dsp)
	}
	wg.Wait()

	// Build a map to hold triggerList for each channel index, and then ask the TriggerBroker
	// to compute the corresponding slice of secondary trigger FrameIndex values for each
	// channel index.
	allchanTrigList := make(map[int]triggerList)
	for idx, dsp := range ds.processors {
		allchanTrigList[idx] = dsp.lastTrigList
	}
	allSecondaries, err := ds.broker.Distribute(allchanTrigList)
	if err != nil {
		return err
	}
	ds.broker.GenerateTriggerMessages()

	// Each processor (channel) with secondary records analyzes them in parallel.
	if len(allSecondaries) > 0 {
		for idx, dsp := range ds.processors {
			flist := allSecondaries[idx]
			if len(flist) > 0 {
				wg.Add(1)
				go func(dsp *DataStreamProcessor, flist []FrameIndex) {
					defer wg.Done()
					dsp.processSecondaries(flist)
				}(dsp, flist)
			}
		}
	}
	wg.Wait()

	// Clean up: mark the data segments as processed, trim the streams of data we no longer need,
	// and once every 20 reads, flush the output files (but do the files out of phase, so it's not
	// done for all files at once).
	tStart := time.Now()
	for idx, dsp := range ds.processors {
		segment := block.segments[idx]
		segment.processed = true
		dsp.TrimStream()

		if (idx+ds.readCounter)%20 == 0 {
			dsp.Flush()
		}
	}
	ds.readCounter++
	flushDuration := time.Since(tStart)
	if flushDuration > 50*time.Millisecond {
		log.Println("flushDuration", flushDuration)
	}

	numberWritten := make([]int, ds.nchan)
	for i, dsp := range ds.processors {
		numberWritten[i] = dsp.numberWritten
	}
	if err := ds.HandleExternalTriggers(block.externalTriggerRowcounts); err != nil {
		return err
	}

	// all segments will have the same value for droppedFrames and firstFrameIndex, so we just look at the first segment here
	if err := ds.HandleDataDrop(block.segments[0].droppedFrames, int(block.segments[0].firstFrameIndex)); err != nil {
		return err
	}
	if ds.writingState.Active && !ds.writingState.Paused {
		select {
		case <-ds.numberWrittenTicker.C:
			clientMessageChan <- ClientUpdate{tag: "NUMBERWRITTEN",
				state: struct{ NumberWritten []int }{NumberWritten: numberWritten}} // only exported fields are serialized
		default:
		}
	}
	return nil
}

// SetExperimentStateLabel writes to a file with name like XXX_experiment_state.txt
// the file is created upon the first call to this function for a given file writing
func (ds *AnySource) SetExperimentStateLabel(timestamp time.Time, stateLabel string) error {
	return ds.writingState.SetExperimentStateLabel(timestamp, stateLabel)
}

// HandleDataDrop writes to a file in the case that a data drop is detected.
// "Data drop" refers to a case where a read from a source (e.g., the LanceroSource)
// misses some frames of data.
func (ds *AnySource) HandleDataDrop(droppedFrames, firstFrameIndex int) error {
	if droppedFrames > 0 {
		ds.writingState.dataDropsObserved++
		fmt.Printf("DATA DROP. firstFrameIndex %v, droppedFrames %v\n", firstFrameIndex, droppedFrames)
		if ds.writingState.IsActive() {
			// Set up the log file if not already done
			if ds.writingState.dataDropFileBufferedWriter == nil {
				// create file
				var err error
				ds.writingState.dataDropFile, err = os.Create(ds.writingState.DataDropFilename)
				if err != nil {
					return fmt.Errorf("cannot create DataDropFile filename `%v`: err: %v",
						ds.writingState.DataDropFilename, err)
				}
				ds.writingState.dataDropFileBufferedWriter = bufio.NewWriter(ds.writingState.dataDropFile)
				// write header
				_, err = ds.writingState.dataDropFileBufferedWriter.WriteString("# first framenum after drop, number of dropped frames\n")
				if err != nil {
					return fmt.Errorf("cannot write header to dataDroFileBufferedWriter, err %v", err)
				}
			}

			// Log to the dataDropFile
			line := fmt.Sprintf("%12d %8d\n", firstFrameIndex, droppedFrames)
			_, err := ds.writingState.dataDropFileBufferedWriter.WriteString(line)
			if err != nil {
				return fmt.Errorf("cannot write to externalTriggerFileBufferedWriter, err %v", err)
			}
		}
	}
	select { // occasionally flush file and send message about number of observed data drops
	case <-ds.writingState.dataDropTicker.C:
		// flush file
		if ds.writingState.dataDropFileBufferedWriter != nil {
			err := ds.writingState.dataDropFileBufferedWriter.Flush()
			if err != nil {
				return fmt.Errorf("cannot flush dataDropFileBufferedWriter, err %v", err)
			}
		}

	default:
	}
	if droppedFrames > 0 || !ds.writingState.dataDropHaveSentAMessage {
		// send message
		clientMessageChan <- ClientUpdate{tag: "DATADROP",
			state: struct {
				TotalObserved int
			}{TotalObserved: ds.writingState.dataDropsObserved}} // only exported fields are serialized
		ds.writingState.dataDropHaveSentAMessage = true
	}
	return nil
}

// HandleExternalTriggers writes external trigger to a file, creates that file if neccesary, and sends out messages
// with the number of external triggers observed
func (ds *AnySource) HandleExternalTriggers(externalTriggerRowcounts []int64) error {
	if ds.writingState.externalTriggerFileBufferedWriter == nil && len(externalTriggerRowcounts) > 0 &&
		ds.writingState.ExternalTriggerFilename != "" {
		// setup external trigger file if neccesary
		var err error
		ds.writingState.externalTriggerFile, err = os.Create(ds.writingState.ExternalTriggerFilename)
		if err != nil {
			return fmt.Errorf("cannot create ExternalTriggerFile, %v", err)
		}
		ds.writingState.externalTriggerFileBufferedWriter = bufio.NewWriter(ds.writingState.externalTriggerFile)
		// write header
		_, err1 := ds.writingState.externalTriggerFileBufferedWriter.WriteString("# external trigger rowcounts as int64 binary data follows, rowcounts = framecounts*nrow+row\n")
		if err1 != nil {
			return fmt.Errorf("cannot write header to externalTriggerFileBufferedWriter, err %v", err)
		}
	}
	ds.writingState.externalTriggerNumberObserved += len(externalTriggerRowcounts)
	if ds.writingState.externalTriggerFileBufferedWriter != nil {
		_, err := ds.writingState.externalTriggerFileBufferedWriter.Write(getbytes.FromSliceInt64(externalTriggerRowcounts))
		if err != nil {
			return fmt.Errorf("cannot write to externalTriggerFileBufferedWriter, err %v", err)
		}
	}
	select { // occasionally flush and send message about number of observed external triggers
	case <-ds.writingState.externalTriggerTicker.C:
		if ds.writingState.externalTriggerFileBufferedWriter != nil {
			err := ds.writingState.externalTriggerFileBufferedWriter.Flush()
			if err != nil {
				return fmt.Errorf("cannot flush externalTriggerFileBufferedWriter, err %v", err)
			}
		}
		clientMessageChan <- ClientUpdate{tag: "EXTERNALTRIGGER",
			state: struct {
				NumberObservedInLastSecond int
			}{NumberObservedInLastSecond: ds.writingState.externalTriggerNumberObserved}} // only exported fields are serialized
		ds.writingState.externalTriggerNumberObserved = 0
	default:
	}

	return nil
}

// makeDirectory creates directory of the form basepath/20060102/0000 where
// the 4-digit subdirectory counts separate file-writing occasions.
// It also returns the formatting code for use in an Sprintf call
// (e.g., basepath/20060102/0000/20060102_run0000_%s.%s) and an error, if any.
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
func (ds *AnySource) WriteControl(config *WriteControlConfig) error {
	requestStr := strings.ToUpper(config.Request)
	switch {
	case strings.HasPrefix(requestStr, "PAUSE"):
		for _, dsp := range ds.processors {
			dsp.DataPublisher.SetPause(true)
		}
		ds.writingState.Paused = true

	case strings.HasPrefix(requestStr, "UNPAUSE"):
		if len(config.Request) > 7 {
			// validate format of command "UNPAUSE label"
			if config.Request[7:8] != " " || len(config.Request) == 8 {
				return fmt.Errorf("request format invalid. got::\n%v\nwant someting like: \"UNPAUSE label\"", config.Request)
			}
			stateLabel := config.Request[8:]
			if err := ds.SetExperimentStateLabel(time.Now(), stateLabel); err != nil {
				return err
			}
		}
		for _, dsp := range ds.processors {
			dsp.DataPublisher.SetPause(false)
		}
		ds.writingState.Paused = false

	case strings.HasPrefix(requestStr, "STOP"):
		for _, dsp := range ds.processors {
			dsp.DataPublisher.RemoveLJH22()
			dsp.DataPublisher.RemoveOFF()
			dsp.DataPublisher.RemoveLJH3()
		}
		return ds.writingState.Stop()

	case strings.HasPrefix(requestStr, "START"):
		return ds.writeControlStart(config)

	default:
		return fmt.Errorf("WriteControl config.Request=%q, must be one of (START,STOP,PAUSE,UNPAUSE). Not case sensitive. \"UNPAUSE label\" is also ok",
			config.Request)
	}
	return nil
}

// writeControlStart handles the most complex case of WriteControl: starting to write.
func (ds *AnySource) writeControlStart(config *WriteControlConfig) error {
	if !(config.WriteLJH22 || config.WriteOFF || config.WriteLJH3) {
		return fmt.Errorf("WriteLJH22 and WriteOFF and WriteLJH3 all false")
	}

	for _, dsp := range ds.processors {
		if dsp.DataPublisher.HasLJH22() || dsp.DataPublisher.HasOFF() || dsp.DataPublisher.HasLJH3() {
			return fmt.Errorf(
				"writing already in progress, stop writing before starting again. Currently: LJH22 %v, OFF %v, LJH3 %v",
				dsp.DataPublisher.HasLJH22(), dsp.DataPublisher.HasOFF(), dsp.DataPublisher.HasLJH3())
		}
	}
	if config.WriteOFF {
		// throw an error if no channels have projectors set
		// only channels with projectors set will have OFF files enabled
		anyProjectorsSet := false
		for _, dsp := range ds.processors {
			if dsp.HasProjectors() {
				anyProjectorsSet = true
				break
			}
		}
		if !anyProjectorsSet {
			return fmt.Errorf("no projectors are loaded, OFF files require projectors")
		}
	}
	if config.MapInternalOnly != nil {
		if len(config.MapInternalOnly.Pixels) != ds.nchan/ds.channelsPerPixel {
			return mapError{msg: fmt.Sprintf("map error: have length %v, want %v, want value calculated as (nchan %v / channelsPerPixel %v)",
				len(config.MapInternalOnly.Pixels), ds.nchan/ds.channelsPerPixel, ds.nchan, ds.channelsPerPixel)}
		}
	}
	path := ds.writingState.BasePath
	if len(config.Path) > 0 {
		path = config.Path
	}
	var err error
	filenamePattern, err := makeDirectory(path)
	if err != nil {
		return fmt.Errorf("could not make directory: %s", err.Error())
	}

	channelsWithOff := 0
	for i, dsp := range ds.processors {
		timebase := 1.0 / dsp.SampleRate
		rccode := ds.rowColCodes[i]
		nrows := rccode.rows()
		ncols := rccode.cols()
		rowNum := rccode.row()
		colNum := rccode.col()
		fps := 1
		var pixel Pixel

		if config.MapInternalOnly != nil {
			channelNumber := ds.chanNumbers[i]
			pixel = config.MapInternalOnly.Pixels[channelNumber-1]
		}
		if dsp.Decimate {
			fps = dsp.DecimateLevel
		}
		if config.WriteLJH22 {
			filename := fmt.Sprintf(filenamePattern, dsp.Name, "ljh")
			dsp.DataPublisher.SetLJH22(i, dsp.NPresamples, dsp.NSamples, fps,
				timebase, DastardStartTime, nrows, ncols, ds.nchan, rowNum, colNum, filename,
				ds.name, ds.chanNames[i], ds.chanNumbers[i], pixel)
		}
		if config.WriteOFF && dsp.HasProjectors() {
			filename := fmt.Sprintf(filenamePattern, dsp.Name, "off")
			dsp.DataPublisher.SetOFF(i, dsp.NPresamples, dsp.NSamples, fps,
				timebase, DastardStartTime, nrows, ncols, ds.nchan, rowNum, colNum, filename,
				ds.name, ds.chanNames[i], ds.chanNumbers[i], dsp.projectors, dsp.basis,
				dsp.modelDescription, pixel)
			channelsWithOff++
		}
		if config.WriteLJH3 {
			filename := fmt.Sprintf(filenamePattern, dsp.Name, "ljh3")
			dsp.DataPublisher.SetLJH3(i, timebase, nrows, ncols, filename)
		}
	}
	return ds.writingState.Start(filenamePattern, path)
}

// ComputeWritingState returns a partial copy of the writingState
func (ds *AnySource) ComputeWritingState() *WritingState {
	return ds.writingState.ComputeState()
}

// WritingIsActive returns whether the current writers are active
func (ds *AnySource) WritingIsActive() bool {
	return ds.writingState.IsActive()
}

// ConfigureProjectorsBases calls SetProjectorsBasis on ds.processors[channelIndex]
func (ds *AnySource) ConfigureProjectorsBases(channelIndex int, projectors *mat.Dense, basis *mat.Dense, modelDescription string) error {
	if channelIndex >= len(ds.processors) || channelIndex < 0 {
		return fmt.Errorf("channelIndex out of range, channelIndex=%v, len(ds.processors)=%v", channelIndex, len(ds.processors))
	}
	dsp := ds.processors[channelIndex]
	return dsp.SetProjectorsBasis(projectors, basis, modelDescription)
}

// ChannelsWithProjectors returns a list of the ChannelIndices of channels that have projectors loaded
func (ds *AnySource) ChannelsWithProjectors() []int {
	result := make([]int, 0)
	for channelIndex := 0; channelIndex < len(ds.processors); channelIndex++ {
		dsp := ds.processors[channelIndex]
		if dsp.HasProjectors() {
			result = append(result, channelIndex)
		}
	}
	return result
}

// Nchan returns the current number of valid channels in the data source.
func (ds *AnySource) Nchan() int {
	return ds.nchan
}

// ChanGroups returns the sorted slice of channel GroupIndex values.
func (ds *AnySource) ChanGroups() []GroupIndex {
	cg := make([]GroupIndex, len(ds.groupKeysSorted))
	copy(cg, ds.groupKeysSorted)
	return cg
}

// Running tells whether the source is actively running.
func (ds *AnySource) Running() bool {
	return ds.GetState() == Active
}

// GetState returns the sourceState value in a race-free fashion
func (ds *AnySource) GetState() SourceState {
	ds.sourceStateLock.Lock()
	defer ds.sourceStateLock.Unlock()
	return ds.sourceState
}

// SetStateStarting sets the sourceState value to Starting in a race-free fashion
func (ds *AnySource) SetStateStarting() error {
	ds.sourceStateLock.Lock()
	defer ds.sourceStateLock.Unlock()
	if ds.sourceState == Inactive {
		ds.sourceState = Starting
		return nil
	}
	return fmt.Errorf("cannot Start() a source that's %v, not Inactive", ds.sourceState)
}

// SetStateInactive sets the sourceState value to Inactive in a race-free fashion
func (ds *AnySource) SetStateInactive() error {
	ds.sourceStateLock.Lock()
	defer ds.sourceStateLock.Unlock()
	ds.sourceState = Inactive
	return nil
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

// PrepareChannels configures an AnySource by initializing all data structures that
// have to do with channels. Other objects that meet the DataSource spec might
// override this default version.
func (ds *AnySource) PrepareChannels() error {
	ds.channelsPerPixel = 1

	ds.groupKeysSorted = make([]GroupIndex, 1)
	cg := GroupIndex{Firstchan: 0, Nchan: ds.nchan}
	ds.groupKeysSorted[0] = cg

	ds.chanNames = make([]string, ds.nchan)
	ds.chanNumbers = make([]int, ds.nchan)
	for i := 0; i < ds.nchan; i++ {
		ds.chanNames[i] = fmt.Sprintf("chan%d", i)
		ds.chanNumbers[i] = i
	}
	return nil
}

// PrepareRun configures an AnySource by initializing all data structures that
// cannot be prepared until we know the number of channels. It's an error for
// ds.nchan to be less than 1.
func (ds *AnySource) PrepareRun(Npresamples int, Nsamples int) error {
	if ds.nchan <= 0 {
		return fmt.Errorf("PrepareRun could not run with %d channels (require > 0)", ds.nchan)
	}

	ds.abortSelf = make(chan struct{})
	ds.nextBlock = make(chan *dataBlock)

	// Create a TriggerBroker to handle secondary triggering
	ds.broker = NewTriggerBroker(ds.nchan)

	ds.numberWrittenTicker = time.NewTicker(1 * time.Second)
	ds.writingState.externalTriggerTicker = time.NewTicker(time.Second * 1)
	ds.writingState.dataDropTicker = time.NewTicker(time.Second * 10)

	// Create processors to drain the data produced by this source, one per channel.
	ds.processors = make([]*DataStreamProcessor, ds.nchan)
	vpa := ds.VoltsPerArb()

	// Load last trigger state from config file
	var fts []FullTriggerState
	if err := viper.UnmarshalKey("trigger", &fts); err != nil {
		// could not read trigger state from config file.
		fts = []FullTriggerState{}
	}
	tsptrs := make([]*TriggerState, ds.nchan)
	for i, ts := range fts {
		for _, channelIndex := range ts.ChannelIndices {
			if channelIndex < ds.nchan {
				tsptrs[channelIndex] = &(fts[i].TriggerState)
				tsptrs[channelIndex].EdgeMulti = false // fix for issue #271
				// won't be neccesary if we get rid of the old EMT fields used for rpc compatability
				// basically EMTState is not saved, so it has threshold = 0
				// so if we come back with EdgeMulti=true
				// then we trigger on every sample, and it crashes
				// should we even load trigger states at all?
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

	for channelIndex := range ds.processors {
		dsp := NewDataStreamProcessor(channelIndex, ds.broker, Npresamples, Nsamples)
		dsp.Name = ds.chanNames[channelIndex]
		dsp.ChannelNumber = ds.chanNumbers[channelIndex]
		dsp.SampleRate = ds.sampleRate
		dsp.stream.voltsPerArb = vpa[channelIndex]
		ds.processors[channelIndex] = dsp

		ts := tsptrs[channelIndex]
		if ts == nil {
			ts = &defaultTS
		}
		dsp.TriggerState = *ts

		// Publish Records and Record Summaries over ZMQ. Not optional at this time.
		dsp.SetPubRecords()
		dsp.SetPubSummaries()
	}
	ds.lastread = time.Now()
	return nil
}

// ComputeGroupTriggerState returns the current `GroupTriggerState`.
func (ds *AnySource) ComputeGroupTriggerState() GroupTriggerState {
	return ds.broker.computeGroupTriggerState()
}

// FullTriggerState used to collect channels that share the same TriggerState
type FullTriggerState struct {
	ChannelIndices []int
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
		fts = append(fts, FullTriggerState{ChannelIndices: v, TriggerState: k})
	}
	return fts
}

// ChangeTriggerState changes the trigger state for 1 or more channels.
func (ds *AnySource) ChangeTriggerState(state *FullTriggerState) error {
	if state.ChannelIndices == nil || len(state.ChannelIndices) < 1 {
		return fmt.Errorf("got ConfigureTriggers with no valid ChannelIndices")
	}
	for _, channelIndex := range state.ChannelIndices {
		if channelIndex >= ds.nchan {
			return fmt.Errorf("channelIndex %v is >= ds.nchan %v", channelIndex, ds.nchan)
		}
	}
	for _, channelIndex := range state.ChannelIndices {
		dsp := ds.processors[channelIndex]
		if err := dsp.ConfigureTrigger(state.TriggerState); err != nil {
			return err
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
	if npre < 3 || // edgeTrigger looks at npre-3
		nsamp < 1 || // require at least 1 sample
		nsamp < npre+1 { // require at least one post trigger sample
		return fmt.Errorf("ConfigurePulseLengths nsamp %v, npre %v are invalid", nsamp, npre)
	}
	for _, dsp := range ds.processors {
		if err := dsp.ConfigurePulseLengths(nsamp, npre); err != nil {
			return err
		}
	}
	return nil
}

// SetCoupling sets the FB/Err coupling status.
// SetCoupling(NoCoupling) is allowed for generic data sources, but other values are not.
func (ds *AnySource) SetCoupling(status CouplingStatus) error {
	if status == NoCoupling {
		return nil
	}
	return fmt.Errorf("generic data sources do not support FB/error coupling")
}

// ChangeGroupTrigger either adds or deletes the connections in `gts` (add when `turnon` is true,
// otherwise delete).
func (ds *AnySource) ChangeGroupTrigger(turnon bool, gts *GroupTriggerState) error {
	// changer is either the Add or Delete function, depending on turnon
	changer := ds.broker.DeleteConnection
	if turnon {
		changer = ds.broker.AddConnection
	}
	for source, receivers := range gts.Connections {
		for _, receiver := range receivers {
			changer(source, receiver)
		}
	}
	return nil
}

// StopTriggerCoupling turns off all trigger coupling, including all group triggers and FB/Err coupling.
func (ds *AnySource) StopTriggerCoupling() error {
	return ds.broker.StopTriggerCoupling()
}

// DataSegment is a continuous, single-channel raw data buffer, plus info about (e.g.)
// raw-physical units, first sample’s frame number and sample time. Not yet triggered.
type DataSegment struct {
	rawData         []RawType
	signed          bool
	framesPerSample int // Normally 1, but can be larger if decimated
	firstFrameIndex FrameIndex
	firstTime       time.Time
	framePeriod     time.Duration
	voltsPerArb     float32
	processed       bool
	droppedFrames   int // normally zero, positive if dropped frames detected
}

// NewDataSegment generates a pointer to a new, initialized DataSegment object.
func NewDataSegment(data []RawType, framesPerSample int, firstFrame FrameIndex,
	firstTime time.Time, period time.Duration) *DataSegment {
	seg := DataSegment{rawData: data, framesPerSample: framesPerSample,
		firstFrameIndex: firstFrame, firstTime: firstTime, framePeriod: period}
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
	stream.firstFrameIndex = segment.firstFrameIndex - framesNowInStream
	stream.firstTime = segment.firstTime.Add(-timeNowInStream)
	stream.rawData = append(stream.rawData, segment.rawData...)
	stream.signed = segment.signed
	// There are multiple sources of truth on whether something is signed.
	// This syncs with segment.signed, which comes from deep in a datasource,
	// but there is also AnySource.signed which seems indepdendent (?)
	stream.samplesSeen += len(segment.rawData)
}

// TrimKeepingN will trim (discard) all but the last N values in the DataStream.
// N larger than the number of available values is NOT an error.
// Returns the number of values in the stream after trimming (should be <= N).
func (stream *DataStream) TrimKeepingN(N int) int {
	L := len(stream.rawData)
	if N >= L {
		return L
	}
	copy(stream.rawData[:N], stream.rawData[L-N:L])
	stream.rawData = stream.rawData[:N]
	deltaFrames := (L - N) * stream.framesPerSample
	stream.firstFrameIndex += FrameIndex(deltaFrames)
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
	pretrigDelta float64
	pulseAverage float64
	pulseRMS     float64
	peakValue    float64

	// Real time Analysis quantities
	modelCoefs     []float64
	residualStdDev float64
}
