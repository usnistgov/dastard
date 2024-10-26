package dastard

import (
	"fmt"
	"math"
	"slices"
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
	AutoTrigger   bool // Whether to have automatic (timed) triggers
	AutoDelay     time.Duration
	AutoVetoRange RawType // Veto any auto triggers when (max-min) exceeds this value (if it's >0)

	LevelTrigger bool // Whether to trigger records when the level exceeds some value
	LevelRising  bool
	LevelLevel   RawType

	EdgeTrigger bool // Whether to trigger records when the "local derivative" exceeds some value
	EdgeRising  bool
	EdgeFalling bool
	EdgeLevel   int32
	EdgeMulti   bool // enable EdgeMulti (actually used in triggering)

	EMTBackwardCompatibleRPCFields // used to allow the old RPC messages to still work
	EMTState
}

// create a record using dsp.NPresamples and dsp.NSamples
func (dsp *DataStreamProcessor) triggerAt(i int) *DataRecord {
	return dsp.triggerAtSpecificSamples(i, dsp.NPresamples, dsp.NSamples)
}

// create a record with NPresamples and NSamples passed as arguments
func (dsp *DataStreamProcessor) triggerAtSpecificSamples(i int, NPresamples int, NSamples int) *DataRecord {

	data := make([]RawType, NSamples)
	stream := dsp.stream
	if i < NPresamples {
		fmt.Printf("We would panic, except for returning nil, with i=%d, NPre=%d, NSamp=%d\n",
			i, NPresamples, NSamples)
		return nil
	}
	copy(data, stream.rawData[i-NPresamples:i+NSamples-NPresamples])
	tf := stream.firstFrameIndex + FrameIndex(i)
	tt := stream.TimeOf(i)
	sampPeriod := float32(1.0 / dsp.SampleRate)
	record := &DataRecord{data: data, trigFrame: tf, trigTime: tt,
		channelIndex: dsp.channelIndex, signed: stream.signed,
		voltsPerArb: stream.voltsPerArb,
		presamples:  NPresamples, sampPeriod: sampPeriod}
	return record
}

// firstPotentialTriggerFrame returns the first frame at which a trigger can be allowed in
// the current segment, for non-auto-type triggers.
func (dsp *DataStreamProcessor) firstPotentialTriggerFrame() int {
	// dsp.LastTrigger stores the frame of the last trigger found by the most recent invocation of TriggerData
	mindelay := dsp.NSamples
	nextPotentialTrig := int(dsp.LastTrigger-dsp.stream.firstFrameIndex) + mindelay
	if nextPotentialTrig < dsp.NPresamples {
		return dsp.NPresamples
	}
	return nextPotentialTrig
}

// firstPotentialAutoTriggerFrame returns the first frame at which an auto trigger can be allowed in
// the current segment. The auto trigger delay is considered when determining the first frame.
func (dsp *DataStreamProcessor) firstPotentialAutoTriggerFrame() int {
	// dsp.LastTrigger stores the frame of the last trigger found by the most recent invocation of TriggerData
	mindelay := dsp.NSamples
	autoDelaySamples := int(dsp.AutoDelay.Seconds()*dsp.SampleRate + 0.5)
	if autoDelaySamples > mindelay {
		mindelay = autoDelaySamples
	}
	nextPotentialTrig := int(dsp.LastTrigger-dsp.stream.firstFrameIndex) + mindelay
	if nextPotentialTrig < dsp.NPresamples {
		return dsp.NPresamples
	}
	return nextPotentialTrig
}

