package lancero

import (
    "encoding/binary"
    "os"
    "syscall"
    "fmt"
)

type Lancero struct {
    fdUser     *os.File
    fdControl  *os.File
    fdEvents   *os.File
    fdSGDMA    *os.File
    validFDs   bool
    userFilename string
    verbosity  int
}

// Create a new Lancero object. Open all necessary /dev/lancero_* files, if
// possible.
func Open(devnum int) (dev *Lancero, err error) {
    dev = new(Lancero)
    dev.verbosity = 3

    fname := func(name string) string {
        return fmt.Sprintf("/dev/lancero_%s%d", name, devnum)
    }
    dev.userFilename = fname("user")
    dev.fdUser, err = os.OpenFile(fname("user"), os.O_RDWR, 0666)
    if err != nil {
        return nil, err
    }
    dev.fdControl, err = os.OpenFile(fname("control"), os.O_RDWR, 0666)
    if err != nil {
        dev.Close()
        return nil, err
    }
    dev.fdEvents, err = os.OpenFile(fname("events"), os.O_RDONLY, 0666)
    if err != nil {
        dev.Close()
        return nil, err
    }

    dev.fdSGDMA, err = os.OpenFile(fname("sgdma"), os.O_RDWR|syscall.O_NONBLOCK, 0666)
    if err != nil {
        dev.Close()
        return nil, err
    }

    dev.validFDs = true
    return dev, err
}

// Close any open file descriptors in the /dev/lancero_* set.
func (dev *Lancero) Close() (err error) {
    files := [...]*os.File {dev.fdUser, dev.fdControl, dev.fdEvents, dev.fdSGDMA}
    for _, file := range files {
        if file != nil {
            file.Close()
        }
    }
    dev.validFDs = false
    return err
}

func (dev *Lancero) String() string {
    return fmt.Sprintf("device %s valid: %v", dev.userFilename, dev.validFDs)
}

// Read /dev/lancero_user? register at the given offset.
func (dev *Lancero) readRegister(offset int64) (uint32, error) {
    result := make([]byte, 4)
    n, err := dev.fdUser.ReadAt(result, offset)
    if n < 4 || err != nil {
        return 0, fmt.Errorf("Could not read %s", dev.userFilename)
    }
    return binary.LittleEndian.Uint32(result[0:]), nil
}
