package dastard

import "time"

// DataSource is the interface for hardware or simulated data sources that
// produce data.
type DataSource interface {
	Sample() error
	Start() error
	Stop() error
	BlockingRead(<-chan struct{}) error
	Outputs() []chan DataSegment
}

// DataSegment is a continuous, single-channel raw data buffer, plus info about (e.g.)
// raw-physical units, first sample’s frame number and sample time. Not yet triggered.
type DataSegment struct {
	rawData         []RawType
	framesPerSample int // Normally 1, but can be larger if decimated
	firstFramenum   int64
	firstTime       time.Time
	framePeriod     time.Duration
	// something about raw-physical conversion???
	// facts about the data source?
}

// NewDataSegment generates a pointer to a new, initialized DataSegment object.
func NewDataSegment(data []RawType, framesPerSample int, firstFrame int64,
	firstTime time.Time, period time.Duration) *DataSegment {
	seg := DataSegment{rawData: data, framesPerSample: framesPerSample,
		firstFramenum: firstFrame, firstTime: firstTime, framePeriod: period}
	return &seg
}

// DataStream models a continuous stream of data, though we have only a finite
// amount at any time. For now, it's semantically different from a DataSegment,
// yet they need the same information.
type DataStream struct {
	DataSegment
	samplesSeen int
}

// NewDataStream generates a pointer to a new, initialized DataStream object.
func NewDataStream(data []RawType, framesPerSample int, firstFrame int64,
	firstTime time.Time, period time.Duration) *DataStream {
	ds := DataStream{DataSegment: DataSegment{rawData: data, framesPerSample: framesPerSample,
		firstFramenum: firstFrame, firstTime: firstTime, framePeriod: period},
		samplesSeen: len(data)}
	return &ds
}

// AppendSegment will append the data in segment to the DataStream.
// It will update the frame/time counters to be consistent with the appended
// segment, not necessarily with the previous values.
func (stream *DataStream) AppendSegment(segment *DataSegment) {
	oldFrameCount := int64(len(stream.rawData) * segment.framesPerSample)
	stream.framesPerSample = segment.framesPerSample
	stream.rawData = append(stream.rawData, segment.rawData...)
	stream.firstFramenum = segment.firstFramenum - oldFrameCount
	stream.firstTime = segment.firstTime.Add(-time.Duration(oldFrameCount) * stream.framePeriod)
	stream.samplesSeen += len(segment.rawData)
	// TODO: this doesn't handle decimated data!
}

// TrimKeepingN will trim (remove) all but the last N values in the DataStream
func (stream *DataStream) TrimKeepingN(N int) {
	L := len(stream.rawData)
	if N >= L {
		return
	}
	copy(stream.rawData[:N], stream.rawData[L-N:L])
	stream.rawData = stream.rawData[:N]
	stream.firstFramenum += int64(L - N)
	stream.firstTime = stream.firstTime.Add(time.Duration(L-N) * stream.framePeriod)
}

// DataRecord contains a single triggered pulse record.
type DataRecord struct {
	data      []RawType
	trigFrame int64
	trigTime  time.Time

	// trigger type?

	// Analyzed quantities
	pretrigMean  float64
	pulseAverage float64
	pulseRMS     float64
	peakValue    float64
}
