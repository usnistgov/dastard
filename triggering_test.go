package main

import (
	"math"
	"testing"
	"time"
)

// TestBrokerConnections checks that we can connect/disconnect group triggers from the broker.
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
			trigs := triggerList{i, []FrameIndex{FrameIndex(i) + 10, FrameIndex(i) + 20, 30}}
			broker.PrimaryTrigs <- trigs
		}
		t0 := <-broker.SecondaryTrigs[0]
		t1 := <-broker.SecondaryTrigs[1]
		t2 := <-broker.SecondaryTrigs[2]
		t3 := <-broker.SecondaryTrigs[3]
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
		if iter == 2 {
			close(abort)
		}
	}
}

// TestSingles tests that single edge, level, or auto triggers happen where expected.
func TestSingles(t *testing.T) {
	const nchan = 1
	abort := make(chan struct{})
	defer close(abort)

	publisher := make(chan []*DataRecord)
	broker := NewTriggerBroker(nchan)
	go broker.Run(abort)
	dsp := NewDataStreamProcessor(0, abort, publisher, broker)
	dsp.NPresamples = 100
	dsp.NSamples = 1000
	dsp.SampleRate = 10000.0

	dsp.EdgeTrigger = true
	dsp.EdgeRising = true
	dsp.EdgeLevel = 100
	testTriggerSubroutine(t, dsp, "Edge", []FrameIndex{1000})

	dsp.EdgeTrigger = false
	dsp.LevelTrigger = true
	dsp.LevelRising = true
	dsp.LevelLevel = 100
	testTriggerSubroutine(t, dsp, "Level", []FrameIndex{1000})

	dsp.LevelTrigger = false
	dsp.AutoTrigger = true
	dsp.AutoDelay = 500 * time.Millisecond
	testTriggerSubroutine(t, dsp, "Auto", []FrameIndex{100, 5100})

	dsp.LevelTrigger = true
	testTriggerSubroutine(t, dsp, "Level+Auto", []FrameIndex{1000, 6000})

	dsp.AutoDelay = 200 * time.Millisecond
	testTriggerSubroutine(t, dsp, "Level+Auto", []FrameIndex{1000, 3000, 5000, 7000, 9000})
}

func testTriggerSubroutine(t *testing.T, dsp *DataStreamProcessor, trigname string, expectedFrames []FrameIndex) {
	const bigval = 8000
	const tframe = 1000
	raw := make([]RawType, 10000)
	for i := tframe; i < tframe+10; i++ {
		raw[i] = bigval
	}
	dsp.LastTrigger = math.MinInt64 / 4 // far in the past, but not so far we can't subtract from it.
	sampleTime := time.Duration(float64(time.Second) / dsp.SampleRate)
	segment := NewDataSegment(raw, 1, 0, time.Now(), sampleTime)
	primaries, secondaries := dsp.TriggerData(segment)
	if len(primaries) != len(expectedFrames) {
		t.Errorf("%s trigger found %d triggers, want %d", trigname, len(primaries), len(expectedFrames))
	}
	if len(secondaries) != 0 {
		t.Errorf("%s trigger found %d secondary (group) triggers, want 0", trigname, len(secondaries))
	}
	for i, pt := range primaries {
		if pt.trigFrame != expectedFrames[i] {
			t.Errorf("%s trigger at frame %d, want %d", trigname, pt.trigFrame, expectedFrames[i])
		}
	}

	// Check the data samples for the first trigger
	if len(primaries) == 0 {
		return
	}
	pt := primaries[0]
	offset := int(expectedFrames[0]) - dsp.NPresamples
	for i := 0; i < len(pt.data); i++ {
		expect := raw[i+offset]
		if pt.data[i] != expect {
			t.Errorf("%s trigger found data[%d]=%d, want %d", trigname, i,
				pt.data[i], expect)
		}
	}
}

// TestEdgeLevelInteraction tests that a single edge trigger happens where expected, even if
// there's also a level trigger.
func TestEdgeLevelInteraction(t *testing.T) {
	const nchan = 1
	abort := make(chan struct{})
	defer close(abort)

	publisher := make(chan []*DataRecord)
	broker := NewTriggerBroker(nchan)
	go broker.Run(abort)
	dsp := NewDataStreamProcessor(0, abort, publisher, broker)
	dsp.NPresamples = 100
	dsp.NSamples = 1000

	dsp.EdgeTrigger = true
	dsp.EdgeRising = true
	dsp.EdgeLevel = 100
	dsp.LevelTrigger = true
	dsp.LevelRising = true
	dsp.LevelLevel = 100
	testTriggerSubroutine(t, dsp, "Edge", []FrameIndex{1000})
	dsp.LevelLevel = 10000
	testTriggerSubroutine(t, dsp, "Edge", []FrameIndex{1000})
	dsp.EdgeLevel = 20000
	dsp.LevelLevel = 100
	testTriggerSubroutine(t, dsp, "Level", []FrameIndex{1000})
}

// TestEdgeVetosLevel tests that an edge trigger vetoes a level trigger as needed.
func TestEdgeVetosLevel(t *testing.T) {
	const nchan = 1
	abort := make(chan struct{})
	defer close(abort)

	publisher := make(chan []*DataRecord)
	broker := NewTriggerBroker(nchan)
	go broker.Run(abort)
	dsp := NewDataStreamProcessor(0, abort, publisher, broker)
	dsp.NPresamples = 20
	dsp.NSamples = 100

	dsp.EdgeTrigger = true
	dsp.EdgeLevel = 290
	dsp.EdgeRising = true
	dsp.LevelTrigger = true
	dsp.LevelRising = true
	dsp.LevelLevel = 99

	levelChangeAt := []int{50, 199, 200, 201, 299, 300, 301, 399, 400, 401, 500}
	edgeChangeAt := 300
	const rawLength = 1000
	expectNT := []int{2, 2, 2, 1, 1, 1, 1, 1, 1, 2, 2}
	for j, lca := range levelChangeAt {
		want := expectNT[j]

		raw := make([]RawType, rawLength)
		for i := lca; i < rawLength; i++ {
			raw[i] = 100
		}
		for i := edgeChangeAt; i < edgeChangeAt+100; i++ {
			raw[i] = 400
		}

		segment := NewDataSegment(raw, 1, 0, time.Now(), time.Millisecond)
		primaries, _ := dsp.TriggerData(segment)
		if len(primaries) != want {
			t.Errorf("EdgeVetosLevel problem with LCA=%d: saw %d triggers, want %d", lca, len(primaries), want)
		}
	}
}
