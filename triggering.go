package dastard

import (
	"math"
	"sort"
	"time"
)

type triggerList struct {
	channelIndex                int
	frames                      []FrameIndex
	keyFrame                    FrameIndex
	keyTime                     time.Time
	sampleRate                  float64
	firstFrameThatCannotTrigger FrameIndex
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

	EdgeMulti                      bool // enable EdgeMulti (actually used in triggering)
	EMTBackwardCompatibleRPCFields      // used to allow the old RPC messages to still work
	EMTState

	// TODO: group source/rx info.
}

// create a record using dsp.NPresamples and dsp.NSamples
func (dsp *DataStreamProcessor) triggerAt(segment *DataSegment, i int) *DataRecord {
	record := dsp.triggerAtSpecificSamples(segment, i, dsp.NPresamples, dsp.NSamples)
	return record
}

// create a record with NPresamples and NSamples passed as arguments
func (dsp *DataStreamProcessor) triggerAtSpecificSamples(segment *DataSegment, i int, NPresamples int, NSamples int) *DataRecord {
	data := make([]RawType, NSamples)
	// fmt.Printf("triggerAtSpecificSamples i %v, NPresamples %v, NSamples %v, len(rawData) %v\n", i, NPresamples, NSamples, len(segment.rawData))
	copy(data, segment.rawData[i-NPresamples:i+NSamples-NPresamples])
	tf := segment.firstFrameIndex + FrameIndex(i)
	tt := segment.TimeOf(i)
	sampPeriod := float32(1.0 / dsp.SampleRate)
	record := &DataRecord{data: data, trigFrame: tf, trigTime: tt,
		channelIndex: dsp.channelIndex, signed: segment.signed,
		voltsPerArb: segment.voltsPerArb,
		presamples:  NPresamples, sampPeriod: sampPeriod}
	return record
}

func (dsp *DataStreamProcessor) edgeTriggerComputeAppend(records []*DataRecord) []*DataRecord {
	if !dsp.EdgeTrigger {
		return records
	}
	segment := &dsp.stream.DataSegment
	raw := segment.rawData
	ndata := len(raw)

	// Solve the problem of signed data by shifting all values up by 2^15
	if dsp.stream.signed {
		raw = make([]RawType, ndata)
		copy(raw, segment.rawData)
		for i := 0; i < ndata; i++ {
			raw[i] += 32768
		}
	}

	for i := dsp.NPresamples; i < ndata+dsp.NPresamples-dsp.NSamples; i++ {
		diff := int32(raw[i]) + int32(raw[i-1]) - int32(raw[i-2]) - int32(raw[i-3])
		if (dsp.EdgeRising && diff >= dsp.EdgeLevel) ||
			(dsp.EdgeFalling && diff <= -dsp.EdgeLevel) {
			newRecord := dsp.triggerAt(segment, i)
			records = append(records, newRecord)
			i += dsp.NSamples
		}
	}
	return records
}

func (dsp *DataStreamProcessor) levelTriggerComputeAppend(records []*DataRecord) []*DataRecord {
	if !dsp.LevelTrigger {
		return records
	}
	segment := &dsp.stream.DataSegment
	raw := segment.rawData
	ndata := len(raw)
	nsamp := FrameIndex(dsp.NSamples)

	idxNextTrig := 0
	nFoundTrigs := len(records)
	nextFoundTrig := FrameIndex(math.MaxInt64)
	if nFoundTrigs > 0 {
		nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFrameIndex
	}

	// Solve the problem of signed data by shifting all values up by 2^15
	threshold := dsp.LevelLevel
	if dsp.stream.signed {
		threshold += 32768
		raw = make([]RawType, ndata)
		copy(raw, segment.rawData)
		for i := 0; i < ndata; i++ {
			raw[i] += 32768
		}
	}

	// Normal loop through all samples in triggerable range
	for i := dsp.NPresamples; i < ndata+dsp.NPresamples-dsp.NSamples; i++ {

		// Now skip over 2 record's worth of samples (minus 1) if an edge trigger is too soon in future.
		// Notice how this works: edge triggers get priority, vetoing (1 record minus 1 sample) into the past
		// and 1 record into the future.
		if FrameIndex(i)+nsamp > nextFoundTrig {
			i = int(nextFoundTrig) + dsp.NSamples - 1
			idxNextTrig++
			if nFoundTrigs > idxNextTrig {
				nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFrameIndex
			} else {
				nextFoundTrig = math.MaxInt64
			}
			continue
		}

		// If you get here, a level trigger is permissible. Check for it.
		if (dsp.LevelRising && raw[i] >= threshold && raw[i-1] < threshold) ||
			(!dsp.LevelRising && raw[i] <= threshold && raw[i-1] > threshold) {
			newRecord := dsp.triggerAt(segment, i)
			records = append(records, newRecord)
		}
	}
	sort.Sort(RecordSlice(records))
	return records
}

