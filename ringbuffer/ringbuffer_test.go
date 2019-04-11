package ringbuffer

import (
	"testing"
)

func TestBufferOpenClose(t *testing.T) {
	goodname1 := "will_exist_buffer"
	goodname2 := "will_exist_description"
	badname1 := "does_not_exist"
	badname2 := "does_not_exist_either"

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
