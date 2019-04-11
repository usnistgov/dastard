package ringbuffer

import (
	"testing"

	"github.com/fabiokung/shm"
)

func TestBufferOpenClose(t *testing.T) {
	goodname1 := "will_exist_buffer"
	goodname2 := "will_exist_description"
	badname1 := "does_not_exist"
	badname2 := "does_not_exist_either"

	// In case these memory regions exist from earlier tests, remove them.
	names := []string{goodname1, goodname2, badname1, badname2}
	for _, name := range names {
		shm.Unlink(name)
	}

	// Now test the writeable buffer, which we need for testing.
	writebuf, err := NewRingBuffer(goodname1, goodname2)
	if err != nil {
		t.Error("Failed NewRingBuffer")
	}
	defer writebuf.unlink()
	if err = writebuf.create(8192); err != nil {
		t.Error("Failed RingBuffer.create", err)
	}

	// Now 2 buffers that should not be Openable
	r, err := NewRingBuffer(badname1, badname2)
	if err != nil {
		t.Error("Failed NewRingBuffer", err)
	}
	if err = r.Open(); err == nil {
		t.Errorf("NewRingBuffer(%s, %s) succeeds, should fail", badname1, badname2)
	}
	r, err = NewRingBuffer(badname1, goodname2)
	if err != nil {
		t.Error("Failed NewRingBuffer", err)
	}
	if err = r.Open(); err == nil {
		t.Errorf("NewRingBuffer(%s, %s) succeeds, should fail", badname1, goodname2)
	}

	// This buffer should be Openable and Closeable
	r, err = NewRingBuffer(goodname1, goodname2)
	if err != nil {
		t.Error("Failed NewRingBuffer", err)
	}
	if err = r.Open(); err != nil {
		t.Error("Failed RingBuffer.Open", err)
	}
	if err = r.Close(); err != nil {
		t.Error("Failed RingBuffer.Close", err)
	}

	if err = writebuf.Close(); err != nil {
		t.Error("Failed RingBuffer.Close", err)
	}
}

func TestBufferWriteRead(t *testing.T) {
	name1 := "test_ring_buffer"
	name2 := "test_ring_description"
	// In case these memory regions exist from earlier tests, remove them.
	names := []string{name1, name2}
	for _, name := range names {
		shm.Unlink(name)
	}

	writebuf, err := NewRingBuffer(name1, name2)
	if err != nil {
		t.Error("Failed NewRingBuffer", err)
	}
	if err = writebuf.create(8192); err != nil {
		t.Error("Failed RingBuffer.create", err)
	}

	b, err := NewRingBuffer(name1, name2)
	if err != nil {
		t.Error("Failed NewRingBuffer", err)
	}
	if err = b.Open(); err != nil {
		t.Error("Failed RingBuffer.Open", err)
	}

	data, nbytes, err := b.Read(0)
	if err != nil {
		t.Error("Failed to b.Read(0)")
	} else if nbytes != 0 || len(data) > 0 {
		t.Errorf("b.Read(0) returned len(data)=%d, nbytes=%d, want 0 and 0",
			len(data), nbytes)
	}

	if err = b.Close(); err != nil {
		t.Error("Failed RingBuffer.Close", err)
	}
	if err = writebuf.Close(); err != nil {
		t.Error("Failed RingBuffer.Close", err)
	}
	if err = writebuf.unlink(); err != nil {
		t.Error("Failed RingBuffer.unlink", err)
	}
}
