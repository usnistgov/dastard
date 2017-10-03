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

// DataSegment is a continuous, single-channel raw data buffer, plus info about (e.g.) raw-physical units, first sample’s frame number and sample time. Not yet triggered.
type DataSegment struct {
	rawData       []RawType
	firstFramenum int64
	firstTime     time.Time
	// something about raw-physical conversion???
	// facts about the data source? sample rate?
}
