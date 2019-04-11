package ringbuffer

import (
	"os"
	"syscall"

	"github.com/fabiokung/shm"
)

type bufferDescription struct {
	writePointer uint64
	readPointer  uint64
	bufferSize   uint64
	bytesLost    uint64
}

// RingBuffer describes the shared-memory ring buffer filled by DEED.
type RingBuffer struct {
	desc     []byte
	raw      []byte
	rawName  string
	descName string
	rawFile  *os.File
	descFile *os.File
}

// NewRingBuffer creates and returns a new RingBuffer object
func NewRingBuffer(rawName, descName string) (rb *RingBuffer, err error) {
	rb = new(RingBuffer)
	rb.rawName = rawName
	rb.descName = descName
	return rb, nil
}

// Open opens the ring buffer shared memory regions and memory maps them.
func (rb *RingBuffer) Open() (err error) {
	file, err := shm.Open(rb.descName, os.O_RDONLY, 0600)
	if err != nil {
		return err
	}
	rb.descFile = file
	fd := int(rb.descFile.Fd())
	size := 4096
	rb.desc, err = syscall.Mmap(fd, 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return err
	}

	file, err = shm.Open(rb.rawName, os.O_RDONLY, 0600)
	if err != nil {
		rb.descFile.Close()
		return err
	}
	rb.rawFile = file
	fd = int(rb.rawFile.Fd())
	rb.raw, err = syscall.Mmap(fd, 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return err
	}
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
	if rb.desc != nil {
		if err = syscall.Munmap(rb.desc); err != nil {
			return
		}
		rb.desc = nil
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
