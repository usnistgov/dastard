// Package lancero provides an interface to all Lancero scatter-gather DMA
// character devices, read/write from/to registers of SOPC slaves, wait for
// SOPC component interrupt events and handle the cyclic mode of SGDMA.
// Exports object Lancero for general use. Internally, that object works with
// the lower-level adapter, collector, and lanceroDevice.
//
package lancero

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
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
	AvailableBuffer() ([]byte, time.Time, error)
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
	lan.ChangeRingBuffer(256000000, 128000000)

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

// AvailableBuffer returns a COPY OF the ring buffer segment now ready for reading,
// plus the best estimate of the time stamp taken immediately after the end of the
// segment, and any error.
// Because this returns a COPY, it is encouraged to call ReleaseBytes as soon as the
// caller is sure how many bytes are to be released, even if the caller is not done
// using the copy of the data.
func (lan *Lancero) AvailableBuffer() ([]byte, time.Time, error) {
	return lan.adapter.availableBuffer()
}

// ReleaseBytes instructed the ring buffer adapter to release nBytes bytes for over-writing.
func (lan *Lancero) ReleaseBytes(nBytes int) error {
	return lan.adapter.releaseBytes(uint32(nBytes))
}

// InspectAdapter prints adapter status info and returns the status word.
func (lan *Lancero) InspectAdapter() uint32 {
	return lan.adapter.inspect()
}

// FindFrameBits (buffer, offset) studies the buffer and returns (q,p,n,err).
// buffer is the raw data; offset is the byte where frame bits are expected in
// in the least significant bit. So if the offset is b, frame bits will be expected
// in buffer[b]&1, buffer[b+4]&1, buffer[b+8]&1, etc.
//
// q index of word with first frame bit following non-frame index
// p index of word with next  frame bit following non-frame index
// word means 4 bytes: errLerrMfbkLfbkM (L=least signifiant byte, M=most significant byte)
// n number of consecutive words with frame bit set, starting at q
// err is nil if q,p,n all found as expected
func FindFrameBits(b []byte, offset int) (int, int, int, error) {
	const frameMask = byte(1)
	var q, p, n int

	var seenWordWithoutFrameBit bool
	var frameBitInPreviousWord bool
	for i := offset; i < len(b); i += 4 {
		if seenWordWithoutFrameBit {
			if frameBitInPreviousWord && !(frameMask&b[i] == 1) { // first look for lack of frame bit
				frameBitInPreviousWord = true
			} else if !frameBitInPreviousWord && frameMask&b[i] == 1 {
				// found a frame bit when before there was none
				q = i
				break
			}
		} else {
			seenWordWithoutFrameBit = !(frameMask&b[i] == 1)
		}
	}
	for i := q; i < len(b); i += 4 { // count consecutive frame bits
		if frameMask&b[i] == 1 {
			n++
		} else {
			break
		}
	}
	if n < 1 {
		spew.Dump(b)
		fmt.Println(q)
		return q / 4, p / 4, n, fmt.Errorf("n = zero, not clear how this is possible")
	}
	frameBitInPreviousWord = true
	for i := q + 4*n; i < len(b); i += 4 {
		if frameBitInPreviousWord && !(frameMask&b[i] == 1) { // first look for lack of frame bit
			frameBitInPreviousWord = false
		} else if !frameBitInPreviousWord && frameMask&b[i] == 1 {
			// found a frame bit when before there was none
			p = i
			return q / 4, p / 4, n, nil
		}
	}
	return q / 4, p / 4, n, fmt.Errorf("b did not contain two frame starts")
}

// OdDashTX creates output like od -tx4, used for debugging Lancero
func OdDashTX(b []byte, maxLines int) string {
	var lineBuffer, outBuffer bytes.Buffer
	var line, lastLine string
	encoder := hex.NewEncoder(&lineBuffer)
	repeatCount := int(0)
	outBuffer.WriteString(fmt.Sprintf("dumping %v bytes, output max lines %v\n", len(b), maxLines))
	outBuffer.WriteString("L = least significant byte, M = most significant byte\n")
	outBuffer.WriteString("framebit in fbkL\n")
	outBuffer.WriteString("fMfLeMeL fMfLeMeL fMfLeMeL fMfLeMeL fMfLeMeL fMfLeMeL fMfLeMeL fMfLeMeL \n")
	outBuffer.WriteString("-------- -------- -------- -------- -------- -------- -------- -------- \n")
	lines := 0
	for i := 0; i+3 < len(b) && lines < maxLines; i += 4 {
		encoder.Write([]byte{b[i+3]})
		encoder.Write([]byte{b[i+2]})
		encoder.Write([]byte{b[i+1]})
		encoder.Write([]byte{b[i]})
		lineBuffer.WriteString(" ")
		if (i+4)%32 == 0 {
			line = lineBuffer.String()
			lineBuffer.Reset()
			if strings.Compare(line, lastLine) == 0 {
				repeatCount++
			} else {
				if repeatCount > 0 {
					outBuffer.WriteString(fmt.Sprintf("* (%v identical lines)\n", repeatCount))
					repeatCount = 0
					lines++
				}
				outBuffer.WriteString(line)
				outBuffer.WriteString("\n")
				lines++
				lastLine = line
			}
		}
	}
	if repeatCount > 0 {
		outBuffer.WriteString(fmt.Sprintf("* (%v identical lines)", repeatCount))
	}
	return outBuffer.String()
}

// SetLogOutput calls log.SetOutput from within Lancero
func SetLogOutput(w io.Writer) {
	log.SetOutput(w)
}
