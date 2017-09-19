// Package lancero provides an interface to all Lancero SGDMA
// character devices, read/write from/to registers of SOPC slaves, wait for
// SOPC component interrupt events and handle the cyclic mode of SGDMA.
// Exports object Lancero for general use. Internally, that object works with
// the lower-level adapter, collector, and lanceroDevice.
//
package lancero

// Notes:
// Want 4 objects:
// Lancero (high-level, exported). This isn't in the C++ version.
// adapter (for the ring buffer)
// collector (for the data serialization engine)
// lanceroDevice (for the low-level register communication). This is lancero in C++

// Lancero is the high-level object used to manipulate all user-space functions of
// the Lancero device driver.
type Lancero struct {
	adapter   *adapter
	collector *collector
	device    *lanceroDevice
}

// NewLancero generates and returns a new Lancero object and configures it properly. The devnum
// value is used to select among /dev/lancero_user0, lancero_user1, etc., if there are more
// than 1 card in the computer. Usually, you'll use 0 here.
func NewLancero(devnum int) (*Lancero, error) {
	lan := new(Lancero)
	dev, err := openLanceroDevice(devnum)
	if err != nil {
		return nil, err
	}
	lan.device = dev
	lan.collector = &collector{device: dev, simulated: false}
	lan.adapter = &adapter{device: dev}
	lan.adapter.verbosity = 3
	lan.adapter.allocateRingBuffer(1<<24, 1<<23)

	lan.adapter.status()
	lan.adapter.inspect()

	return lan, nil
}

// Release all resources used by this lancero device.
func (lan *Lancero) Close() {
	if lan.device != nil {
		lan.device.Close()
	}
	if lan.adapter != nil {
        lan.adapter.freeBuffer()
	}
}

// StartAdapter starts the ring buffer adapter, waiting up to waitSeconds sec for it to work.
func (lan *Lancero) StartAdapter(waitSeconds int) error {
	return lan.adapter.start(waitSeconds)
}

// StopAdapter stops the ring buffer adapter.
func (lan *Lancero) StopAdapter() error {
	return lan.adapter.stop()
}

// CollectorConfigure configures the data serialization component.
func (lan *Lancero) CollectorConfigure(linePeriod, dataDelay int, channelMask uint32,
	frameLength int) error {
	lp := uint32(linePeriod)
	dd := uint32(dataDelay)
	cm := uint32(channelMask)
	fl := uint32(frameLength)
	return lan.collector.configure(lp, dd, cm, fl)
}

// StartCollector starts the data serializer.
func (lan *Lancero) StartCollector(simulate bool) error {
	return lan.collector.start(simulate)
}

// StopCollector stops the data serializer.
func (lan *Lancero) StopCollector() error {
	return lan.collector.stop()
}

// Wait blocks until there is data in the ring buffer adapter.
func (lan *Lancero) Wait() error {
	return lan.adapter.wait()
}

// AvailableBuffers returns the ring buffer segment(s) now ready for reading.
func (lan *Lancero) AvailableBuffers() (buffers [][]byte, totalBytes int, err error) {
	return lan.adapter.availableBuffers()
}

// ReleaseBytes instructed the ring buffer adapter to release nBytes bytes for over-writing.
func (lan *Lancero) ReleaseBytes(nBytes int) error {
	return lan.adapter.releaseBytes(uint32(nBytes))
}

// InspectAdapter prints adapter status info and returns the status word.
func (lan *Lancero) InspectAdapter() uint32 {
	return lan.adapter.inspect()
}
