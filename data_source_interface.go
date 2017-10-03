package dastard

// DataSource is the interface for hardware or simulated data sources that
// produce data.
type DataSource interface {
	Start() error
	Stop() error
	Sample() error
	BlockingRead(<-chan struct{}) error
	Outputs() []chan []RawType
}
