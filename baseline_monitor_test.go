package dastard

import (
	"testing"
	"time"
)

func TestBaselineMonitor(t *testing.T) {
	nAvg := 100
	nStore := 100
	nPeak := 20
	bmon := NewBaselineMonitor(0, nAvg, nStore, nPeak)
	// Fill exactly once using AddOneValue with nAvg*nStore total values. Expect one message.
	for range nAvg*nStore - nAvg {
		msg := bmon.AddOneValue(12345)
		if msg != nil {
			t.Errorf("BaselineMonitor has %d values in queue but returned messages %v", len(bmon.averages), msg)
		}
	}
	for range nAvg - 1 {
		msg := bmon.AddOneValue(0)
		if msg != nil {
			t.Errorf("BaselineMonitor has %d values in queue but returned messages %v", len(bmon.averages), msg)
		}
	}
	msg := bmon.AddOneValue(993)
	if msg == nil {
		t.Error("BaselineMonitor did not return a message when filled")
	}

	// Fill exactly once using AddOneValue with nAvg*nStore total values. Expect one message.
	data := make([]RawType, nAvg*nStore)
	for i := range nAvg * 3 {
		data[i] = 900
	}
	msgs := bmon.AddSliceValues(data)
	if msgs == nil {
		t.Error("baseline monitor did not return a message when filled")
	}
	if len(msgs) != 1 {
		t.Errorf("baseline monitor returned %d messages, want 1", len(msgs))
	}
	msg = msgs[0]
	if msg.Value != 0. {
		t.Errorf("baseline monitor returned %f, want 0", msg.Value)
	}

	// Fill 3x more in one call
	data = make([]RawType, nAvg*nStore*3)
	msgs = bmon.AddSliceValues(data)
	if msgs == nil {
		t.Error("baseline monitor did not return a message when filled")
	}
	if len(msgs) != 3 {
		t.Errorf("baseline montitor returned %d messages, want 3", len(msgs))
	}
	for i, msg := range msgs {
		if msg.Value != 0. {
			t.Errorf("baseline monitor returned %f in message %d, want 0", msg.Value, i)
		}
	}

	// Check for expected errors
	if NewBaselineMonitor(0, nAvg, nStore, nStore+nPeak) != nil {
		t.Errorf("func NewBaselineMonitor should fail with nPeak > nStore")
	}
}

func TestBaselineMonitorWriter(t *testing.T) {
	datadir := t.TempDir()
	ch := make(chan []*BaselineMonitorMessage, 10)
	if err := RunBaselineUpdater(datadir, ch); err != nil {
		t.Errorf("func RunBaselineUpdater failed on %s with error %v", datadir, err)
	}
	msg := BaselineMonitorMessage{ChanNum: 1, Timestamp: time.Now(), Value: 123.456}
	msgs := []*BaselineMonitorMessage{&msg, &msg}
	ch <- msgs
	ch <- msgs[:1]
	ch <- nil
	close(ch)
}
