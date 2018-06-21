// Package lancero provides an interface to all Lancero scatter-gather DMA
// character devices, read/write from/to registers of SOPC slaves, wait for
// SOPC component interrupt events and handle the cyclic mode of SGDMA.
// Exports object Lancero for general use. Internally, that object works with
// the lower-level adapter, collector, and lanceroDevice.
//
package lancero

import (
	"fmt"
	"time"
)

// Lanceroer is the interaface shared by Lancero and NoHardware
// used to allow testing without lancero hardware
type Lanceroer interface {
	ChangeRingBuffer(int, int) error
	Close() error
	StartAdapter(int) error
	StopAdapter() error
	CollectorConfigure(int, int, uint32, int) error
	StartCollector(bool) error
	StopCollector() error
	Wait() (time.Time, time.Duration, error)
	AvailableBuffers() ([]byte, error)
	ReleaseBytes(int) error
	InspectAdapter() uint32
}

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
	// lan.adapter.verbosity = 3
	// lan.adapter.allocateRingBuffer(1<<24, 1<<23)
	lan.ChangeRingBuffer(32000000, 16000000)

	lan.adapter.status()
	lan.adapter.inspect()

	return lan, nil
}

//ChangeRingBuffer re-sizes the adapter's ring buffer.
func (lan *Lancero) ChangeRingBuffer(length, threshold int) error {
	return lan.adapter.allocateRingBuffer(length, threshold)
}

// Close releases all resources used by this lancero device.
func (lan *Lancero) Close() error {
	if lan.device != nil {
		lan.device.Close()
	}
	if lan.collector != nil {
		lan.collector.stop()
	}
	if lan.adapter != nil {
		lan.adapter.stop()
		lan.adapter.freeBuffer()
	}
	return nil
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

// Wait until a the threshold amount of data is available.
// Return timestamp when ready, duration since last ready, and error.
func (lan *Lancero) Wait() (time.Time, time.Duration, error) {
	return lan.adapter.wait()
}

// AvailableBuffers returns a COPY OF the ring buffer segment now ready for reading.
func (lan *Lancero) AvailableBuffers() ([]byte, error) {
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

// FindFrameBits returns q,p,n,err
// q index of word with first frame bit following non-frame index
// p index of word with next  frame bit following non-frame index
// n number of consecutive words with frame bit set, starting at q
// err is nil if q,p,n all found as expected
func FindFrameBits(b []byte) (int, int, int, error) {
	const frameMask = byte(1)
	var q, p, n int

	var state int // was frame bit seen in previous word?
	for i := 2; i < len(b); i += 4 {
		if state == 0 && !(frameMask&b[i] == 1) { // first look for lack of frame bit
			state = 1
		} else if state == 1 && frameMask&b[i] == 1 {
			// found a frame bit when before there was none
			q = i
			break
		}
	}
	for i := q; i < len(b); i += 4 { // count consecutive frame bits
		if frameMask&b[i] == 1 {
			n++
		} else {
			break
		}
	}
	state = 0
	for i := q + 4*n; i < len(b); i += 4 {
		if state == 0 && !(frameMask&b[i] == 1) { // first look for lack of frame bit
			state = 1
		} else if state == 1 && frameMask&b[i] == 1 {
			// found a frame bit when before there was none
			p = i
			return q / 4, p / 4, n, nil
		}
	}
	return q / 4, p / 4, n, fmt.Errorf("b did not contain two frame starts")
}
