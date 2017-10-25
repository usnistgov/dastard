package dastard

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
func (dc *DataChannel) TriggerData(segment *DataSegment) (records []*DataRecord) {
	nd := len(segment.rawData)
	raw := segment.rawData
	if dc.LevelTrigger {
		for i := dc.NPresamples; i < nd+dc.NPresamples-dc.NSamples; i++ {
			if raw[i] >= dc.LevelLevel && raw[i-1] < dc.LevelLevel {
				records = append(records, dc.triggerAt(segment, i))
				i += dc.NSamples
			}
		}
	}
	return
}
