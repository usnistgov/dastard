package dastarddb

import (
	"testing"
)

// How to test a module that hits a real database?
// There are conflicting philosophies here, but for now, we are going
// to test only the unconnected "connection".

// func TestConnection(t *testing.T) {
// 	PingServer()
// }

func TestDummyConnection(t *testing.T) {
	conn := DummyDBConnection()
	if conn.IsConnected() {
		t.Error("Dummy connection IsConected()=true, want false")
	}
	abort := make(chan struct{})
	go conn.handleConnection(abort)
	conn.logActivity()
	if conn.err != nil {
		t.Error("Dummy connection logActivity() set error ", conn.err)
	}
	close(abort)
	conn.Wait()
}
