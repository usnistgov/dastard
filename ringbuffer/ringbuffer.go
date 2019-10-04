package ringbuffer

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/fabiokung/shm"
)

type bufferDescription struct {
	magic        uint32
	version      uint32
	writePointer uint64
	readPointer  uint64
	bufferSize   uint64
	packetSize   int64
}

// RingBuffer describes the shared-memory ring buffer filled by DEED.
type RingBuffer struct {
	descSlice []byte
	desc      *bufferDescription
	size      uint64
	raw       []byte
	rawName   string
	descName  string
	rawFile   *os.File
	descFile  *os.File
	writeable bool // Is this a writeable buffer? False, except for testing
}

// NewRingBuffer creates and returns a new RingBuffer object
func NewRingBuffer(rawName, descName string) (rb *RingBuffer, err error) {
	rb = new(RingBuffer)
	rb.rawName = rawName
	rb.descName = descName
	return rb, nil
}

// Create makes a writeable buffer. Though exported, it's only for testing.
func (rb *RingBuffer) Create(bufsize int) (err error) {
	rb.writeable = true
	file, err := shm.Open(rb.descName, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		return err
	}
	rb.descFile = file
	fd := int(rb.descFile.Fd())
	descsize := 4096
	if err = syscall.Ftruncate(fd, int64(descsize)); err != nil {
		return err
	}
	rb.descSlice, err = syscall.Mmap(fd, 0, descsize, syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return err
	}
	rb.desc = (*bufferDescription)(unsafe.Pointer(&rb.descSlice[0]))
	rb.desc.magic = 0xb0ffde5c
	rb.desc.version = 0x01020003
	rb.desc.writePointer = 0
	rb.desc.readPointer = 0
	rb.desc.bufferSize = uint64(bufsize)
	rb.desc.packetSize = 8192
	rb.size = rb.desc.bufferSize

	file, err = shm.Open(rb.rawName, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		return err
	}
	rb.rawFile = file
	fd = int(rb.rawFile.Fd())
	if err = syscall.Ftruncate(fd, int64(bufsize)); err != nil {
		return err
	}
	rb.raw, err = syscall.Mmap(fd, 0, bufsize, syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return err
	}
	return nil
}

// Write adds data bytes to the buffer. Though exported, it's only for testing.
func (rb *RingBuffer) Write(data []byte) (written int, err error) {
	w := rb.desc.writePointer
	r := rb.desc.readPointer
	cap := rb.desc.bufferSize
	available := int(cap - (w - r + 1))
	if len(data) > available {
		written = available
	} else {
		written = len(data)
	}
	wAfter := w + uint64(written)
	dataWraps := wAfter/cap > w/cap
	rawbegin := w % cap
	rawend := wAfter % cap
	if dataWraps {
		rawend = cap
	}
	firstblocksize := int(rawend - rawbegin)
	copy(rb.raw[rawbegin:rawend], data[0:firstblocksize])
	if dataWraps {
		nextblocksize := written - firstblocksize
		copy(rb.raw[0:nextblocksize], data[firstblocksize:])
	}
	rb.desc.writePointer = wAfter
	return

}

// bytesWriteable tells how many bytes can be written. Actual answer may be larger,
// if reading is underway.
func (rb *RingBuffer) bytesWriteable() int {
	w := rb.desc.writePointer
	r := rb.desc.readPointer
	cap := rb.desc.bufferSize
	return int(cap - (w - r))
}

// Unlink removes a writeable buffer's shared memory regions.
func (rb *RingBuffer) Unlink() (err error) {
	if err = shm.Unlink(rb.rawName); err != nil {
		return err
	}
	if err = shm.Unlink(rb.descName); err != nil {
		return err
	}
	return nil
}