func (dsp *DataStreamProcessor) autoTriggerComputeAppend(records []*DataRecord) []*DataRecord {
	if !dsp.AutoTrigger {
		return records
	}
	segment := &dsp.stream.DataSegment
	raw := segment.rawData
	ndata := len(raw)
	nsamp := FrameIndex(dsp.NSamples)
	npre := FrameIndex(dsp.NPresamples)

	delaySamples := FrameIndex(dsp.AutoDelay.Seconds()*dsp.SampleRate + 0.5)
	if delaySamples < nsamp {
		delaySamples = nsamp
	}
	idxNextTrig := 0
	nFoundTrigs := len(records)
	nextFoundTrig := FrameIndex(math.MaxInt64)
	if nFoundTrigs > 0 {
		nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFrameIndex
	}

	// dsp.LastTrigger stores the frame of the last trigger found by the most recent invocation of TriggerData
	nextPotentialTrig := dsp.LastTrigger - segment.firstFrameIndex + delaySamples
	if nextPotentialTrig < npre {
		nextPotentialTrig = npre
	}

	// Loop through all potential trigger times.
	for nextPotentialTrig+nsamp-npre < FrameIndex(ndata) {
		if nextPotentialTrig+nsamp <= nextFoundTrig {
			// auto trigger is allowed: no conflict with previously found non-auto triggers
			newRecord := dsp.triggerAt(segment, int(nextPotentialTrig))
			records = append(records, newRecord)
			nextPotentialTrig += delaySamples

		} else {
			// auto trigger not allowed: conflict with previously found non-auto triggers
			nextPotentialTrig = nextFoundTrig + delaySamples
			idxNextTrig++
			if nFoundTrigs > idxNextTrig {
				nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFrameIndex
			} else {
				nextFoundTrig = math.MaxInt64
			}
		}
	}
	sort.Sort(RecordSlice(records))
	return records
}

// TriggerData analyzes a DataSegment to find and generate triggered records.
// All edge triggers are found, then level triggers, then auto and noise triggers.
// Returns slice of complete DataRecord objects, while dsp.lastTrigList stores
// a triggerList object just when the triggers happened.
func (dsp *DataStreamProcessor) TriggerData() (records []*DataRecord) {
	if dsp.EdgeMulti {
		// EdgeMulti does not play nice with other triggers!!
		records = dsp.edgeMultiTriggerComputeAppend(records)
		trigList := triggerList{channelIndex: dsp.channelIndex}
		trigList.frames = make([]FrameIndex, len(records))
		for i, r := range records {
			trigList.frames[i] = r.trigFrame
		}
		trigList.keyFrame = dsp.stream.DataSegment.firstFrameIndex
		trigList.keyTime = dsp.stream.DataSegment.firstTime
		trigList.sampleRate = dsp.SampleRate
		trigList.firstFrameThatCannotTrigger = dsp.stream.DataSegment.firstFrameIndex +
			FrameIndex(len(dsp.stream.rawData)) - FrameIndex(dsp.NSamples-dsp.NPresamples)

		dsp.lastTrigList = trigList
		return records
	}

	// Step 1: compute where the primary triggers are, one pass per trigger type.

	// Step 1a: compute all edge triggers on a first pass. Separated by at least 1 record length
	records = dsp.edgeTriggerComputeAppend(records)

	// Step 1b: compute all level triggers on a second pass. Only insert them
	// in the list of triggers if they are properly separated from the edge triggers.
	records = dsp.levelTriggerComputeAppend(records)

	// Step 1c: compute all auto triggers, wherever they fit in between edge+level.
	records = dsp.autoTriggerComputeAppend(records)

	// TODO Step 1d: compute all noise triggers, wherever they fit in between edge+level.
	//

	// Step 1e: note the last trigger for the next invocation of TriggerData
	if len(records) > 0 {
		dsp.LastTrigger = records[len(records)-1].trigFrame
	}

	// Step 2: prepare the primary trigger list from the DataRecord list.
	trigList := triggerList{channelIndex: dsp.channelIndex}
	trigList.frames = make([]FrameIndex, len(records))
	for i, r := range records {
		trigList.frames[i] = r.trigFrame
	}
	trigList.keyFrame = dsp.stream.DataSegment.firstFrameIndex
	trigList.keyTime = dsp.stream.DataSegment.firstTime
	trigList.sampleRate = dsp.SampleRate
	trigList.firstFrameThatCannotTrigger = dsp.stream.DataSegment.firstFrameIndex +
		FrameIndex(len(dsp.stream.rawData)) - FrameIndex(dsp.NSamples-dsp.NPresamples)

	dsp.lastTrigList = trigList
	return records
}

// TriggerDataSecondary converts a slice of secondary trigger frame numbers into a slice
// of records, the secondary trigger records.
func (dsp *DataStreamProcessor) TriggerDataSecondary(secondaryTrigList []FrameIndex) (secRecords []*DataRecord) {
	segment := &dsp.stream.DataSegment
	for _, st := range secondaryTrigList {
		secRecords = append(secRecords, dsp.triggerAt(segment, int(st-segment.firstFrameIndex)))
	}
	return secRecords
}

// RecordSlice attaches the methods of sort.Interface to slices of *DataRecords, sorting in increasing order.
type RecordSlice []*DataRecord

func (p RecordSlice) Len() int           { return len(p) }
func (p RecordSlice) Less(i, j int) bool { return p[i].trigFrame < p[j].trigFrame }
func (p RecordSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
