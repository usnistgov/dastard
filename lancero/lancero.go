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
	// adapter   *adapter
	collector *collector
	device    *lanceroDevice
}

// NewLancero generates and returns a new Lancero object and configures it properly. The devnum
// value is used to select among /dev/lancero_user0, lancero_user1, etc., if there are more
// than 1 card in the computer. Usually, you'll use 0 here.
func NewLancero(devnum int) *Lancero {
	lan := new(Lancero)
	dev, err := openLanceroDevice(devnum)
	if err != nil {
		return nil
	}
	lan.device = dev
	lan.collector = &collector{device: dev, simulated: false}
	// Same for adapter
	// lan.adapter = &adapter{...}

	return lan
}
