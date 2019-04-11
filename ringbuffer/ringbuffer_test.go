package ringbuffer

import (
	"testing"
)

func TestBufferOpenClose(t *testing.T) {
	goodname1 := "abaco_ring_buffer"
	goodname2 := "abaco_ring_description"
	badname1 := "does_not_exist"
	badname2 := "does_not_exist_either"
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
}