// Open opens the ring buffer shared memory regions and memory maps them.
func (rb *RingBuffer) Open() (err error) {
	file, err := shm.Open(rb.descName, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	rb.descFile = file
	fd := int(rb.descFile.Fd())
	size := 4096
	rb.descSlice, err = syscall.Mmap(fd, 0, size, syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		rb.Close()
		return err
	}
	rb.desc = (*bufferDescription)(unsafe.Pointer(&rb.descSlice[0]))
	if rb.desc == nil {
		rb.Close()
		return fmt.Errorf("RingBuffer.desc pointer = nil")
	}
	rb.size = rb.desc.bufferSize

	file, err = shm.Open(rb.rawName, os.O_RDONLY, 0600)
	if err != nil {
		rb.Close()
		return err
	}
	rb.rawFile = file
	fd = int(rb.rawFile.Fd())
	rb.raw, err = syscall.Mmap(fd, 0, int(rb.desc.bufferSize), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		rb.Close()
		return err
	}
	if rb.raw == nil {
		rb.Close()
		return fmt.Errorf("RingBuffer.raw pointer = nil")
	}
	// fmt.Printf("Ring.Open succeeds. desc = %p\n", rb.desc)
	return nil
}

// Close closes the ring buffer by munmap and closing the shared memory regions.
func (rb *RingBuffer) Close() (err error) {
	if rb.raw != nil {
		if err = syscall.Munmap(rb.raw); err != nil {
			return
		}
		rb.raw = nil
	}
	rb.desc = nil
	if rb.descSlice != nil {
		if err = syscall.Munmap(rb.descSlice); err != nil {
			return
		}
		rb.descSlice = nil
	}
	if rb.rawFile != nil {
		if err = rb.rawFile.Close(); err != nil {
			return
		}
		rb.rawFile = nil
	}
	if rb.descFile != nil {
		if err = rb.descFile.Close(); err != nil {
			return
		}
		rb.descFile = nil
	}
	return nil
}

// Read reads a byte slice from the buffer of size no larger than size.
// It is not an error to request more bytes than the buffer could hold.
func (rb *RingBuffer) Read(size int) (data []byte, err error) {
	if rb.desc == nil {
		return []byte{}, fmt.Errorf("Could not RingBuffer.Read with nil ring description pointer.")
	}
	w := rb.desc.writePointer
	r := rb.desc.readPointer
	cap := rb.desc.bufferSize
	available := int(w - r)
	var bytesRead int
	if size > available {
		bytesRead = available
	} else {
		bytesRead = size
	}
	if bytesRead <= 0 {
		return []byte{}, nil
	}
	rAfter := r + uint64(bytesRead)
	dataWraps := rAfter/cap > r/cap
	rawbegin := r % cap
	rawend := rAfter % cap
	if dataWraps {
		rawend = cap
	}
	data = rb.raw[rawbegin:rawend]
	if dataWraps {
		nextblocksize := bytesRead - len(data)
		data = append(data, rb.raw[0:nextblocksize]...)
	}
	rb.desc.readPointer = rAfter
	return
}

// ReadMultipleOf will return a multiple (possibly 0) of chunksize bytes.
func (rb *RingBuffer) ReadMultipleOf(chunksize int) (data []byte, err error) {
	if uint64(chunksize) >= rb.size {
		return nil, fmt.Errorf("Cannot call ReadMultipleOf(%d), want < %d", chunksize,
			rb.size)
	}
	available := rb.BytesReadable()
	nchunks := available / chunksize
	return rb.Read(chunksize * nchunks)
}

// ReadAll reads all the bytes available in the buffer.
func (rb *RingBuffer) ReadAll() (data []byte, err error) {
	if uintptr(unsafe.Pointer(rb.desc)) < 0x1000 {
		fmt.Printf("RingBuffer.ReadAll with rb.desc=%p\n", rb.desc)
	}
	return rb.Read(int(rb.size))
}

// ReadMinimum blocks until it can read at least minimum bytes.
// It does this by sleeping in 1 ms units
func (rb *RingBuffer) ReadMinimum(minimum int) (data []byte, err error) {
	if uint64(minimum) >= rb.size {
		return nil, fmt.Errorf("Cannot call ReadMinimum(%d), want < %d", minimum,
			rb.desc.bufferSize)
	}
	for rb.BytesReadable() < minimum {
		time.Sleep(time.Millisecond)
	}
	return rb.Read(int(rb.size))
}

// BytesReadable tells how many bytes can be read. Actual answer may be larger,
// if writing is underway.
func (rb *RingBuffer) BytesReadable() int {
	w := rb.desc.writePointer
	r := rb.desc.readPointer
	cap := rb.desc.bufferSize
	if w-r >= cap {
		return int(cap - 1)
	}
	return int(w - r)
}

// DiscardAll removes all readable bytes and empties the buffer.
func (rb *RingBuffer) DiscardAll() (err error) {
	rb.desc.readPointer = rb.desc.writePointer
	return nil
}

// PacketSize returns the packetSize held in the buffer description
func (rb *RingBuffer) PacketSize() int64 {
	if rb.desc == nil {
		return -1
	}
	return rb.desc.packetSize
}
