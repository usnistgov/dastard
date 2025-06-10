package ringbuffer

import (
	"testing"
	"time"

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
	defer writebuf.Unlink()
	if err = writebuf.Create(8192); err != nil {
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
	for i := 0; i < 4; i++ {
		if err = r.Open(); err != nil {
			t.Error("Failed RingBuffer.Open", err)
		}
		if ps, _ := r.PacketSize(); ps != 8192 || err != nil {
			t.Errorf("PacketSize() returns (%d, err=%v), want (8192,nil)", ps, err)
		}
		if err = r.Close(); err != nil {
			t.Error("Failed RingBuffer.Close", err)
		}
		if ps, err := r.PacketSize(); ps != 0 || err == nil {
			t.Errorf("Closed RingBuffer PacketSize() returns (%d, err=nil), want (0, err)", ps)
		}
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
	buffersize := 8192
	if err = writebuf.Create(buffersize); err != nil {
		t.Error("Failed RingBuffer.create", err)
	}
	nbeef := 2000
	deadbeef := make([]byte, 0)
	bead5678 := make([]byte, 0)
	for i := 0; i < nbeef; i++ {
		deadbeef = append(deadbeef, []byte{0xde, 0xad, 0xbe, 0xef}...)
		bead5678 = append(bead5678, []byte{0xbe, 0xad, 0x56, 0x78}...)
	}
	writebuf.Write(deadbeef)

	b, err := NewRingBuffer(name1, name2)
	if err != nil {
		t.Error("Failed NewRingBuffer", err)
	}
	if err = b.Open(); err != nil {
		t.Error("Failed RingBuffer.Open", err)
	}

	// Test that reading 0 reads 0.
	data, err := b.Read(0)
	if err != nil {
		t.Error("Failed to b.Read(0)")
	} else if len(data) > 0 {
		t.Errorf("b.Read(0) returned len(data)=%d, want 0 and 0",
			len(data))
	}

	// There are <expect> bytes in the buffer. Test that reading them works.
	expect := 4 * nbeef
	if expect*2 <= buffersize {
		t.Error("Test design failure: intend to write enough bytes to exceed half buffer size.")
	}
	readable := b.BytesReadable()
	if readable != expect {
		t.Errorf("b.BytesReadable() returns %d, want %d", readable, expect)
	}
	data, err = b.Read(expect)
	if err != nil {
		t.Errorf("Failed to b.Read(%d)", expect)
	} else if len(data) != expect {
		t.Errorf("b.Read(%d) returned len(data)=%d, want %d",
			expect, len(data), expect)
	}
	readable = b.BytesReadable()
	if readable != 0 {
		t.Errorf("b.BytesReadable() returns %d, want 0", readable)
	}

	// There are 0 bytes in the buffer. Verify that.
	data, err = b.Read(expect)
	if err != nil {
		t.Errorf("Failed to b.Read(%d) when empty", expect)
	} else if len(data) != 0 {
		t.Errorf("b.Read(%d) returned len(data)=%d, want %d",
			expect, len(data), 0)
	}

	// Now put bytes in the buffer, clear it, and verify that there are 0.
	writebuf.Write(deadbeef)
	b.DiscardAll()
	data, err = b.Read(expect)
	if err != nil {
		t.Errorf("Failed to b.Read(%d) when empty", expect)
	} else if len(data) != 0 {
		t.Errorf("b.Read(%d) returned len(data)=%d, want %d",
			expect, len(data), 0)
	}

	// Now put bytes in the buffer, use DiscardStride, and verify that the remaining size is what we expect.
	writebuf.Write(deadbeef)
	writes := 3 //  because we've written deadbeef 3 times now.
	wantRump := 1000
	b.DiscardStride(uint64(writes*expect - wantRump))
	data, err = b.Read(expect)
	if err != nil {
		t.Errorf("Failed to b.Read(%d) when nearly empty", expect)
	} else if len(data) != wantRump {
		t.Errorf("b.Read(%d) after DiscardStride(%d) returned len(data)=%d, want %d",
			expect, writes*expect-wantRump, len(data), wantRump)
	}

	// Now put different bytes in the buffer, verify that they are the right values.
	writebuf.Write(bead5678)
	readable = b.BytesReadable()
	if readable != expect {
		t.Errorf("b.BytesReadable() returns %d, want %d", readable, expect)
	}
	data, err = b.Read(expect)
	if err != nil {
		t.Errorf("Failed to b.Read(%d): %v", expect, err)
	} else if len(data) != expect {
		t.Errorf("b.Read(%d) returned len(data)=%d, want %d",
			expect, len(data), expect)
	}
	for i := 0; i < len(data); i += 4 {
		if data[i] != 0xbe || data[i+1] != 0xad || data[i+2] != 0x56 || data[i+3] != 0x78 {
			t.Errorf("b.Read() returned data[%d:%d] = %v, want 0xbead5678", i, i+4, data[i:i+4])
		}
	}
	readable = b.BytesReadable()
	if readable != 0 {
		t.Errorf("b.BytesReadable() returns %d, want 0", readable)
	}

	// Write and read consecutive values, wrapping around the boundary
	w := b.desc.writePointer
	cap := b.desc.bufferSize
	nwrite := 100 + int(cap-(w%cap))
	consec := make([]byte, nwrite)
	for i := 0; i < nwrite; i++ {
		consec[i] = byte(i)
	}
	writebuf.Write(consec)
	expect = nwrite
	readable = b.BytesReadable()
	if readable != expect {
		t.Errorf("b.BytesReadable() returns %d, want %d", readable, expect)
	}
	data, err = b.Read(expect)
	if err != nil {
		t.Errorf("Failed to b.Read(%d)", expect)
	} else if len(data) != expect {
		t.Errorf("b.Read(%d) returned len(data)=%d, want %d",
			expect, len(data), expect)
	}
	for i := 0; i < nwrite; i++ {
		if data[i] != byte(i) {
			t.Errorf("b.Read() returned data[%d] = %d, want %d", i, data[i], byte(i))
		}
	}

	// Now write a certain amount and try to read more than that.
	writebuf.Write(consec)
	readable = b.BytesReadable()
	if readable != expect {
		t.Errorf("b.BytesReadable() returns %d, want %d", readable, expect)
	}
	data, err = b.Read(expect + 1000)
	if err != nil {
		t.Errorf("Failed to b.Read(%d)", expect)
	} else if len(data) != expect {
		t.Errorf("b.Read(%d) returned len(data)=%d, want %d",
			expect, len(data), expect)
	}

	// Now try to write more than the buffer can hold
	zeros := make([]byte, buffersize+20)
	written, err := writebuf.Write(zeros)
	if err != nil {
		t.Errorf("writebuf.Write() of buffer of size %d errors", len(zeros))
	}
	if written >= buffersize {
		t.Errorf("writebuf.Write() of buffer of size %d writes %d bytes, want < %d",
			len(zeros), written, buffersize)
	}

	// Empty the buffer. Fill it halfway, read half, fill completely, and read exactly half,
	// such that this read ends exactly where the buffer wraps. Is this okay? Does
	// BytesReadable() come back as expected?
	if err = b.DiscardAll(); err != nil {
		t.Error("b.DiscardAll() fails: ", err)
	}
	nh := buffersize / 2
	halfbuffer := make([]byte, nh)
	writebuf.Write(halfbuffer)
	if nr := writebuf.BytesReadable(); nr != nh {
		t.Errorf("writebuf.BytesReadable()=%d, want %d", nr, nh)
	}
	hbtest, err := writebuf.Read(nh)
	if err != nil {
		t.Errorf("writebuf.Read(%d) test failed on halfbuffer 1", nh)
	}
	for i := 0; i < nh; i++ {
		if halfbuffer[i] != hbtest[i] {
			t.Errorf("writebuf.Read(%d)[%d] = %d, want %d", nh, i, hbtest[i], halfbuffer[i])
			break
		}
	}
	if nr := writebuf.BytesReadable(); nr != 0 {
		t.Errorf("writebuf.BytesReadable()=%d, want %d", nr, 0)
	}

	// Now fill halfbuffer with nonzero values
	for i := 0; i < nh; i++ {
		halfbuffer[i] = byte(i)
	}

	expectedReadable := []int{nh, 2*nh - 1, 2*nh - 1}
	for iter, er := range expectedReadable {
		writebuf.Write(halfbuffer)
		if nr := writebuf.BytesReadable(); nr != er {
			t.Errorf("after iteration %d writebuf.BytesReadable()=%d, want %d", iter, nr, er)
		}
		ew := buffersize - 1 - er
		if nw := writebuf.BytesWriteable(); nw != ew {
			t.Errorf("after iteration %d writebuf.BytesWriteable()=%d, want %d", iter, nw, ew)
		}
	}
	hbtest, err = writebuf.Read(nh)
	if err != nil {
		t.Errorf("writebuf.Read(%d) test failed on halfbuffer 1", nh)
	}
	for i := 0; i < nh; i++ {
		if halfbuffer[i] != hbtest[i] {
			t.Errorf("writebuf.Read(%d)[%d] = %d, want %d", nh, i, hbtest[i], halfbuffer[i])
			break
		}
	}
	hbtest, err = writebuf.Read(nh)
	if err != nil {
		t.Errorf("writebuf.Read(%d) test failed on halfbuffer 1", nh)
	}
	for i := 0; i < len(hbtest); i++ {
		if halfbuffer[i] != hbtest[i] {
			t.Errorf("writebuf.Read(%d)[%d] = %d, want %d", nh, i, hbtest[i], halfbuffer[i])
			break
		}
	}

	// Empty the buffer, write some, and test ReadAll.
	if err = b.DiscardAll(); err != nil {
		t.Error("b.DiscardAll() fails: ", err)
	}
	writebuf.Write(deadbeef)
	data, err = b.ReadAll()
	if err != nil {
		t.Error("b.ReadAll() fails: ", err)
	} else if len(data) != len(deadbeef) {
		t.Errorf("b.ReadAll() returns %d bytes, want %d", len(data), len(deadbeef))
	}

	// Empty the buffer, try ReadMinimum first, then fill after a delay
	if err = b.DiscardAll(); err != nil {
		t.Error("b.DiscardAll() fails: ", err)
	}
	msize := 100
	shortmessage := make([]byte, msize)
	for i := 0; i < msize; i++ {
		shortmessage[i] = byte(i)
	}
	writebuf.Write(shortmessage)

	// Need this goroutine to close a channel: prevents an apparent race condition
	doneSleepTest := make(chan interface{})
	go func() {
		time.Sleep(5 * time.Millisecond)
		writebuf.Write(shortmessage)
		close(doneSleepTest)
	}()
	read, err := b.ReadMinimum(2 * msize)
	if err != nil {
		t.Errorf("b.ReadMinimum(%d) fails", 2*msize)
	}
	if len(read) != 2*msize {
		t.Errorf("b.ReadMinimum(%d) returns %d bytes",
			2*msize, read)
	}
	if _, err = b.ReadMinimum(buffersize * 2); err == nil {
		t.Errorf("b.ReadMinimum(%d) should fail for buffer size %d",
			2*buffersize, buffersize)
	}

	// Empty the buffer, try ReadMultipleOf first, then fill and try again
	if err = b.DiscardAll(); err != nil {
		t.Error("b.DiscardAll() fails: ", err)
	}
	writebuf.Write(shortmessage)
	data, err = b.ReadMultipleOf(msize * 2)
	if err != nil {
		t.Errorf("b.ReadMultipleOf(%d) fails", msize*2)
	}
	if len(data) > 0 {
		t.Errorf("b.ReadMultipleOf(%d) returns %d bytes, expect 0",
			msize*2, len(data))
	}
	data, err = b.ReadMultipleOf(msize / 2)
	if err != nil {
		t.Errorf("b.ReadMultipleOf(%d) fails", msize/2)
	}
	if len(data) != msize {
		t.Errorf("b.ReadMultipleOf(%d) returns %d bytes, expect %d",
			msize/2, len(data), msize)
	}
	writebuf.Write(shortmessage)
	data, err = b.ReadMultipleOf(msize - 1)
	if err != nil {
		t.Errorf("b.ReadMultipleOf(%d) fails", msize-1)
	}
	if len(data) != msize-1 {
		t.Errorf("b.ReadMultipleOf(%d) returns %d bytes, expect %d",
			msize-1, len(data), msize-1)
	}
	if _, err = b.ReadMultipleOf(buffersize * 2); err == nil {
		t.Errorf("b.ReadMultipleOf(%d) should fail for buffer size %d",
			2*buffersize, buffersize)
	}

	// Done with writing and reading. Close buffers.
	if err = b.Close(); err != nil {
		t.Error("Failed RingBuffer.Close", err)
	}
	<-doneSleepTest // prevent an apparent race condition
	if err = writebuf.Close(); err != nil {
		t.Error("Failed RingBuffer.Close", err)
	}
	if err = writebuf.Unlink(); err != nil {
		t.Error("Failed RingBuffer.unlink", err)
	}
}
