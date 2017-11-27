package dastard

import (
	"math"
	"sort"
)

type triggerList struct {
	channum int
	frames  []int64
}

func (dc *DataChannel) triggerAt(segment *DataSegment, i int) *DataRecord {
	data := make([]RawType, dc.NSamples)
	copy(data, segment.rawData[i-dc.NPresamples:i+dc.NSamples-dc.NPresamples])
	tf := segment.firstFramenum + int64(i)
	tt := segment.TimeOf(i)
	record := &DataRecord{data: data, trigFrame: tf, trigTime: tt,
		channum: dc.Channum}
	return record
}

func (dc *DataChannel) edgeTriggerData(segment *DataSegment, records []*DataRecord) []*DataRecord {
	ndata := len(segment.rawData)
	raw := segment.rawData
	if dc.EdgeTrigger {
		for i := dc.NPresamples; i < ndata+dc.NPresamples-dc.NSamples; i++ {
			diff := int32(raw[i]+raw[i-1]) - int32(raw[i-2]+raw[i-3])
			if (dc.EdgeRising && diff >= dc.EdgeLevel) ||
				(dc.EdgeFalling && diff <= -dc.EdgeLevel) {
				newRecord := dc.triggerAt(segment, i)
				records = append(records, newRecord)
				i += dc.NSamples
			}
		}
	}
	return records
}

func (dc *DataChannel) levelTriggerData(segment *DataSegment, records []*DataRecord) []*DataRecord {
	if !dc.LevelTrigger {
		return records
	}

	ndata := len(segment.rawData)
	nsamp := int64(dc.NSamples)
	raw := segment.rawData

	idxNextTrig := 0
	nFoundTrigs := len(records)
	nextFoundTrig := int64(math.MaxInt64)
	if nFoundTrigs > 0 {
		nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFramenum
	}

	// Normal loop through all samples in triggerable range
	for i := dc.NPresamples; i < ndata+dc.NPresamples-dc.NSamples; i++ {

		// Now skip over 2 record's worth of samples (minus 1) if an edge trigger is too soon in future.
		// Notice how this works: edge triggers get priority, vetoing 1 record minus 1 sample into the past
		// and 1 record into the future.
		if int64(i)+nsamp > nextFoundTrig {
			i = int(nextFoundTrig) + dc.NSamples - 1
			idxNextTrig++
			if nFoundTrigs > idxNextTrig {
				nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFramenum
			} else {
				nextFoundTrig = math.MaxInt64
			}
		}

		// If you get here, a level trigger is permissible. Check for it.
		if (dc.LevelRising && raw[i] >= dc.LevelLevel && raw[i-1] < dc.LevelLevel) ||
			(!dc.LevelRising && raw[i] <= dc.LevelLevel && raw[i-1] > dc.LevelLevel) {
			newRecord := dc.triggerAt(segment, i)
			records = append(records, newRecord)
		}
	}
	sort.Sort(RecordSlice(records))
	return records
}

func (dc *DataChannel) autoTriggerData(segment *DataSegment, records []*DataRecord) []*DataRecord {
	if !dc.AutoTrigger {
		return records
	}

	ndata := len(segment.rawData)
	nsamp := int64(dc.NSamples)
	npre := int64(dc.NPresamples)

	delaySamples := int64(dc.AutoDelay.Seconds()*dc.SampleRate + 0.5)
	idxNextTrig := 0
	nFoundTrigs := len(records)
	nextFoundTrig := int64(math.MaxInt64)
	if nFoundTrigs > 0 {
		nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFramenum
	}

	nextPotentialTrig := dc.LastTrigger - segment.firstFramenum + delaySamples
	if nextPotentialTrig < npre {
		nextPotentialTrig = npre
	}

	// Loop through all potential trigger times.
	for nextPotentialTrig+nsamp-npre < int64(ndata) {
		if nextPotentialTrig+nsamp < nextFoundTrig {
			// auto trigger is allowed
			newRecord := dc.triggerAt(segment, int(nextPotentialTrig))
			records = append(records, newRecord)
			nextPotentialTrig += delaySamples

		} else {
			// auto trigger not allowed
			nextPotentialTrig = nextFoundTrig + delaySamples
			idxNextTrig++
			if nFoundTrigs > idxNextTrig {
				nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFramenum
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
func (dc *DataChannel) TriggerData(segment *DataSegment) (records []*DataRecord, secondaries []*DataRecord) {

	// Step 1: compute where the primary triggers are, one pass per trigger type.
	// Step 1a: compute all edge triggers on a first pass. Separated by at least 1 record length
	records = dc.edgeTriggerData(segment, records)

	// Step 1b: compute all level triggers on a second pass. Only insert them
	// in the list of triggers if they are properly separated from the edge triggers.
	records = dc.levelTriggerData(segment, records)

	// Step 1c: compute all auto triggers, wherever they fit in between edge+level.
	records = dc.autoTriggerData(segment, records)

	// TODO Step 1d: compute all noise triggers, wherever they fit in between edge+level.
	//

	// Step 2: send the primary trigger list to the group trigger broker and await its
	// answer about when the secondary triggers are.

	// Step 2a: prepare the primary trigger list from the DataRecord list
	trigList := triggerList{channum: dc.Channum}
	trigList.frames = make([]int64, len(records))
	for i, r := range records {
		trigList.frames[i] = r.trigFrame
	}
	if len(records) > 0 {
		dc.LastTrigger = trigList.frames[len(records)-1]
	}

	// Step 2b: send the primary list to the group trigger broker; receive the secondary list.
	dc.Broker.PrimaryTrigs <- trigList
	secondaryTrigList := <-dc.Broker.SecondaryTrigs[dc.Channum]
	for _, st := range secondaryTrigList {
		secondaries = append(secondaries, dc.triggerAt(segment, int(st-segment.firstFramenum)))
	}
	return
}

// RecordSlice attaches the methods of sort.Interface to slices, sorting in increasing order.
type RecordSlice []*DataRecord

func (p RecordSlice) Len() int           { return len(p) }
func (p RecordSlice) Less(i, j int) bool { return p[i].trigFrame < p[j].trigFrame }
func (p RecordSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
