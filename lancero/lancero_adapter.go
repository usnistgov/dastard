package lancero

import "fmt"

const (
	adapterIDV  int64 = 0x00 // Adapter ID and version numbers
	adapterSTA  int64 = 0x04 // Adapter status register
	adapterCTRL int64 = 0x08 // Adapter control register
	adapterRBS  int64 = 0x0c // Adapter ring buffer size
	adapterRBWI int64 = 0x10 // Adapter ring buffer write index (byte offset)
	adapterRBRI int64 = 0x14 // Adapter ring buffer read index (byte offset)
	adapterRBAD int64 = 0x18 // Adapter ring buffer available data (bytes)
	adapterFILL int64 = 0x1c // Adapter ring buffer max fill level (bytes)
	adapterRBTH int64 = 0x20 // Adapter ring buffer interrupt threshold
	adapterALRM int64 = 0x24 // Adapter ring buffer alarm threshold

	bitsAdapterCtrlRun      uint32 = 1  // Adapter control bit: run
	bitsAdapterCtrlIEThresh uint32 = 4  // Adapter control bit: interrupt enable threshold
	bitsAdapterCtrlIEFull   uint32 = 8  // Adapter control bit: interrupt enable full
	bitsAdapterCtrlIEFlush  uint32 = 16 // Adapter control bit: flush buffer
	bitsAdapterCtrlRunFlush uint32 = bitsAdapterCtrlRun | bitsAdapterCtrlIEFlush
)

// adapter is the interface to the DMA ring buffer adapter.
// The DMA transfers take place into a fixed-length buffer of size
type adapter struct {
	device         *lanceroDevice // Object managing open file handles to the device driver.
	buffer         []byte         // Acquisition buffer: written by FPGA SGDMA, read by CPU
	length         uint32         // Length (bytes) of the acquisition buffer.
	readIndex      uint32         // Read index at which the CPU should start next read
	writeIndex     uint32         // Write index of the FPGA, last time we asked the FPGA
	thresholdLevel uint32         // Threshold level of the adapter, in bytes.
	verbosity      int            // log level
}

// Returns from the device the number of bytes available for reading.
func (a *adapter) available() (uint32, error) {
	return a.device.readRegister(adapterRBAD)
}

// Returns from the device the ID and version number of the adapter firmware.
func (a *adapter) idVersion() (uint32, error) {
	return a.device.readRegister(adapterIDV)
}

// Notify the FPGA that data have been read from the ring buffer and can now be overwritten.
// The bytesRead = how many bytes can be released.
func (a *adapter) releaseBytes(bytesRead uint32) {
	// Update the read index internally, then publish it to the firmware.
	a.readIndex = (a.readIndex + bytesRead) % a.length
	a.device.writeRegister(adapterRBRI, a.readIndex)
}

// Reads, optionally prints, and returns the Avalon ST/MM adapter status word.
func (a *adapter) status() (uint32, error) {
	status, err := a.device.readRegister(adapterSTA)
	if a.verbosity >= 3 {
		fmt.Printf("adapter status = 0x%08x\n", status)
	}
	return status, err
}

// TODO:
// available pointers
// wait

func memalign(length, alignment int) []byte {
	paddedLength := length + alignment
	unalignedBuf := make([]byte, paddedLength)
	// What to do to align here?
	return unalignedBuf
}

func (a *adapter) allocateRingBuffer(length, threshold int) error {
	switch {
	case a.device == nil:
		return fmt.Errorf("adapter.allocateRingBuffer(%d): lanceroDevice instance not provided", length)
	case length%32 != 0:
		return fmt.Errorf("adapter.allocateRingBuffer(%d): length must be a multiple of the 32-byte SGDMA bus width", length)
	case length > 1<<31:
		return fmt.Errorf("adapter.allocateRingBuffer(%d): length must be a < 2 GB", length)
	case threshold*2 > length:
		return fmt.Errorf("adapter.allocateRingBuffer(%d): threshold (%d) must be at most 50%% of length", length, threshold)
	case threshold <= 0:
		return fmt.Errorf("adapter.allocateRingBuffer(%d): threshold (%d) must be positive", length, threshold)
	}

	a.length = uint32(length)
	a.thresholdLevel = uint32(threshold)
	a.buffer = memalign(length, 4096)

	a.stop()
	if a.verbosity >= 3 {
		a.inspect()
	}
	return nil
}

