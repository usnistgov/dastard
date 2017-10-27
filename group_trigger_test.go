package dastard

import "testing"

// TestConnections checks that we can connect/disconnect group triggers.
func TestConnections(t *testing.T) {
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

	// Make connections that should fail
	if err := broker.AddConnection(N, 0); err == nil {
		t.Errorf("TriggerBroker.AddConnection(%d,0) should fail but didn't", N)
	}
	if err := broker.DeleteConnection(N, 0); err == nil {
		t.Errorf("TriggerBroker.DeleteConnection(%d,0) should fail but didn't", N)
	}
}
