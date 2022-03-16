package dastard

import (
	"testing"
)

// TestBrokerConnections checks that we can connect/disconnect group triggers
// from the broker and the coupling of err and FB into each other for LanceroSources.
func TestBrokerConnections(t *testing.T) {
	N := 4
	broker := NewTriggerBroker(N)

	// First be sure there are no connections, initially.
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

	// Check the SourcesForReceiver method
	for i := -1; i < 1; i++ {
		con := broker.SourcesForReceiver(i)
		if len(con) > 0 {
			t.Errorf("TriggerBroker.SourcesForReceiver(%d)) has length %d, want 0", i, len(con))
		}
	}
	broker.AddConnection(1, 0)
	broker.AddConnection(2, 0)
	broker.AddConnection(3, 0)
	broker.AddConnection(2, 0)
	broker.AddConnection(3, 0)
	sources := broker.SourcesForReceiver(0)
	if len(sources) != 3 {
		t.Errorf("TriggerBroker.SourcesForReceiver(0) has length %d, want 3", len(sources))
	}
	if sources[0] {
		t.Errorf("TriggerBroker.SourcesForReceiver(0)[0]==true, want false")
	}
	for i := 1; i < 4; i++ {
		if !sources[i] {
			t.Errorf("TriggerBroker.SourcesForReceiver(0)[%d]==false, want true", i)
		}
	}

	// Now test FB <-> err coupling. This works when broker is embedded in a
	// LanceroSource.
	broker = NewTriggerBroker(N)
	var ls LanceroSource
	ls.nchan = N
	ls.broker = broker

	// FBToErr
	if err := ls.SetCoupling(FBToErr); err != nil {
		t.Errorf("SetCoupling(FBToErr) failed: %v", err)
	} else {
		for src := 0; src < N; src++ {
			for rx := 0; rx < N; rx++ {
				expect := (src-rx) == 1 && src%2 == 1
				c := broker.isConnected(src, rx)
				if c != expect {
					t.Errorf("After FB->Error isConnected(src=%d, rx=%d) is %t, want %t",
						src, rx, c, expect)
				}
			}
		}
	}

	// ErrToFB
	if err := ls.SetCoupling(ErrToFB); err != nil {
		t.Errorf("SetCoupling(ErrToFB) failed: %v", err)
	} else {
		for src := 0; src < N; src++ {
			for rx := 0; rx < N; rx++ {
				expect := (rx-src) == 1 && src%2 == 0
				c := broker.isConnected(src, rx)
				if c != expect {
					t.Errorf("After Error->Fb isConnected(src=%d, rx=%d) is %t, want %t",
						src, rx, c, expect)
				}
			}
		}
	}

	// None
	if err := ls.SetCoupling(NoCoupling); err != nil {
		t.Errorf("SetCoupling(NoCoupling) failed: %v", err)
	} else {
		for src := 0; src < N; src++ {
			for rx := 0; rx < N; rx++ {
				expect := false
				c := broker.isConnected(src, rx)
				if c != expect {
					t.Errorf("After NoCoupling isConnected(src=%d, rx=%d) is %t, want %t",
						src, rx, c, expect)
				}
			}
		}
	}
}

// TestBrokering checks the group trigger brokering operations.
func TestBrokering(t *testing.T) {
	N := 4
	broker := NewTriggerBroker(N)
	broker.AddConnection(0, 3)
	broker.AddConnection(2, 3)

	for iter := 0; iter < 3; iter++ {
		allTrigs := make(map[int]triggerList)
		for i := 0; i < N; i++ {
			trigs := triggerList{channelIndex: i, frames: []FrameIndex{FrameIndex(i) + 10, FrameIndex(i) + 20, 30}}
			allTrigs[i] = trigs
		}
		secondaryMap, _ := broker.Distribute(allTrigs)
		t0 := secondaryMap[0]
		t1 := secondaryMap[1]
		t2 := secondaryMap[2]
		t3 := secondaryMap[3]
		for i, tn := range [][]FrameIndex{t0, t1, t2} {
			if len(tn) > 0 {
				t.Errorf("TriggerBroker chan %d received %d secondary triggers, want 0", i, len(tn))
			}
		}
		expected := []FrameIndex{10, 12, 20, 22, 30, 30}
		if len(t3) != len(expected) {
			t.Errorf("TriggerBroker chan %d received %d secondary triggers, want %d", 3, len(t3), len(expected))
		}
		for i := 0; i < len(expected); i++ {
			if t3[i] != expected[i] {
				t.Errorf("TriggerBroker chan %d secondary trig[%d]=%d, want %d", 3, i, t2[i], expected[i])
			}
		}
	}
}
