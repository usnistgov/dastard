package dastard

import (
	"testing"
)

func TestBaselineMonitor(t *testing.T) {
	nAvg := 100
	nStore := 100
	nPeak := 20
	bmon := NewBaselineMonitor(0, nAvg, nStore, nPeak)

	// Fill exactly once using AddOneValue repeatedly nAvg*nStore times. Expect one message.
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
	if msg.Value != 12345. {
		t.Errorf("Baseline monitor returned %f, want 12345", msg.Value)
	}

	// Fill exactly once using AddOneValue with nAvg*nStore total values. Expect one message.
	data := make([]RawType, nAvg*nStore)
	for i := range nAvg * 3 {
		data[i] = 900
	}
	messages := bmon.AddSliceValues(data)
	if messages == nil {
		t.Error("BaselineMonitor did not return a message when filled")
	}
	if len(messages) != 1 {
		t.Errorf("Baseline montitor returned %d messages, want 1", len(messages))
	}
	msg = messages[0]
	if msg.Value != 0. {
		t.Errorf("Baseline monitor returned %f, want 0", msg.Value)
	}

	// Fill 3x more in one call
	data = make([]RawType, nAvg*nStore*3)
	messages = bmon.AddSliceValues(data)
	if messages == nil {
		t.Error("BaselineMonitor did not return a message when filled")
	}
	if len(messages) != 3 {
		t.Errorf("Baseline montitor returned %d messages, want 3", len(messages))
	}
	for i, msg := range messages {
		if msg.Value != 0. {
			t.Errorf("Baseline monitor returned %f in message %d, want 0", msg.Value, i)
		}
	}

	// Check for expected errors
	if NewBaselineMonitor(0, nAvg, nStore, nStore+nPeak) != nil {
		t.Errorf("NewBaselineMonitor should fail with nPeak > nStore")
	}
}