// Reads and prints the state of the Avalon ST/MM adapter.
// This includes all relevant registers (more than checked by status()). Returns status.
func (a *adapter) inspect() uint32 {
	// status, writeIndex and available are continuously updated by the FPGA,
	// readIndex and threshold may be set by the application at all times.
	status, _ := a.device.readRegister(adapterSTA)
	writeIndex, _ := a.device.readRegister(adapterRBWI)
	readIndex, _ := a.device.readRegister(adapterRBRI)
	available, _ := a.device.readRegister(adapterRBAD)
	fill, _ := a.device.readRegister(adapterFILL)
	threshold, err := a.device.readRegister(adapterRBTH)
	fmt.Printf("writeIndex = 0x%08x, readIndex = 0x%08x  available = 0x%08x\n",
		writeIndex, readIndex, available)
	fmt.Printf("max filled = 0x%08x, threshold = 0x%08x\n", fill, threshold)
	fmt.Printf("status = 0x%08x", status)
	if status&bitsAdapterCtrlIEFlush != 0 {
		fmt.Printf(" ALARM")
	}
	if status&bitsAdapterCtrlIEFull != 0 {
		fmt.Printf(" FULL")
	}
	if status&bitsAdapterCtrlIEThresh != 0 {
		fmt.Printf(" THRESH")
	}
	if status&bitsAdapterCtrlRun != 0 {
		fmt.Printf(" RUN")
	}
	fmt.Printf("\n")
	if err != nil {
		fmt.Println(err)
	}
	return status
}

// Start acquisition (SGDMA and ST/MM Adapter)
func (a *adapter) start(waitSeconds int) error {
	a.stop()

	if a.verbosity >= 3 {
		fmt.Println("start(IE_FULL | IE_THRES | RUN).")
	}
	value := bitsAdapterCtrlIEFull | bitsAdapterCtrlIEThresh | bitsAdapterCtrlRun
	a.device.writeRegister(adapterCTRL, value)

	if a.verbosity >= 3 {
		fmt.Printf("start(): lancero_cyclic_start(length = %d).\n", a.length)
	}
	return a.device.cyclicStart(a.buffer, waitSeconds)
}

func (a *adapter) stop() error {
	verbose := a.verbosity >= 3
	if verbose {
		fmt.Println("adapter.stop(): setting RUN | FLUSH bits.")
	}
	// Enable flush so that adapter completes outstanding Avalon MM read requests;
	// as there is no abort mechanism in the Avalon fabric, we must complete the
	// outstanding read requests by providing dummy data which is ignored later.
	a.device.writeRegister(adapterCTRL, bitsAdapterCtrlRunFlush)
	value, err := a.device.readRegister(adapterCTRL)
	if err != nil || value != bitsAdapterCtrlRunFlush {
		return fmt.Errorf("adapter.stop() could not set state RUN|FLUSH")
	}

	if verbose {
		fmt.Printf("adapter.stop(): ADAP_CTRL = 0x%08x.\n", value)
		fmt.Printf("adapter.stop(): lancero_cyclic_stop()).\n")
	}
	if err = a.device.cyclicStop(); err != nil {
		return err
	}

	// stop adapter
	if verbose {
		fmt.Printf("adapter.stop(): clearing RUN | FLUSH bits.\n")
	}
	a.device.writeRegister(adapterCTRL, 0)
	value, err = a.device.readRegister(adapterCTRL)
	if err != nil || value != 0 {
		return fmt.Errorf("adapter.stop() could not set state 0")
	}

	// configure the Avalon ST/MM adapter
	a.readIndex = 0
	a.device.writeRegister(adapterRBS, a.length)
	a.device.writeRegister(adapterRBTH, a.thresholdLevel)
	a.device.writeRegister(adapterRBRI, a.readIndex)
	a.device.writeRegister(adapterRBAD, 0)

	// We want to get alarm when ring buffer more than 75% filled.
	a.device.writeRegister(adapterALRM, (3*a.length)/4)
	return nil
}
