// Package lancero provides an interface to all Lancero SGDMA
// character devices, read/write from/to registers of SOPC slaves, wait for
// SOPC component interrupt events and handle the cyclic mode of SGDMA.
//
// It uses the IEEE POSIX interface of the four Lancero character devices:
// * lancero_user -- for SOPC slaves on the target user bus
// * lancero_control -- for Lancero SGDMA internal registers
// * lancero_events -- for the interrupt event character device,
//                     which provides a sychronous blocking read().
// * lancero_sgdma -- for the SGDMA character device, which provides
//                    asynchronous and sychronous blocking read/write().
package lancero

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
)

// Lancero is the interface to a Lancero SGDMA engine on an Arria-II dev card.
type Lancero struct {
	FileUser        *os.File // talks to FPGA user SOPC/Qsys slave registers
	FileControl     *os.File // talks to FPGA Lancero internal registers
	FileEvents      *os.File // FPGA user SOPC/Qsys slaves interrupt status events
	FileSGDMA       *os.File // FPGA user SOPC/Qsys slave SGDMA read/write
	validFiles      bool
	userFilename    string
	verbosity       int
	engineStopValue uint32
}

// Open all necessary /dev/lancero_* files for a new Lancero object, if
// possible.
func Open(devnum int) (dev *Lancero, err error) {
	dev = new(Lancero)
	dev.verbosity = 3

	fname := func(name string) string {
		return fmt.Sprintf("/dev/lancero_%s%d", name, devnum)
	}
	dev.userFilename = fname("user")
	dev.FileUser, err = os.OpenFile(fname("user"), os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}
	dev.FileControl, err = os.OpenFile(fname("control"), os.O_RDWR, 0666)
	if err != nil {
		dev.Close()
		return nil, err
	}
	dev.FileEvents, err = os.OpenFile(fname("events"), os.O_RDONLY, 0666)
	if err != nil {
		dev.Close()
		return nil, err
	}

	dev.FileSGDMA, err = os.OpenFile(fname("sgdma"), os.O_RDWR|syscall.O_NONBLOCK, 0666)
	if err != nil {
		dev.Close()
		return nil, err
	}

	dev.validFiles = true
	return dev, err
}

// Close any open file descriptors in the /dev/lancero_* set.
func (dev *Lancero) Close() (err error) {
	files := [...]*os.File{dev.FileUser, dev.FileControl, dev.FileEvents, dev.FileSGDMA}
	for _, file := range files {
		if file != nil {
			file.Close()
		}
	}
	dev.validFiles = false
	return err
}

func (dev *Lancero) String() string {
	return fmt.Sprintf("device %s valid: %v", dev.userFilename, dev.validFiles)
}

// Read a Lancero register at the given offset.
func (dev *Lancero) preadUint32(file *os.File, offset int64) (uint32, error) {
	result := make([]byte, 4)
	n, err := file.ReadAt(result, offset)
	if n < 4 || err != nil {
		return 0, fmt.Errorf("Could not read %s", dev.userFilename)
	}
	return binary.LittleEndian.Uint32(result[0:]), nil
}

// Read /dev/lancero_user* register at the given offset.
func (dev *Lancero) readRegister(offset int64) (uint32, error) {
	return dev.preadUint32(dev.FileUser, offset)
}

// Read /dev/lancero_user* register at the given offset.
func (dev *Lancero) readControl(offset int64) (uint32, error) {
	return dev.preadUint32(dev.FileControl, offset)
}

// Read /dev/lancero_events* to know when data might be ready. This
// function will block until the threshold interrupt event occurs:
// at least threshold bytes of data are now available.
func (dev *Lancero) readEvents() (uint32, error) {
	result := make([]byte, 4)
	n, err := dev.FileEvents.Read(result)
	if n < 4 || err != nil {
		return 0, fmt.Errorf("Could not readEvents")
	}
	return binary.LittleEndian.Uint32(result[0:]), nil
}

// Write a uint32 to a Lancero register at the given offset.
func (dev *Lancero) pwriteUint32(file *os.File, offset int64, value uint32) error {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, value)
	n, err := file.WriteAt(bytes, offset)
	if n < 4 || err != nil {
		return fmt.Errorf("Could not write %s", dev.userFilename)
	}
	return nil
}

// Write a uint32 to /dev/lancero_user* register at the given offset.
func (dev *Lancero) writeRegister(offset int64, value uint32) error {
	return dev.pwriteUint32(dev.FileUser, offset, value)
}

// Write a uint32 to /dev/lancero_user* register at the given offset.
func (dev *Lancero) writeControl(offset int64, value uint32) error {
	return dev.pwriteUint32(dev.FileControl, offset, value)
}

// Stop the cyclic SGDMA.
func (dev *Lancero) cyclicStop() error {
	verbose := dev.verbosity >= 3
	var BUSYFLAG uint32 = 1

	// Read engine status
	value, err := dev.readControl(0x204)
	if err != nil {
		return err
	}
	if verbose {
		fmt.Printf("cyclicStop(): Engine status = 0x%x\n", value)
	}
	if value&BUSYFLAG == 0 {
		if verbose {
			fmt.Println("cyclicStop(): Engine is not BUSY, so no need to stop it.")
		}
		return nil
	}

	// Stop the engine
	if err = dev.writeControl(0x208, dev.engineStopValue); err != nil {
		return err
	}
	if verbose {
		fmt.Printf("cyclicStop(): Writing 0x%x to engine control 0x208.\n", value)
	}

	// Read engine status again, repeatedly until it is not BUSY.
	var laststatus uint32 = 0xdeadbeef
	for {
		value, err = dev.readControl(0x204)
		if err != nil {
			return err
		}
		if verbose && laststatus != value {
			fmt.Printf("cyclicStop(): Engine status = 0x%x\n", value)
			laststatus = value
		}
		if value&BUSYFLAG == 0 {
			break
		}
		if verbose {
			fmt.Printf("cyclicStop(): Now polling write engine, waiting for BUSY to clear\n")
		}
	}
	if verbose {
		fmt.Printf("cyclicStop(): Write engine no longer BUSY.\n")
		fmt.Printf("cyclicStop(): Engine status = 0x%08x.\n", value)
	}
	return nil
}
