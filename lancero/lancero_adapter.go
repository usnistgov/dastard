package lancero

//
// #include <stdlib.h>
// #include <stdio.h>
// #include <string.h>
//
// char* posixMemAlign(size_t alignment, size_t size) {
//     void *vout;
//    ssize_t _ = posix_memalign(&vout, alignment, size);
//     return (char *)vout;
// }
import "C"

import (
	"fmt"
	"log"
	"time"
	"unsafe"
)

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

	// HardMaxBufSize Longest allowed adapter buffer
	HardMaxBufSize uint32 = 40 * (1 << 20)
	// For latest value, look in driver lancero/Lancero-RELEASE/C/driver/lancero-base.c
	// for the line that reads #define LANCERO_TRANSFER_MAX_BYTES (40 * (1<<20))
)

// adapter is the interface to the DMA ring buffer adapter.
// The DMA transfers take place into a fixed-length buffer of size
type adapter struct {
	device         *lanceroDevice // Object managing open file handles to the device driver.
	buffer         *C.char        // Acquisition buffer: written by FPGA SGDMA, read by CPU
	length         uint32         // Length (bytes) of the acquisition buffer.
	readIndex      uint32         // Read index at which the CPU should start next read
	writeIndex     uint32         // Write index of the FPGA, last time we asked the FPGA
	thresholdLevel uint32         // Threshold level of the adapter, in bytes.
	verbosity      int            // log level
	lastThresh     time.Time      // When the previous threshold wait ended
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
// The bytesRead gives how many bytes should now be released.
func (a *adapter) releaseBytes(bytesRead uint32) error {
	if a.verbosity >= 3 {
		log.Printf("adapter.releaseBytes(%d): moving read index from 0x%08x to ", bytesRead, a.readIndex)
	}
	// Update the read index internally, then publish it to the firmware.
	a.readIndex = (a.readIndex + bytesRead) % a.length
	if a.verbosity >= 3 {
		log.Printf("0x%08x\n", a.readIndex)
	}
	if err := a.device.writeRegister(adapterRBRI, a.readIndex); err != nil {
		return err
	}
	status, err := a.status()
	if err != nil {
		return err
	}
	if status&bitsAdapterCtrlIEFull != 0 {
		return fmt.Errorf("FAILURE: Ring buffer overflow; verify ring buffer size and I/O contention in the system")
	}
	return nil
}

func (a *adapter) freeBuffer() {
	if a.buffer != nil {
		C.free(unsafe.Pointer(a.buffer))
	}
	a.buffer = nil
}

// Reads, optionally prints, and returns the Avalon ST/MM adapter status word.
func (a *adapter) status() (uint32, error) {
	status, err := a.device.readRegister(adapterSTA)
	if a.verbosity >= 3 {
		log.Printf("adapter status = 0x%08x\n", status)
	}
	return status, err
}

func (a *adapter) allocateRingBuffer(length, threshold int) error {
	switch {
	case a.device == nil:
		return fmt.Errorf("adapter.allocateRingBuffer(%d): lanceroDevice instance not provided", length)
	case length%32 != 0:
		return fmt.Errorf("adapter.allocateRingBuffer(%d): length must be a multiple of the 32-byte SGDMA bus width", length)
	case length > 1<<31:
		return fmt.Errorf("adapter.allocateRingBuffer(%d): length must be <= 2 GB", length)
	case length > int(HardMaxBufSize):
		return fmt.Errorf("adapter.allocateRingBuffer(%d): length must not exceed hard max of %d", length, HardMaxBufSize)
	case threshold*2 > length:
		return fmt.Errorf("adapter.allocateRingBuffer(%d): threshold (%d) must be at most 50%% of length", length, threshold)
	case threshold <= 0:
		return fmt.Errorf("adapter.allocateRingBuffer(%d): threshold (%d) must be positive", length, threshold)
	}

	a.length = uint32(length)
	a.thresholdLevel = uint32(threshold)
	const PAGEALIGN C.size_t = 4096
	a.freeBuffer()
	a.buffer = C.posixMemAlign(PAGEALIGN, C.size_t(length))

	a.stop()
	if a.verbosity >= 3 {
		a.inspect()
	}
	return nil
}

// Find total amount of available data and return a buffer (byte slice).
// Returns the data buffer and any possible error.
func (a *adapter) availableBuffers() (buffer []byte, err error) {
	// Ask the hardware for the current write pointer and the bytes available.
	// Note that "write pointer" is where the DRIVER is about to write to.
	a.writeIndex, err = a.device.readRegister(adapterRBWI)
	if err != nil {
		return
	}
	if a.writeIndex == a.readIndex {
		return
	}

	// Handle the easier, single-buffer case first.
	if a.writeIndex >= a.readIndex {
		// There's only one continuous region in the buffer, not crossing the ring boundary
		length := C.int(a.writeIndex - a.readIndex) // can equal 0, b/f collector gets going
		if length > 0 {
			buffer = C.GoBytes(unsafe.Pointer(uintptr(unsafe.Pointer(a.buffer))+uintptr(a.readIndex)), length)
		}
		return
	}

	// The available data cross the ring boundary, so return separate buffers joined together.
	// buffers = a.buffer[a.readIndex:] + a.buffer[0:a.writeIndex])
	length1 := C.int(a.length - a.readIndex)
	length2 := C.int(a.writeIndex)
	buffer = make([]byte, length1+length2)
	C.memcpy(unsafe.Pointer(&buffer[0]), unsafe.Pointer(uintptr(unsafe.Pointer(a.buffer))+uintptr(a.readIndex)), C.size_t(length1))
	// buffer = C.GoBytes(unsafe.Pointer(uintptr(unsafe.Pointer(a.buffer))+uintptr(a.readIndex)), length1+length2)
	if length2 > 0 {
		// fmt.Printf("\tjoining buffers of length %7d+%7d\n", length1, length2)
		// buffer = append(buffer, C.GoBytes(unsafe.Pointer(a.buffer), length2)...)
		C.memcpy(unsafe.Pointer(&buffer[length1]), unsafe.Pointer(a.buffer), C.size_t(length2))
	}
	return
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
	log.Printf("writeIndex = 0x%08x, readIndex = 0x%08x  available = 0x%08x\n",
		writeIndex, readIndex, available)
	log.Printf("max filled = 0x%08x, threshold = 0x%08x\n", fill, threshold)
	log.Printf("inspect status = 0x%08x (these bits signify: ", status)
	if status&bitsAdapterCtrlIEFlush != 0 {
		log.Printf(" ALARM")
	}
	if status&bitsAdapterCtrlIEFull != 0 {
		log.Printf(" FULL")
	}
	if status&bitsAdapterCtrlIEThresh != 0 {
		log.Printf(" THRESH")
	}
	if status&bitsAdapterCtrlRun != 0 {
		log.Printf(" RUN")
	}
	if status == 0 {
		log.Printf(" not running")
	}
	log.Printf(")\n")
	if err != nil {
		log.Println(err)
	}
	return status
}

// Start acquisition (SGDMA and ST/MM Adapter)
func (a *adapter) start(waitSeconds int) error {
	a.stop()

	if a.verbosity >= 3 {
		log.Println("start(IE_FULL | IE_THRES | RUN).")
	}
	value := bitsAdapterCtrlIEFull | bitsAdapterCtrlIEThresh | bitsAdapterCtrlRun
	a.device.writeRegister(adapterCTRL, value)

	if a.verbosity >= 3 {
		log.Printf("start(): lancero_cyclic_start(length = %d).\n", a.length)
	}
	a.lastThresh = time.Now()
	return a.device.cyclicStart(a.buffer, a.length, waitSeconds)
}

func (a *adapter) stop() error {
	verbose := a.verbosity >= 3
	if verbose {
		log.Println("adapter.stop(): setting RUN | FLUSH bits.")
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
		log.Printf("adapter.stop(): ADAP_CTRL = 0x%08x.\n", value)
		log.Printf("adapter.stop(): lancero_cyclic_stop()).\n")
	}
	if err = a.device.cyclicStop(); err != nil {
		return err
	}

	// stop adapter
	if verbose {
		log.Printf("adapter.stop(): clearing RUN | FLUSH bits.\n")
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

// Wait until a the threshold amount of data is available.
// Return timestamp when ready, duration since last ready, and error.
func (a *adapter) wait() (time.Time, time.Duration, error) {
	now := time.Now()
	dataAvailable, err := a.available()
	if err != nil {
		sinceLast := now.Sub(a.lastThresh)
		a.lastThresh = now
		return now, sinceLast, err
	}
	if dataAvailable >= a.thresholdLevel {
		sinceLast := now.Sub(a.lastThresh)
		a.lastThresh = now
		return now, sinceLast, nil
	}
	if a.verbosity >= 3 {
		log.Println("adapter.wait(): Waiting for threshold event.")
	}

	// block until an interrupt event occurs.
	start := time.Now()
	_, err = a.device.readEvents()
	now = time.Now()
	sinceLast := now.Sub(a.lastThresh)
	a.lastThresh = now
	if a.verbosity >= 3 {
		waitTime := now.Sub(start)
		log.Printf("adapter.wait(): Waited for %v (%v since last thresh).\n",
			waitTime, sinceLast)
	}
	return now, sinceLast, err
}
