package dastard

import "math"

func (dc *DataChannel) triggerAt(segment *DataSegment, i int) *DataRecord {
	data := make([]RawType, dc.NSamples)
	copy(data, segment.rawData[i-dc.NPresamples:i+dc.NSamples-dc.NPresamples])
	tf := segment.firstFramenum + int64(i)
	tt := segment.TimeOf(i)
	record := &DataRecord{data: data, trigFrame: tf, trigTime: tt,
		channum: dc.Channum}
	return record
}

// TriggerData analyzes a DataSegment to find and generate triggered records
/* Consider a design where we find all possible triggers of one type, all of the
next, etc, and then merge all lists giving precedence to earlier over later triggers? */
func (dc *DataChannel) TriggerData(segment *DataSegment) (records []*DataRecord, secondaries []*DataRecord) {
	nd := len(segment.rawData)
	raw := segment.rawData

	// Step 1: compute where the primary triggers are.
	trigList := triggerList{channum: dc.Channum}

	// Step 1a: compute all edge triggers on a first pass. Separated by at least 1 record length
	if dc.EdgeTrigger {
		for i := dc.NPresamples; i < nd+dc.NPresamples-dc.NSamples; i++ {
			diff := raw[i] + raw[i-1] - (raw[i-2] + raw[i-3])
			if (dc.EdgeRising && diff >= dc.EdgeLevel) ||
				(dc.EdgeFalling && diff <= -dc.EdgeLevel) {
				newRecord := dc.triggerAt(segment, i)
				records = append(records, newRecord)
				trigList.frames = append(trigList.frames, newRecord.trigFrame)
				i += dc.NSamples
			}
		}
	}

	// Step 1b: compute all level triggers on a second pass. Any separation counts at first, but only insert them
	// in the list of triggers if they are properly separated from the edge triggers.
	if dc.LevelTrigger {
		j := 0
		nextET := int64(math.MaxInt64)
		if len(trigList.frames) > j {
			nextET = trigList.frames[j] - segment.firstFramenum
		}
		for i := dc.NPresamples; i < nd+dc.NPresamples-dc.NSamples; i++ {
			if int64(i) > nextET-int64(dc.NSamples) {
				i = int(nextET) + dc.NSamples - 1
				j++
				if len(trigList.frames) > j {
					nextET = trigList.frames[j] - segment.firstFramenum
				} else {
					nextET = math.MaxInt64
				}
				continue
			}
			if (dc.LevelRising && raw[i] >= dc.LevelLevel && raw[i-1] < dc.LevelLevel) ||
				(!dc.LevelRising && raw[i] <= dc.LevelLevel && raw[i-1] > dc.LevelLevel) {
				newRecord := dc.triggerAt(segment, i)
				records = append(records, newRecord)
				trigList.frames = append(trigList.frames, newRecord.trigFrame)
			}
		}
	}

	// Step 1c: compute all auto triggers, wherever they fit in between edge+level.
	// Step 1c: compute all noise triggers, wherever they fit in between edge+level.

	// Step 2: send the primary trigger list to the group trigger broker and await its
	// answer about when the secondary triggers are.
	dc.Broker.PrimaryTrigs <- trigList
	secondaryTrigList := <-dc.Broker.SecondaryTrigs[dc.Channum]
	for _, st := range secondaryTrigList {
		secondaries = append(secondaries, dc.triggerAt(segment, int(st-segment.firstFramenum)))
	}
	return
}
