package lancero

import (
	"bytes"
	"fmt"
	"time"
)

// NoHardware is a drop in replacement for Lancero (implements Lanceroer)
// that requires no hardware. Use it for testing the interface.
type NoHardware struct {
	ncols               int
	nrows               int
	linePeriod          int
	nsPerLinePeriod     int
	isOpen              bool
	isStarted           bool
	collectorStarted    bool
	bytesReleased       int
	lastReadTime        time.Time
	minTimeBetweenReads time.Duration
	rowCount            int
	idNum               int
}

// String implements Stringer for NoHardware, aka controls how Println output looks
func (lan NoHardware) String() string {
	return fmt.Sprintf("lancero.NoHardware: idNum %v", lan.idNum)
}

var idNumCounter int

// NewNoHardware generates and returns a new Lancero object in test mode,
// meaning it emulate a lancero without any hardware. Here linePeriod is
// the equivalent of LSYNC: how many 8 ns clocks per row of readout.
func NewNoHardware(ncols int, nrows int, linePeriod int) (*NoHardware, error) {
	const nsPerLinePeriod = 8
	lan := NoHardware{ncols: ncols, nrows: nrows, linePeriod: linePeriod,
		nsPerLinePeriod: nsPerLinePeriod, isOpen: true, lastReadTime: time.Now(),
		minTimeBetweenReads: 10 * time.Millisecond, idNum: idNumCounter}
	idNumCounter++
	return &lan, nil
}

// ChangeRingBuffer doesnt error
func (lan *NoHardware) ChangeRingBuffer(length, threshold int) error {
	return nil
}

// Close errors if already closed
func (lan *NoHardware) Close() error {
	if !lan.isOpen {
		return fmt.Errorf("NoHardware.Close: already closed: id %v", lan.idNum)
	}
	lan.isOpen = false
	return nil
}

// StartAdapter errors if already started
func (lan *NoHardware) StartAdapter(waitSeconds, verbosity int) error {
	if lan.isStarted {
		return fmt.Errorf("NoHardware.StartAdapter: already started: id %v", lan.idNum)
	}
	lan.isStarted = true
	return nil
}

// StopAdapter errors if not started
func (lan *NoHardware) StopAdapter() error {
	if !lan.isStarted {
		return fmt.Errorf("NoHardware.StopAdapter: not started: id %v", lan.idNum)
	}
	lan.isStarted = false
	return nil
}

// CollectorConfigure returns nil
func (lan *NoHardware) CollectorConfigure(linePeriod, dataDelay int, channelMask uint32,
	frameLength int) error {
	return nil
}

// StartCollector errors if Collector Already Started
func (lan *NoHardware) StartCollector(simulate bool) error {
	if lan.collectorStarted {
		return fmt.Errorf("NoHardware.StartCollector: collector started already: id %v", lan.idNum)
	}
	lan.collectorStarted = true
	return nil
}

// StopCollector errors if Collector not started
func (lan *NoHardware) StopCollector() error {
	if !lan.collectorStarted {
		return fmt.Errorf("NoHardware.StopCollector: collector stopped already: id %v", lan.idNum)
	}
	lan.collectorStarted = false
	return nil
}

// Wait sleeps until lastReadTime + minTimeBetweenReads
func (lan *NoHardware) Wait() (time.Time, time.Duration, error) {
	sleepDuration := time.Until(lan.lastReadTime.Add(lan.minTimeBetweenReads))
	time.Sleep(sleepDuration)
	now := time.Now()
	return now, now.Sub(lan.lastReadTime), nil
}

// AvailableBuffer returns some simulated data
// size matches what you should get in 1 millisecond
// all entries other than frame bits are zeros
func (lan *NoHardware) AvailableBuffer() ([]byte, time.Time, error) {
	var buf bytes.Buffer
	now := time.Now()
	if !lan.isStarted {
		return buf.Bytes(), now, fmt.Errorf("err in NoHardware.AvailableBuffers: not started: id %v", lan.idNum)
	}
	if !lan.collectorStarted {
		return buf.Bytes(), now, fmt.Errorf("err in NoHardware.AvailableBuffers: collector not started: id %v", lan.idNum)
	}
	if !lan.isOpen {
		return buf.Bytes(), now, fmt.Errorf("err in NoHardware.AvailableBuffers: not open: id %v", lan.idNum)
	}
	sinceLastRead := now.Sub(lan.lastReadTime)
	lan.lastReadTime = now
	frameDurationNanoseconds := lan.linePeriod * lan.nsPerLinePeriod * lan.nrows
	frames := int(sinceLastRead.Nanoseconds()) / frameDurationNanoseconds
	// log.Printf("id %v read at %v\n", lan.idNum, time.Now())
	if sinceLastRead > 100*lan.minTimeBetweenReads {
		return buf.Bytes(), now, fmt.Errorf("reads were %v apart, want < %v", sinceLastRead, 50*lan.minTimeBetweenReads)
	}

	for i := 0; i < frames; i++ { // i counts frames
		for row := 0; row < lan.nrows; row++ {
			for col := 0; col < lan.ncols; col++ {
				v := byte(uint8(lan.rowCount))
				lan.rowCount++
				if row == 0 {
					// first row has frame bit
					buf.Write([]byte{0x00, v, 0x01, v})
				} else {
					// all data is zeros
					buf.Write([]byte{0x00, v, 0x00, v})
				}
			}
		}

	}
	return buf.Bytes(), now, nil
}

// ReleaseBytes increments bytesReleased
func (lan *NoHardware) ReleaseBytes(nBytes int) error {
	lan.bytesReleased += nBytes
	return nil
}

// InspectAdapter does nothing and returns 0
func (lan *NoHardware) InspectAdapter() uint32 {
	return uint32(0)
}
