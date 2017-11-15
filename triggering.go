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
			diff := int32(raw[i]+raw[i-1]) - int32(raw[i-2]+raw[i-3])
			if (dc.EdgeRising && diff >= dc.EdgeLevel) ||
				(dc.EdgeFalling && diff <= -dc.EdgeLevel) {
				// fmt.Printf("Found edge at %d with diff=%d\n", i, diff)
				newRecord := dc.triggerAt(segment, i)
				records = append(records, newRecord)
				trigList.frames = append(trigList.frames, newRecord.trigFrame)
				i += dc.NSamples
			}
		}
	}

	// Step 1b: compute all level triggers on a second pass. Only insert them
	// in the list of triggers if they are properly separated from the edge triggers.
	if dc.LevelTrigger {
		idxET := 0
		nextEdgeTrig := int64(math.MaxInt64)
		if len(trigList.frames) > idxET {
			nextEdgeTrig = trigList.frames[idxET] - segment.firstFramenum
		}
		// Normal loop through all samples in triggerable range
		for i := dc.NPresamples; i < nd+dc.NPresamples-dc.NSamples; i++ {

			// Now skip over 2 record's worth of samples (minus 1) if an edge trigger is too soon in future.
			// Notice how this works: edge triggers get priority, vetoing 1 record minus 1 sample into the past
			// and 1 record into the future.
			if int64(i) > nextEdgeTrig-int64(dc.NSamples) {
				i = int(nextEdgeTrig) + dc.NSamples - 1
				idxET++
				if len(trigList.frames) > idxET {
					nextEdgeTrig = trigList.frames[idxET] - segment.firstFramenum
				} else {
					nextEdgeTrig = math.MaxInt64
				}
				continue
			}

			// If you get here, a level trigger is permissible. Check for it.
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
