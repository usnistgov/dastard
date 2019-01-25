package dastard

import "fmt"

// AbacoDevice represents a single Abaco device-special file.
type AbacoDevice struct {
}

// AbacoSource represents all Abaco devices that supply data.
type AbacoSource struct {
	devices []*AbacoDevice
	AnySource
}

// NewAbacoSource creates a new AbacoSource.
func NewAbacoSource() (*AbacoSource, error) {
	source := new(AbacoSource)
	source.name = "Abaco"
	source.devices = make([]*AbacoDevice, 0)

	return source, nil
}

// Delete closes all Abaco cards
func (as *AbacoSource) Delete() {
	for i, _ := range as.devices {
		fmt.Printf("Closing device %d.\n", i)
	}
}

// AbacoSourceConfig holds the arguments needed to call AbacoSource.Configure by RPC.
type AbacoSourceConfig struct {
}

// Configure sets up the internal buffers with given size, speed, and min/max.
func (as *AbacoSource) Configure(config *AbacoSourceConfig) (err error) {
	as.sourceStateLock.Lock()
	defer as.sourceStateLock.Unlock()
	return nil
}

// Sample determines key data facts by sampling some initial data.
func (as *AbacoSource) Sample() error {
	return nil
}

// StartRun tells the hardware to switch into data streaming mode.
// For Abaco µMUX systems, we need to consume any initial data that constitutes
// a fraction of a frame.
func (as *AbacoSource) StartRun() error {
	return nil
}

// stop ends the data streaming on all active lancero devices.
func (as *AbacoSource) stop() error {
	return nil
}
