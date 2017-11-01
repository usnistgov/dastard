package dastard

import (
	"testing"
	"time"
)

// TestBrokerConnections checks that we can connect/disconnect group triggers.
func TestBrokerConnections(t *testing.T) {
	N := 4
	broker := NewTriggerBroker(N)

	// First be sure there are no connections
	for i := 0; i < N+1; i++ {
		for j := 0; j < N+1; j++ {
			if broker.isConnected(i, j) {
				t.Errorf("New TriggerBroker.isConnected(%d,%d)==true, want false", i, j)
			}
		}
	}

	// Add 2 connections and make sure they are completed, but others aren't.
	broker.AddConnection(0, 2)
	broker.AddConnection(2, 0)
	if !broker.isConnected(0, 2) {
		t.Errorf("TriggerBroker.isConnected(0,2)==false, want true")
	}
	if !broker.isConnected(2, 0) {
		t.Errorf("TriggerBroker.isConnected(2,0)==false, want true")
	}
	i := 1
	for j := 0; j < N+1; j++ {
		if broker.isConnected(i, j) {
			t.Errorf("TriggerBroker.isConnected(%d,%d)==true, want false after connecting 0->2", i, j)
		}
	}

	// Now break the connections and check that they are disconnected
	broker.DeleteConnection(0, 2)
	broker.DeleteConnection(2, 0)
	for i := 0; i < N+1; i++ {
		for j := 0; j < N+1; j++ {
			if broker.isConnected(i, j) {
				t.Errorf("TriggerBroker.isConnected(%d,%d)==true, want false after disconnecting all", i, j)
			}
		}
	}

	// Try Add/Delete/check on channel numbers that should fail
	if err := broker.AddConnection(0, N); err == nil {
		t.Errorf("TriggerBroker.AddConnection(%d,0) should fail but didn't", N)
	}
	if err := broker.DeleteConnection(0, N); err == nil {
		t.Errorf("TriggerBroker.DeleteConnection(%d,0) should fail but didn't", N)
	}

	// Check the Connections method
	for i := -1; i < 1; i++ {
		con := broker.Connections(i)
		if len(con) > 0 {
			t.Errorf("TriggerBroker.Connections(%d)) has length %d, want 0", i, len(con))
		}
	}
	broker.AddConnection(1, 0)
	broker.AddConnection(2, 0)
	broker.AddConnection(3, 0)
	broker.AddConnection(2, 0)
	broker.AddConnection(3, 0)
	con := broker.Connections(0)
	if len(con) != 3 {
		t.Errorf("TriggerBroker.Connections(0) has length %d, want 3", len(con))
	}
	if con[0] {
		t.Errorf("TriggerBroker.Connections(0)[0]==true, want false")
	}
	for i := 1; i < 4; i++ {
		if !con[i] {
			t.Errorf("TriggerBroker.Connections(0)[%d]==false, want true", i)
		}
	}
}

// TestBrokering checks the group trigger brokering operations.
func TestBrokering(t *testing.T) {
	N := 4
	broker := NewTriggerBroker(N)
	abort := make(chan struct{})
	go broker.Run(abort)
	broker.AddConnection(0, 3)
	broker.AddConnection(2, 3)

	for iter := 0; iter < 3; iter++ {
		for i := 0; i < N; i++ {
			trigs := triggerList{i, []int64{int64(i) + 10, int64(i) + 20, 30}}
			broker.PrimaryTrigs <- trigs
		}
		t0 := <-broker.SecondaryTrigs[0]
		t1 := <-broker.SecondaryTrigs[1]
		t2 := <-broker.SecondaryTrigs[2]
		t3 := <-broker.SecondaryTrigs[3]
		for i, tn := range [][]int64{t0, t1, t2} {
			if len(tn) > 0 {
				t.Errorf("TriggerBroker chan %d received %d secondary triggers, want 0", i, len(tn))
			}
		}
		expected := []int64{10, 12, 20, 22, 30, 30}
		if len(t3) != len(expected) {
			t.Errorf("TriggerBroker chan %d received %d secondary triggers, want %d", 3, len(t3), len(expected))
		}
		for i := 0; i < len(expected); i++ {
			if t3[i] != expected[i] {
				t.Errorf("TriggerBroker chan %d secondary trig[%d]=%d, want %d", 3, i, t2[i], expected[i])
			}
		}
		if iter == 2 {
			close(abort)
		}
	}
}

func TestEdge(t *testing.T) {
	const nchan = 1
	abort := make(chan struct{})
	publisher := make(chan []*DataRecord)
	broker := NewTriggerBroker(nchan)
	go broker.Run(abort)
	dc := NewDataChannel(0, abort, publisher, broker)
	dc.NPresamples = 100
	dc.NSamples = 1000
	dc.EdgeTrigger = true
	dc.EdgeRising = true
	dc.EdgeLevel = 100

	raw := make([]RawType, 10000)
	for i := 1000; i < 1010; i++ {
		raw[i] = 8000
	}
	segment := NewDataSegment(raw, 1, 0, time.Now(), time.Millisecond)
	primaries, secondaries := dc.TriggerData(segment)
	if len(primaries) != 1 {
		t.Errorf("Edge trigger found %d triggers, want 1", len(primaries))
	}
	if len(secondaries) != 0 {
		t.Errorf("Edge trigger found %d secondary (group) triggers, want 0", len(secondaries))
	}
	pt := primaries[0]
	// Check the last 2 samples in pretrigger and the first 2 after
	expect := []RawType{0, 0, 8000, 8000}
	for i, exp := range expect {
		sampnum := i + dc.NPresamples - 1
		if pt.data[sampnum] != exp {
			t.Errorf("Edge trigger found data[%d]=%d, want %d", sampnum, pt.data[sampnum], exp)
		}
	}
	close(abort)
}