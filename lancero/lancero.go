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
	"time"
)

// Lancero is the interface to a Lancero SGDMA engine on an Arria-II dev card.
type Lancero struct {
	FileUser        *os.File // talks to FPGA user SOPC/Qsys slave registers
	FileControl     *os.File // talks to FPGA Lancero internal registers
	FileEvents      *os.File // FPGA user SOPC/Qsys slaves interrupt status events
	FileSGDMA       *os.File // FPGA user SOPC/Qsys slave SGDMA read/write
	validFiles      bool
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

	if dev.FileUser, err = os.OpenFile(fname("user"), os.O_RDWR, 0666); err != nil {
		return nil, err
	}
	if dev.FileControl, err = os.OpenFile(fname("control"), os.O_RDWR, 0666); err != nil {
		dev.Close()
		return nil, err
	}
	if dev.FileEvents, err = os.OpenFile(fname("events"), os.O_RDONLY, 0666); err != nil {
		dev.Close()
		return nil, err
	}

	if dev.FileSGDMA, err = os.OpenFile(fname("sgdma"),
		os.O_RDWR|syscall.O_NONBLOCK, 0666); err != nil {
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
	return fmt.Sprintf("device %s valid status: %v", dev.FileUser.Name(), dev.validFiles)
}

// Read a Lancero register at the given offset.
func (dev *Lancero) preadUint32(file *os.File, offset int64) (uint32, error) {
	result := make([]byte, 4)
	n, err := file.ReadAt(result, offset)
	if n < 4 || err != nil {
		return 0, fmt.Errorf("Could not read file %s offset: 0x%x", file.Name(), offset)
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

// Return the value of the ID+Version number register.
func (dev *Lancero) idVersion() (uint32, error) {
	return dev.readRegister(colRegisterIDV)
}

// Read /dev/lancero_events* to know when data might be ready. This
// function will block until the threshold interrupt event occurs:
// at least threshold bytes of data are now available.
func (dev *Lancero) readEvents() (uint32, error) {
	result := make([]byte, 4)
	n, err := dev.FileEvents.Read(result)
	if n < 4 || err != nil {
		return 0, fmt.Errorf("Lancero: Could not readEvents")
	}
	return binary.LittleEndian.Uint32(result[0:]), nil
}

// Write a uint32 to a Lancero register at the given offset.
func (dev *Lancero) pwriteUint32(file *os.File, offset int64, value uint32) error {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, value)
	n, err := file.WriteAt(bytes, offset)
	if n < 4 || err != nil {
		return fmt.Errorf("Could not write file %s offset: 0x%x, value: 0x%x", file.Name(), offset, value)
	}
	return nil
}

// Write a uint32 to /dev/lancero_user* register at the given offset.
func (dev *Lancero) writeRegister(offset int64, value uint32) error {
	return dev.pwriteUint32(dev.FileUser, offset, value)
}

// Write a uint32 to /dev/lancero_user* register at the given offset, then ensure write
// is flushed (by reading the ID+version register).
func (dev *Lancero) writeRegisterFlush(offset int64, value uint32) error {
	if err := dev.writeRegister(offset, value); err != nil {
		return err
	}
	// Reading any register will flush the previous write. Choose ID+Version register.
	_, err := dev.readRegister(colRegisterIDV)
	return err
}

// Write a uint32 to /dev/lancero_user* register at the given offset.
func (dev *Lancero) writeControl(offset int64, value uint32) error {
	return dev.pwriteUint32(dev.FileControl, offset, value)
}

// Start a cyclic SGDMA. Wait for the engine to be running as the
// data sources do not support back pressure; We make sure the SGDMA engines
// are running and accepting data before enabling the data inputs.
func (dev *Lancero) cyclicStart(buffer []byte, waitSeconds int) error {
	verbose := dev.verbosity >= 3
	n, err := dev.FileSGDMA.Read(buffer)
	if err != nil {
		return err
	}
	if n < len(buffer) {
		return fmt.Errorf("cyclicStart(): could not start SGDMA")
	}
	value, err := dev.readControl(0x200)
	if err != nil {
		return err
	}
	if verbose {
		fmt.Printf("Write engine ID = 0x%08x\n", value)
		fmt.Println("Polling write engine control, waiting for RUN...")
	}

	// Wait for something, with a timeout.
	var RUN uint32 = 1
	var prevvalue uint32 = 0xdeadbeef
	abortTimeout := time.After(time.Duration(waitSeconds) * time.Second)
	for {
		value, err = dev.readControl(0x208)
		if err != nil {
			return err
		}
		if verbose && value != prevvalue {
			if value&RUN == 1 {
				fmt.Printf("Write engine is RUNNING. (control = 0x%08x)\n", value)
			} else {
				fmt.Printf("Write engine is NOT RUNNING. (control = 0x%08x)\n", value)
			}
			prevvalue = value
		}
		// The engine is now running. Save its control register to know how to stop it.
		if value&RUN == 1 {
			dev.engineStopValue = value &^ 1
			break
		}
		tryAgain := time.After(40 * time.Millisecond)
		select {
		case <-abortTimeout:
			return fmt.Errorf("cyclicStart(): failed to reach RUN mode after %d sec", waitSeconds)
		case <-tryAgain:
			continue
		}
	}

	if verbose {
		fmt.Printf("Will stop write engine by writing 0x%08x to control later.\n", dev.engineStopValue)
		fmt.Println("Now polling write engine, waiting for BUSY status...")
	}
	var BUSY uint32 = 1
	prevvalue = 0xdeadbeef
	for {
		value, err = dev.readControl(0x204)
		if err != nil {
			return err
		}
		if verbose && value != prevvalue {
			if value&BUSY == 1 {
				fmt.Printf("Write engine is BUSY. (status = 0x%08x)\n", value)
			} else {
				fmt.Printf("Write engine is NOT BUSY. (status = 0x%08x)\n", value)
			}
			prevvalue = value
		}
		if value&BUSY == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if verbose {
		fmt.Printf("cyclicStart(): Engine status = 0x%08x.\n", value)
	}
	return nil
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