func (dsp *DataStreamProcessor) edgeTriggerComputeAppend(records []*DataRecord) []*DataRecord {
	if !dsp.EdgeTrigger {
		return records
	}
	raw := dsp.stream.rawData
	ndata := len(raw)

	// Solve the problem of signed data by shifting all values up by 2^15
	if dsp.stream.signed {
		raw = make([]RawType, ndata)
		copy(raw, dsp.stream.rawData)
		for i := 0; i < ndata; i++ {
			raw[i] += 32768
		}
	}

	for i := dsp.firstPotentialTriggerFrame(); i < ndata+dsp.NPresamples-dsp.NSamples; i++ {
		diff := int32(raw[i]) + int32(raw[i-1]) - int32(raw[i-2]) - int32(raw[i-3])
		if (dsp.EdgeRising && diff >= dsp.EdgeLevel) ||
			(dsp.EdgeFalling && diff <= -dsp.EdgeLevel) {
			newRecord := dsp.triggerAt(i)
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
	raw := dsp.stream.rawData
	ndata := len(raw)
	nsamp := FrameIndex(dsp.NSamples)

	idxNextTrig := 0
	nFoundTrigs := len(records)
	nextFoundTrig := FrameIndex(math.MaxInt64)
	if nFoundTrigs > 0 {
		nextFoundTrig = records[idxNextTrig].trigFrame - dsp.stream.firstFrameIndex
	}

	// Solve the problem of signed data by shifting all values up by 2^15
	threshold := dsp.LevelLevel
	if dsp.stream.signed {
		threshold += 32768
		raw = make([]RawType, ndata)
		copy(raw, dsp.stream.rawData)
		for i := 0; i < ndata; i++ {
			raw[i] += 32768
		}
	}

	// Normal loop through all samples in triggerable range
	for i := dsp.firstPotentialTriggerFrame(); i < ndata+dsp.NPresamples-dsp.NSamples; i++ {

		// Now skip over 2 record's worth of samples (minus 1) if an edge trigger is too soon in future.
		// Existing edge triggers get priority, vetoing (1 record minus 1 sample) into the past
		// and 1 record into the future.
		if FrameIndex(i)+nsamp > nextFoundTrig {
			i = int(nextFoundTrig) + dsp.NSamples - 1
			idxNextTrig++
			if nFoundTrigs > idxNextTrig {
				nextFoundTrig = records[idxNextTrig].trigFrame - dsp.stream.firstFrameIndex
			} else {
				nextFoundTrig = math.MaxInt64
			}
			continue
		}

		// If you get here, a level trigger is permissible. Check for it.
		if (dsp.LevelRising && raw[i] >= threshold && raw[i-1] < threshold) ||
			(!dsp.LevelRising && raw[i] <= threshold && raw[i-1] > threshold) {
			newRecord := dsp.triggerAt(i)
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
	stream := dsp.stream
	raw := stream.rawData
	ndata := len(raw)
	nsamp := FrameIndex(dsp.NSamples)
	npre := FrameIndex(dsp.NPresamples)

	idxNextTrig := 0
	nFoundTrigs := len(records)
	nextFoundTrig := FrameIndex(math.MaxInt64)
	if nFoundTrigs > 0 {
		nextFoundTrig = records[idxNextTrig].trigFrame - stream.firstFrameIndex
	}

	nextPotentialTrig := FrameIndex(dsp.firstPotentialAutoTriggerFrame())
	autoDelaySamples := FrameIndex(dsp.AutoDelay.Seconds()*dsp.SampleRate + 0.5)
	if autoDelaySamples < FrameIndex(dsp.NSamples) {
		autoDelaySamples = FrameIndex(dsp.NSamples)
	}

	// Loop through all potential trigger times.
	for nextPotentialTrig+nsamp-npre < FrameIndex(ndata) {
		if nextPotentialTrig+nsamp <= nextFoundTrig {
			// Auto trigger is allowed: no conflict with previously found non-auto triggers
			// Now verify that it's not vetoed by the allowed (max-min) limit, if any.
			vetoed := false
			if dsp.AutoVetoRange > 0 {
				begin := nextPotentialTrig - npre
				finish := begin + nsamp
				maxD := dsp.stream.rawData[begin]
				minD := maxD
				for i := begin + 1; i < finish; i++ {
					d := dsp.stream.rawData[i]
					if d > maxD {
						maxD = d
					} else if d < minD {
						minD = d
					}
				}
				vetoed = maxD-minD >= dsp.AutoVetoRange
			}
			if !vetoed {
				newRecord := dsp.triggerAt(int(nextPotentialTrig))
				records = append(records, newRecord)
			}
			nextPotentialTrig += autoDelaySamples

		} else {
			// Auto trigger not allowed: it conflicts with previously found non-auto triggers
			nextPotentialTrig = nextFoundTrig + autoDelaySamples
			idxNextTrig++
			if nFoundTrigs > idxNextTrig {
				nextFoundTrig = records[idxNextTrig].trigFrame - stream.firstFrameIndex
			} else {
				nextFoundTrig = math.MaxInt64
			}
		}
	}
	sort.Sort(RecordSlice(records))
	return records
}

// TriggerData analyzes a DataSegment to find and generate triggered records.
// All edge-multitriggers are found, OR [all edge triggers are found, then level
// triggers, then auto, and noise triggers].
// Returns slice of complete DataRecord objects, while dsp.lastTrigList stores
// a triggerList object just when the triggers happened.
func (dsp *DataStreamProcessor) TriggerData() (records []*DataRecord) {

	// Step 1: compute where the primary triggers are, one pass per trigger type.
	// Edge Multi triggers are exclusive of all other types.
	if dsp.EdgeMulti {
		records = dsp.edgeMultiTriggerComputeAppend(records)

	} else {
		// Step 1a: compute all edge triggers on a first pass. Separated by at least 1 record length.
		records = dsp.edgeTriggerComputeAppend(records)

		// Step 1b: compute all level triggers on a second pass. Only insert them
		// in the list of triggers if they are properly separated from the edge triggers.
		records = dsp.levelTriggerComputeAppend(records)

		// Step 1c: compute all auto triggers, wherever they fit in between edge+level.
		records = dsp.autoTriggerComputeAppend(records)

		// TODO Step 1d: compute all noise triggers, wherever they fit in between edge+level.
		// At the moment, we don't implement this. Historically, a "noise trigger" was like an
		// auto trigger, but with an edge trigger to veto records that would have contained
		// pulses. We have found little need for this approach and leave this comment as a
		// placeholder in case we ever want to add it.
	}

	// Any errors in the triggering calculation produce nil values for a record; remove them now.
	records = slices.DeleteFunc(records, func(r *DataRecord) bool { return r == nil })

	// Step 2: store the last trigger for the next invocation of TriggerData
	if len(records) > 0 {
		dsp.LastTrigger = records[len(records)-1].trigFrame
	}

	// Step 3: prepare the primary trigger list from the DataRecord list.
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
	stream := dsp.stream
	for _, st := range secondaryTrigList {
		secRecords = append(secRecords, dsp.triggerAt(int(st-stream.firstFrameIndex)))
	}
	return secRecords
}

// RecordSlice attaches the methods of sort.Interface to slices of *DataRecords, sorting in increasing order.
type RecordSlice []*DataRecord

func (p RecordSlice) Len() int           { return len(p) }
func (p RecordSlice) Less(i, j int) bool { return p[i].trigFrame < p[j].trigFrame }
func (p RecordSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
