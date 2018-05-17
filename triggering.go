package dastard

import (
	"fmt"
	"math"
	"sort"
)

type triggerList struct {
	channum int
	frames  []FrameIndex
}

func (dsp *DataStreamProcessor) triggerAt(segment *DataSegment, i int) *DataRecord {
	data := make([]RawType, dsp.NSamples)
	copy(data, segment.rawData[i-dsp.NPresamples:i+dsp.NSamples-dsp.NPresamples])
	tf := segment.firstFramenum + FrameIndex(i)
	tt := segment.TimeOf(i)
	sampPeriod := float32(1.0 / dsp.SampleRate)
	record := &DataRecord{data: data, trigFrame: tf, trigTime: tt,
		channum: dsp.Channum, presamples: dsp.NPresamples, sampPeriod: sampPeriod}
	return record
}

func (dsp *DataStreamProcessor) edgeTriggerComputeAppend(records []*DataRecord) []*DataRecord {
	if !dsp.EdgeTrigger {
		return records
	}
	segment := &dsp.stream.DataSegment
	raw := segment.rawData
	ndata := len(raw)

	for i := dsp.NPresamples; i < ndata+dsp.NPresamples-dsp.NSamples; i++ {
		diff := int32(raw[i]+raw[i-1]) - int32(raw[i-2]+raw[i-3])
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
		r := records[idxNextTrig]
		nextFoundTrig = r.trigFrame - segment.firstFramenum
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
				nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFramenum
			} else {
				nextFoundTrig = math.MaxInt64
			}
		}

		// If you get here, a level trigger is permissible. Check for it.
		if (dsp.LevelRising && raw[i] >= dsp.LevelLevel && raw[i-1] < dsp.LevelLevel) ||
			(!dsp.LevelRising && raw[i] <= dsp.LevelLevel && raw[i-1] > dsp.LevelLevel) {
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
	if delaySamples < 0 {
		panic(fmt.Sprintf("delay samples=%v", delaySamples))
	}
	idxNextTrig := 0
	nFoundTrigs := len(records)
	nextFoundTrig := FrameIndex(math.MaxInt64)
	var nextFoundTrigLen int
	if nFoundTrigs > 0 {
		r := records[idxNextTrig]
		nextFoundTrig = r.trigFrame - segment.firstFramenum
		nextFoundTrigLen = len(r.data)
	}

	// dsp.LastTrigger stores the frame of the last trigger found by the most recent invocation of TriggerData
	nextPotentialTrig := dsp.LastTrigger - segment.firstFramenum + delaySamples
	if nextPotentialTrig < npre {
		nextPotentialTrig = npre
	}

	// Loop through all potential trigger times.
	for nextPotentialTrig+nsamp-npre < FrameIndex(ndata) {
		if nextPotentialTrig+nsamp < nextFoundTrig {
			// auto trigger is allowed
			newRecord := dsp.triggerAt(segment, int(nextPotentialTrig))
			records = append(records, newRecord)
			nextPotentialTrig += delaySamples + FrameIndex(nsamp)

		} else {
			// auto trigger not allowed
			nextPotentialTrig = nextFoundTrig + delaySamples + FrameIndex(nextFoundTrigLen)
			idxNextTrig++
			if nFoundTrigs > idxNextTrig {
				r := records[idxNextTrig]
				nextFoundTrig = r.trigFrame - segment.firstFramenum
				nextFoundTrigLen = len(r.data)
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
func (dsp *DataStreamProcessor) TriggerData() (records []*DataRecord, secondaries []*DataRecord) {
	// Step 1: compute where the primary triggers are, one pass per trigger type.
	// Step 1a: compute all edge triggers on a first pass. Separated by at least 1 record length
	records = dsp.edgeTriggerComputeAppend(records)

	// Step 1b: compute all level triggers on a second pass. Only insert them
	// in the list of triggers if they are properly separated from the edge triggers.
	records = dsp.levelTriggerComputeAppend(records)

	// Step 1c: compute all auto triggers, wherever they fit in between edge+level.
	records = dsp.autoTriggerComputeAppend(records)

	// Step 1.5: note the last trigger for the next invocation of TriggerData
	if len(records) > 0 {
		dsp.LastTrigger = records[len(records)-1].trigFrame
	}

	// TODO Step 1d: compute all noise triggers, wherever they fit in between edge+level.
	//

	// Step 2: send the primary trigger list to the group trigger broker and await its
	// answer about when the secondary triggers are.

	// Step 2a: prepare the primary trigger list from the DataRecord list
	trigList := triggerList{channum: dsp.Channum}
	trigList.frames = make([]FrameIndex, len(records))
	for i, r := range records {
		trigList.frames[i] = r.trigFrame
	}

	// Step 2b: send the primary list to the group trigger broker; receive the secondary list.
	dsp.Broker.PrimaryTrigs <- trigList
	secondaryTrigList := <-dsp.Broker.SecondaryTrigs[dsp.Channum]
	segment := &dsp.stream.DataSegment
	for _, st := range secondaryTrigList {
		secondaries = append(secondaries, dsp.triggerAt(segment, int(st-segment.firstFramenum)))
	}

	// leave one full possible trigger in the stream
	// trigger algorithms should not inspect the last NSamples samples
	dsp.stream.TrimKeepingN(dsp.NSamples)
	return
}

// RecordSlice attaches the methods of sort.Interface to slices, sorting in increasing order.
type RecordSlice []*DataRecord

func (p RecordSlice) Len() int           { return len(p) }
func (p RecordSlice) Less(i, j int) bool { return p[i].trigFrame < p[j].trigFrame }
func (p RecordSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
