package dastard

import (
	"testing"
)

func TestBaselineMonitor(t *testing.T) {
	nAvg := 100
	nStore := 100
	nPeak := 20
	bmon := NewBaselineMonitor(0, nAvg, nStore, nPeak)
	// Fill exactly once using AddOneValue with nAvg*nStore total values. Expect one message.
	for range nAvg*nStore - nAvg {
		msgs := bmon.AddOneValue(12345)
		if msgs != nil {
			t.Errorf("BaselineMonitor has %d values in queue but returned messages %v", len(bmon.averages), msgs)
		}
	}
	for range nAvg - 1 {
		msgs := bmon.AddOneValue(0)
		if msgs != nil {
			t.Errorf("BaselineMonitor has %d values in queue but returned messages %v", len(bmon.averages), msgs)
		}
	}
	msgs := bmon.AddOneValue(993)
	if msgs == nil {
		t.Error("BaselineMonitor did not return a message when filled")
	}
	if len(msgs) != 1 {
		t.Errorf("Baseline montitor returned %d messages, want 1", len(msgs))
	}
	msg := msgs[0]
	if msg.Value != 12345. {
		t.Errorf("Baseline monitor returned %f, want 12345", msg.Value)
	}

	// Fill exactly once using AddOneValue with nAvg*nStore total values. Expect one message.
	data := make([]RawType, nAvg*nStore)
	for i := range nAvg * 3 {
		data[i] = 900
	}
	msgs = bmon.AddSliceValues(data)
	if msgs == nil {
		t.Error("BaselineMonitor did not return a message when filled")
	}
	if len(msgs) != 1 {
		t.Errorf("Baseline montitor returned %d messages, want 1", len(msgs))
	}
	msg = msgs[0]
	if msg.Value != 0. {
		t.Errorf("Baseline monitor returned %f, want 0", msg.Value)
	}

	// Fill 3x more in one call
	data = make([]RawType, nAvg*nStore*3)
	msgs = bmon.AddSliceValues(data)
	if msgs == nil {
		t.Error("BaselineMonitor did not return a message when filled")
	}
	if len(msgs) != 3 {
		t.Errorf("Baseline montitor returned %d messages, want 3", len(msgs))
	}
	for i, msg := range msgs {
		if msg.Value != 0. {
			t.Errorf("Baseline monitor returned %f in message %d, want 0", msg.Value, i)
		}
	}

	// Check for expected errors
	if NewBaselineMonitor(0, 100000, nStore, nPeak) != nil {
		t.Errorf("NewBaselineMonitor should fail with nAvg > 65536")
	}
	if NewBaselineMonitor(0, nAvg, nStore, nStore+nPeak) != nil {
		t.Errorf("NewBaselineMonitor should fail with nPeak > nStore")
	}
}
