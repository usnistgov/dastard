package dastard

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestLongRecords ensures that we can generate triggers longer than 1 unit of
// data supply.
func TestLongRecords(t *testing.T) {
	const nchan = 1

	broker := NewTriggerBroker(nchan)
	var tests = []struct {
		npre   int
		nsamp  int
		nchunk int
	}{
		{9600, 10000, 999},
		{600, 10000, 999},
		{100, 10000, 999},
		{100, 10000, 1000},
		{100, 10000, 1001},
		{9100, 10000, 999},
		{9100, 10000, 1000},
		{9100, 10000, 1001},
		{1000, 10000, 9999},
		{1000, 10000, 10000},
		{1000, 10000, 10001},
	}
	for _, test := range tests {
		NPresamples := 256
		NSamples := 1024
		dsp := NewDataStreamProcessor(0, broker, NPresamples, NSamples)
		dsp.NPresamples = test.npre
		dsp.NSamples = test.nsamp
		dsp.SampleRate = 100000.0
		dsp.AutoTrigger = true
		dsp.AutoDelay = 500 * time.Millisecond
		expectedFrames := []FrameIndex{FrameIndex(dsp.NPresamples)}
		trigname := "Long Records auto"

		raw := make([]RawType, test.nchunk)
		dsp.LastTrigger = math.MinInt64 / 4 // far in the past, but not so far we can't subtract from it.
		sampleTime := time.Duration(float64(time.Second) / dsp.SampleRate)
		segment := NewDataSegment(raw, 1, 0, time.Now(), sampleTime)
		for i := 0; i <= dsp.NSamples; i += test.nchunk {
			primaries := dsp.TriggerData()
			ptl0 := map[int]triggerList{0: dsp.lastTrigList}
			secondaryMap, _ := broker.Distribute(ptl0)
			secondaries := dsp.TriggerDataSecondary(secondaryMap[0])
			if (len(primaries) != 0) || (len(secondaries) != 0) {
				t.Errorf("%s trigger found triggers after %d chunks added, want none", trigname, i)
			}
			dsp.stream.AppendSegment(segment)
			segment.firstFrameIndex += FrameIndex(test.nchunk)
		}
		primaries := dsp.TriggerData()
		ptl0 := map[int]triggerList{0: dsp.lastTrigList}
		secondaryMap, _ := broker.Distribute(ptl0)
		secondaries := dsp.TriggerDataSecondary(secondaryMap[0])
		if len(primaries) != len(expectedFrames) {
			t.Errorf("%s trigger (test=%v) found %d triggers, want %d", trigname, test, len(primaries), len(expectedFrames))
		}
		if len(secondaries) != 0 {
			t.Errorf("%s trigger found %d secondary (group) triggers, want 0", trigname, len(secondaries))
		}
		for i, pt := range primaries {
			if pt.trigFrame != expectedFrames[i] {
				t.Errorf("%s trigger at frame %d, want %d", trigname, pt.trigFrame, expectedFrames[i])
			}
		}
	}
}

// TestSingles tests that single edge, level, or auto triggers happen where expected.
func TestSingles(t *testing.T) {
	const nchan = 1

	broker := NewTriggerBroker(nchan)
	NPresamples := 256
	NSamples := 1024
	dsp := NewDataStreamProcessor(0, broker, NPresamples, NSamples)
	nRepeat := 1

	const bigval = 8000
	const tframe = 1000
	raw := make([]RawType, 10000)
	for i := tframe; i < tframe+10; i++ {
		raw[i] = bigval
	}
	const smallval = 1
	const tframe2 = 6000
	for i := tframe2; i < tframe2+10; i++ {
		raw[i] = smallval
	}

	dsp.NPresamples = 100
	dsp.NSamples = 1000
	dsp.SampleRate = 10000.0

	dsp.EdgeTrigger = true
	dsp.EdgeRising = true
	dsp.EdgeLevel = 100
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Edge", []FrameIndex{1000})

	dsp.EdgeTrigger = false
	dsp.LevelTrigger = true
	dsp.LevelRising = true
	dsp.LevelLevel = 100
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Level", []FrameIndex{1000})

	dsp.LevelTrigger = false
	dsp.AutoTrigger = true
	dsp.AutoDelay = 0 * time.Millisecond
	// Zero Delay results in records that are spaced by 1000 samples (dsp.NSamples)
	// starting at 100 (dsp.NPreSamples)
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Auto_0Millisecond", []FrameIndex{100, 1100, 2100, 3100, 4100, 5100, 6100, 7100, 8100})

	dsp.LevelTrigger = false
	dsp.AutoTrigger = true
	dsp.AutoDelay = 500 * time.Millisecond
	// first trigger is at NPreSamples=100
	// AutoDelay corresponds to 5000 samples, so we add that to 1100 to get 5100
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Auto_500Millisecond", []FrameIndex{100, 5100})

	dsp.LevelTrigger = true
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Level+Auto_500Millisecond", []FrameIndex{1000, 6000})

	dsp.LevelLevel = 1
	dsp.AutoTrigger = false
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Level_SmallThresh", []FrameIndex{1000, 6000})

	dsp.AutoDelay = 200 * time.Millisecond
	dsp.AutoTrigger = true
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Level+Auto_200Millisecond", []FrameIndex{1000, 3000, 5000, 6000, 8000})

	// Check that auto triggers are correct, particularly for multiple segments (issue #16)
	nRepeat = 4 // 4 seconds of data
	var expected []FrameIndex
	dsp.LevelTrigger = false
	dsp.NSamples = 1234
	dsp.NPresamples = 456
	expected = make([]FrameIndex, 0)
	for i := dsp.NPresamples; i < nRepeat*int(dsp.SampleRate); i += 2000 {
		expected = append(expected, FrameIndex(i))
	}
	testTriggerSubroutine(t, raw, nRepeat, dsp, "AutoMultipleSegmentsA", expected)

	dsp.AutoDelay = 1200 * time.Millisecond
	expected = make([]FrameIndex, 0)
	for i := dsp.NPresamples; i < nRepeat*int(dsp.SampleRate); i += 12000 {
		expected = append(expected, FrameIndex(i))
	}
	testTriggerSubroutine(t, raw, nRepeat, dsp, "AutoMultipleSegmentsB", expected)

	// Test signed signals
	for i := 0; i < len(raw); i++ {
		raw[i] = 65530
	}
	for i := tframe; i < tframe+10; i++ {
		raw[i] = bigval
	}
	for i := tframe2; i < tframe2+10; i++ {
		raw[i] = smallval
	}
	nRepeat = 1
	dsp.stream.signed = true
	dsp.LevelTrigger = false
	dsp.AutoTrigger = false
	dsp.EdgeTrigger = true
	dsp.EdgeRising = true
	dsp.EdgeLevel = 100
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Edge signed", []FrameIndex{1000})

	dsp.EdgeTrigger = false
	dsp.LevelTrigger = true
	dsp.LevelRising = true
	dsp.LevelLevel = 100
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Level signed", []FrameIndex{1000})
}

func testTriggerSubroutine(t *testing.T, raw []RawType, nRepeat int, dsp *DataStreamProcessor,
	trigname string, expectedFrames []FrameIndex) ([]*DataRecord, []*DataRecord) {
	// fmt.Println(trigname, len(dsp.stream.rawData))
	dsp.LastTrigger = math.MinInt64 / 4 // far in the past, but not so far we can't subtract from it.
	sampleTime := time.Duration(float64(time.Second) / dsp.SampleRate)
	segment := NewDataSegment(raw, 1, 0, time.Now(), sampleTime)
	segment.signed = dsp.stream.signed // dsp.stream.signed is later set equal to segment.signed
	dsp.stream.samplesSeen = 0
	dsp.Broker = NewTriggerBroker(1)
	var primaries, secondaries []*DataRecord
	for i := 0; i < nRepeat; i++ {
		dsp.stream.AppendSegment(segment)
		segment.firstFrameIndex += FrameIndex(len(raw))

		p := dsp.TriggerData()
		ptl0 := map[int]triggerList{0: dsp.lastTrigList}
		secondaryMap, _ := dsp.Broker.Distribute(ptl0)
		s := dsp.TriggerDataSecondary(secondaryMap[0])
		primaries = append(primaries, p...)
		secondaries = append(secondaries, s...)
	}
	pTrigFramesInts := make([]int, len(primaries))
	expectedFramesInts := make([]int, len(expectedFrames))
	for i, primary := range primaries {
		pTrigFramesInts[i] = int(primary.trigFrame)
	}
	for i, expected := range expectedFrames {
		expectedFramesInts[i] = int(expected)
	}
	assert.Equal(t, expectedFramesInts, pTrigFramesInts, fmt.Sprintf("%s: expected trigger frames do not match found frames", trigname))
	// if len(primaries) != len(expectedFrames) {
	// 	t.Errorf("%s: have %v triggers, want %v triggers", trigname, len(primaries), len(expectedFrames))
	// 	fmt.Print("have ")
	// 	for _, p := range primaries {
	// 		fmt.Printf("%v,", p.trigFrame)
	// 	}
	// 	fmt.Println()
	// 	fmt.Print("want ")
	// 	for _, v := range expectedFrames {
	// 		fmt.Printf("%v,", v)
	// 	}
	// 	fmt.Println()
	// }
	if len(secondaries) != 0 {
		t.Errorf("%s: trigger found %d secondary (group) triggers, want 0", trigname, len(secondaries))
	}
	for i, pt := range primaries {
		if i < len(expectedFrames) {
			if pt.trigFrame != expectedFrames[i] {
				t.Errorf("%s: trigger[%d] at frame %d, want %d", trigname, i, pt.trigFrame, expectedFrames[i])
			}
		}
	}

	// Check the data samples for the first trigger match raw, for samples where raw is long enough
	if len(primaries) != 0 && len(expectedFrames) != 0 {
		pt := primaries[0]
		offset := int(expectedFrames[0]) - dsp.NPresamples
		for i := 0; i < len(pt.data) && i+offset < len(raw) && offset >= 0; i++ {
			// fmt.Printf("i %v, offset %v, i+offset %v, len(raw) %v\n", i, offset, i+offset, len(raw))
			expect := raw[i+offset]
			if pt.data[i] != expect {
				t.Errorf("%s trigger[0] found data[%d]=%d, want %d", trigname, i,
					pt.data[i], expect)
			}
		}
	}
	dsp.stream.TrimKeepingN(0)
	return primaries, secondaries
}

// TestEdgeLevelInteraction tests that a single edge trigger happens where expected, even if
// there's also a level trigger.
func TestEdgeLevelInteraction(t *testing.T) {
	const nchan = 1

	broker := NewTriggerBroker(nchan)
	NPresamples := 256
	NSamples := 1024
	dsp := NewDataStreamProcessor(0, broker, NPresamples, NSamples)
	nRepeat := 1

	const bigval = 8000
	const tframe = 1000
	raw := make([]RawType, 10000)
	for i := tframe; i < tframe+10; i++ {
		raw[i] = bigval
	}
	const smallval = 1
	const tframe2 = 6000
	for i := tframe2; i < tframe2+10; i++ {
		raw[i] = smallval
	}
	dsp.NPresamples = 100
	dsp.NSamples = 1000

	dsp.EdgeTrigger = true
	dsp.EdgeRising = true
	dsp.EdgeLevel = 100
	dsp.LevelTrigger = true
	dsp.LevelRising = true
	dsp.LevelLevel = 100
	// should yield a single edge trigger
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Edge+Level 1", []FrameIndex{1000})
	dsp.LevelLevel = 10000
	// should yield a single edge trigger
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Edge+Level 2", []FrameIndex{1000})
	dsp.EdgeLevel = 20000
	dsp.LevelLevel = 100
	// should yield a single level trigger
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Edge + Level 3", []FrameIndex{1000})
	dsp.EdgeLevel = 1
	// should yield 2 edge triggers
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Edge + Level 4", []FrameIndex{1000, 6000})
	dsp.LevelLevel = 1
	dsp.EdgeLevel = 20000
	// should yield 2 level triggers
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Edge + Level 5", []FrameIndex{1000, 6000})
	dsp.LevelLevel = 1
	dsp.EdgeTrigger = false
	dsp.EdgeLevel = 1
	// should yield 2 level triggers
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Edge + Level 5", []FrameIndex{1000, 6000})
	// now exercise issue 25: when the 2nd edge trigger is too close to end-of-frame
	for i := 9050; i < 9060; i++ {
		raw[i] = bigval
	}
	dsp.EdgeTrigger = true
	testTriggerSubroutine(t, raw, nRepeat, dsp, "Edge + Level 6", []FrameIndex{1000, 6000, 9050})
}

func TestEdgeMulti(t *testing.T) {
	const nchan = 1

	broker := NewTriggerBroker(nchan)
	NPresamples := 50
	NSamples := 100
	dsp := NewDataStreamProcessor(0, broker, NPresamples, NSamples)
	dsp.EdgeMulti = true

	//kink model parameters
	var a, b, c float64
	a = 0
	b = 0
	c = 10
	raw := make([]RawType, 1000)
	kinkList := []float64{100, 200.1, 300.5, 400.9, 460, 500, 540, 700}
	kinkListFrameIndex := make([]FrameIndex, len(kinkList))
	for i := 0; i < len(kinkList); i++ {
		k := kinkList[i]
		kint := int(math.Ceil(k))
		for j := kint - 6; j < kint+20; j++ {
			raw[j] = RawType(math.Ceil(kinkModel(k, float64(j), a, b, c)))
			if j == kint+19 {
				raw[j] = RawType(kint) // make it easier to figure out which trigger you are looking at if you print raw
			}
		}
		kinkListFrameIndex[i] = FrameIndex(kint)
	}

	dsp.EMTState = EMTState{threshold: 10000, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 5, npre: 50, nsamp: 100, enableZeroThreshold: true}
	nRepeat := 1
	testTriggerSubroutine(t, raw, nRepeat, dsp, "EdgeMulti A: level too high, no records", []FrameIndex{})
	dsp.EMTState = EMTState{threshold: 1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 5, npre: 50, nsamp: 100, enableZeroThreshold: true}
	dsp.EMTState.reset() // call this between each edgeMulti test
	// here we will find all triggers in trigInds, but triggers that are too short are not recordized
	// the kinks that occur at fractional samples will end up the rounded value
	// my tests suggest testing at 0.5 step increments is ok on real data (eg a set of data from the Raven backup array test in 2018)
	testTriggerSubroutine(t, raw, nRepeat, dsp, "EdgeMulti B: full length isolated records", []FrameIndex{100, 200, 301, 401, 700})

	dsp.EMTState = EMTState{threshold: 1, mode: EMTRecordsTwoFullLength,
		nmonotone: 5, npre: 50, nsamp: 100, enableZeroThreshold: true}
	dsp.EMTState.reset() // call this between each edgeMulti test
	// here we will find all triggers in trigInds, and contaminated records will be created
	primaries, _ := testTriggerSubroutine(t, raw, nRepeat, dsp, "EdgeMulti C: MakeContaminatedRecords", []FrameIndex{100, 200, 301, 401, 460, 500, 540, 700})
	for _, record := range primaries {
		if len(record.data) != int(dsp.EMTState.nsamp) {
			t.Errorf("EdgeMulti C record has wrong number of samples %v", record)
		}
	}

	dsp.EMTState = EMTState{threshold: 1, mode: EMTRecordsVariableLength,
		nmonotone: 5, npre: 50, nsamp: 100, enableZeroThreshold: true}
	dsp.NPresamples = int(dsp.EMTState.npre)
	dsp.NSamples = int(dsp.EMTState.nsamp)
	dsp.EMTState.reset() // call this between each edgeMulti test
	// here we will find all triggers in trigInds, and short records will be created
	primaries, _ = testTriggerSubroutine(t, raw, nRepeat, dsp, "EdgeMulti D: MakeShortRecords", []FrameIndex{100, 200, 301, 401, 460, 500, 540, 700})
	///                                                                 lengths   100, 100, 100, 100, 49,  40,  50,  100
	expectLengths := []int{100, 100, 100, 100, 49, 40, 50, 100}
	for i, record := range primaries {
		if len(record.data) != expectLengths[i] {
			//if true {
			t.Errorf("EdgeMulti D record %v: expect len %v, have len %v, presamples %v, trigFrame %v, %v:%v", i, expectLengths[i],
				len(record.data), record.presamples, record.trigFrame, int(record.trigFrame)-record.presamples, int(record.trigFrame)-record.presamples+len(record.data)-1)
		}
	}

	// edgeMulti searches within a given segment from dsp.NPresamples to ndata + dsp.NPresamples - dsp.NSamples
	// for these values that is from 50 to 950
	// so we want to test triggering on an event that starts before 950, and continues rising past 950
	rawE := make([]RawType, 1000)
	kinkListE := []float64{945}
	kinkListFrameIndexE := make([]FrameIndex, len(kinkListE))
	for i := 0; i < len(kinkListE); i++ {
		k := kinkListE[i]
		kint := int(math.Ceil(k))
		for j := kint - 6; j < kint+20; j++ {
			rawE[j] = RawType(math.Ceil(kinkModel(k, float64(j), a, b, c)))
			if j == kint+19 {
				rawE[j] = RawType(kint) // make it easier to figure out which trigger you are looking at if you print rawE
			}
		}
		kinkListFrameIndexE[i] = FrameIndex(kint)
	}
	dsp.EMTState.reset() // call this between each edgeMulti test
	nRepeatE := 3
	// here we attempt to trigger around a segment boundary
	_, _ = testTriggerSubroutine(t, rawE, nRepeatE, dsp, "EdgeMulti E: handling segment boundary", []FrameIndex{945, 1945})

	dsp.EMTState = EMTState{threshold: 1, mode: EMTRecordsTwoFullLength,
		nmonotone: 5, npre: 6, nsamp: 15, enableZeroThreshold: true}
	dsp.NPresamples = int(dsp.EMTState.npre)
	dsp.NSamples = int(dsp.EMTState.nsamp)
	dsp.EMTState.reset() // call this between each edgeMulti test
	_, _ = testTriggerSubroutine(t, rawE, nRepeat, dsp, "EdgeMulti F: when it keeps rising for more than npost, at least trigger on the first one, after that anything is fine", []FrameIndex{945, 956})

	//kink model parameters
	var aFalling, bFalling, cFalling float64
	aFalling = 1000
	bFalling = 0
	cFalling = -10
	rawK := make([]RawType, 1000)
	for i := range rawK {
		rawK[i] = RawType(aFalling)
	}
	for i := 0; i < len(kinkList); i++ {
		k := kinkList[i]
		kint := int(math.Ceil(k))
		for j := kint - 6; j < kint+20; j++ {
			rawK[j] = RawType(math.Ceil(kinkModel(k, float64(j), aFalling, bFalling, cFalling)))
			if j == kint+19 {
				rawK[j] = RawType(kint) + RawType(aFalling) // make it easier to figure out which trigger you are looking at if you print raw
			}
		}
		kinkListFrameIndex[i] = FrameIndex(kint)
	}

	dsp.EMTState = EMTState{threshold: -10000, mode: EMTRecordsTwoFullLength,
		nmonotone: 5, npre: 50, nsamp: 100, enableZeroThreshold: true}
	dsp.NPresamples = int(dsp.EMTState.npre)
	dsp.NSamples = int(dsp.EMTState.nsamp)
	nRepeatK := 1
	dsp.EMTState.reset() // call this between each edgeMulti test
	testTriggerSubroutine(t, rawK, nRepeatK, dsp, "EdgeMulti K: level too large (negative)", []FrameIndex{})
	dsp.EMTState = EMTState{threshold: -1, mode: EMTRecordsTwoFullLength,
		nmonotone: 5, npre: 50, nsamp: 100, enableZeroThreshold: true}
	dsp.EMTState.reset() // call this between each edgeMulti test
	testTriggerSubroutine(t, rawK, nRepeatK, dsp, "EdgeMulti L: negative trigger level", []FrameIndex{100, 200, 301, 401, 460, 500, 540, 700})
}

// TestEdgeVetoesLevel tests that an edge trigger vetoes a level trigger as needed.
func TestEdgeVetoesLevel(t *testing.T) {
	const nchan = 1

	broker := NewTriggerBroker(nchan)
	NPresamples := 256
	NSamples := 1024
	dsp := NewDataStreamProcessor(0, broker, NPresamples, NSamples)
	dsp.NPresamples = 20
	dsp.NSamples = 100

	dsp.EdgeTrigger = true
	dsp.EdgeLevel = 290
	dsp.EdgeRising = true
	dsp.LevelTrigger = true
	dsp.LevelRising = true
	dsp.LevelLevel = 99

	// Run several data segments to make sure that the edge trigger vetoes the
	// level trigger when they happen too close in time.
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
		dsp.stream.AppendSegment(segment)
		primaries := dsp.TriggerData()
		dsp.TrimStream()
		if len(primaries) != want {
			fmt.Printf("Found %d records:\n", len(primaries))
			for _, p := range dsp.lastTrigList.frames {
				fmt.Printf("\t%v\n", p)
			}

			t.Errorf("EdgeVetosLevel problem with LCA=%d: saw %d triggers, want %d", lca, len(primaries), want)
		}
	}
}

func BenchmarkAutoTriggerOpsAre100SampleTriggers(b *testing.B) {
	const nchan = 1
	broker := NewTriggerBroker(nchan)
	NPresamples := 256
	NSamples := 1024
	dsp := NewDataStreamProcessor(0, broker, NPresamples, NSamples)
	dsp.NPresamples = 20
	dsp.NSamples = 100
	dsp.AutoTrigger = true
	dsp.SampleRate = 10000.0
	dsp.AutoDelay = 10 * time.Millisecond
	dsp.LastTrigger = math.MinInt64 / 4 // far in the past, but not so far we can't subtract from it.

	raw := make([]RawType, (b.N+1)*dsp.NSamples)
	sampleTime := time.Duration(float64(time.Second) / dsp.SampleRate)
	segment := NewDataSegment(raw, 1, 0, time.Now(), sampleTime)
	dsp.stream.AppendSegment(segment)
	b.ResetTimer()
	primaries := dsp.TriggerData()
	if len(primaries) != b.N {
		fmt.Println("wrong number", len(primaries), b.N)
	}
}

func BenchmarkEdgeTrigger0TriggersOpsAreSamples(b *testing.B) {
	const nchan = 1
	broker := NewTriggerBroker(nchan)
	NPresamples := 256
	NSamples := 1024
	dsp := NewDataStreamProcessor(0, broker, NPresamples, NSamples)
	dsp.NPresamples = 20
	dsp.NSamples = 100

	dsp.EdgeTrigger = true
	dsp.EdgeLevel = 290
	dsp.EdgeRising = true
	dsp.LevelTrigger = true
	dsp.LevelRising = true
	dsp.LevelLevel = 99
	dsp.AutoTrigger = true

	raw := make([]RawType, b.N)
	for i := 0; i < b.N; i++ {
		raw[i] = 0
	}
	segment := NewDataSegment(raw, 1, 0, time.Now(), time.Millisecond)
	dsp.stream.AppendSegment(segment)
	records := make([]*DataRecord, 0)
	b.ResetTimer()

	records = dsp.edgeTriggerComputeAppend(records)
	if len(records) != 0 {
		b.Fatal("no records")
	}

}

func BenchmarkLevelTrigger0TriggersOpsAreSamples(b *testing.B) {
	const nchan = 1
	broker := NewTriggerBroker(nchan)
	NPresamples := 256
	NSamples := 1024
	dsp := NewDataStreamProcessor(0, broker, NPresamples, NSamples)
	dsp.NPresamples = 20
	dsp.NSamples = 100

	dsp.EdgeTrigger = true
	dsp.EdgeLevel = 290
	dsp.EdgeRising = true
	dsp.LevelTrigger = true
	dsp.LevelRising = true
	dsp.LevelLevel = 99
	dsp.AutoTrigger = true

	raw := make([]RawType, b.N)
	for i := 0; i < b.N; i++ {
		raw[i] = 0
	}
	segment := NewDataSegment(raw, 1, 0, time.Now(), time.Millisecond)
	dsp.stream.AppendSegment(segment)
	records := make([]*DataRecord, 0)
	b.ResetTimer()

	records = dsp.levelTriggerComputeAppend(records)
	if len(records) != 0 {
		b.Fatal("no records")
	}

}

func TestKinkModel(t *testing.T) {
	xdata := []float64{0, 1, 2, 3, 4, 5, 6, 7}
	ydata := []float64{0, 0, 0, 0, 1, 2, 3, 4}
	ymodel, a, b, c, X2, err := kinkModelResult(3, xdata, ydata)
	if a != 0 || b != 0 || c != 1 || X2 != 0 || err != nil {
		t.Errorf("a %v, b %v, c %v, X2 %v, err %v, ymodel %v", a, b, c, X2, err, ymodel)
	}
	ymodel, a, b, c, X2, err = kinkModelResult(4, xdata, ydata)
	if a != 0.6818181818181821 || b != 0.22727272727272738 ||
		c != 1.1363636363636362 || X2 != 0.45454545454545453 || err != nil {
		t.Errorf("a %v, b %v, c %v, X2 %v, err %v, ymodel %v", a, b, c, X2, err, ymodel)
	}
	kbest, X2min, err := kinkModelFit(xdata, ydata, []float64{1, 2, 2.5, 3, 3.5, 4, 5})
	if kbest != 3 || X2min != 0 || err != nil {
		t.Errorf("kbest %v, X2min %v, err %v", kbest, X2min, err)
	}
}

func TestTriggerCounter(t *testing.T) {
	now := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	tc := NewTriggerCounter(0, time.Second)
	tList := triggerList{channelIndex: 0, frames: []FrameIndex{}, keyFrame: 0,
		keyTime: now, sampleRate: 1000, firstFrameThatCannotTrigger: 0}
	if err := tc.countNewTriggers(&tList); err != nil {
		t.Error(err)
	}
	if tc.hi != 0 {
		t.Errorf("have %v, want %v", tc.hi, 0)
	}
	if tc.lo != -999 {
		t.Errorf("have %v, want %v", tc.lo, -999)
	}
	if tc.countsSeen != len(tList.frames) {
		t.Errorf("want %v, have %v", len(tList.frames), tc.countsSeen)
	}
	tList = triggerList{channelIndex: 0, frames: []FrameIndex{1, 2, 3, 4, 5}, keyFrame: 100,
		keyTime: now.Add(100 * time.Millisecond), sampleRate: 1000, firstFrameThatCannotTrigger: 0}
	if err := tc.countNewTriggers(&tList); err != nil {
		t.Error(err)
	}
	if tc.countsSeen != len(tList.frames) {
		t.Errorf("want %v, have %v", len(tList.frames), tc.countsSeen)
	}
	if tc.hi != 1000 {
		t.Errorf("have %v, want %v", tc.hi, 1000)
	}
	if tc.lo != 1 {
		t.Errorf("have %v, want %v", tc.lo, 1)
	}
	tList = triggerList{channelIndex: 0, frames: []FrameIndex{1007, 1008, 1009, 2000, 2001}, keyFrame: 1900,
		keyTime: now.Add(1900 * time.Millisecond), sampleRate: 1000, firstFrameThatCannotTrigger: 0}
	if err := tc.countNewTriggers(&tList); err != nil {
		t.Error(err)
	}
	if tc.hi != 3000 {
		t.Errorf("have %v, want %v", tc.hi, 3000)
	}
	if tc.lo != 2001 {
		t.Errorf("have %v, want %v", tc.lo, 2001)
	}
	if tc.countsSeen != 1 { // 2001
		t.Errorf("want %v, have %v", 1, tc.countsSeen)
	}
	tList = triggerList{channelIndex: 0, frames: []FrameIndex{}, keyFrame: 1900,
		keyTime: now.Add(1900 * time.Millisecond), sampleRate: 1000, firstFrameThatCannotTrigger: 3001}
	if err := tc.countNewTriggers(&tList); err != nil {
		t.Error(err)
	}
	if tc.hi != 4000 {
		t.Errorf("have %v, want %v", tc.hi, 4000)
	}
	if tc.lo != 3001 {
		t.Errorf("have %v, want %v", tc.lo, 3001)
	}
	if len(tc.messages) != 4 {
		t.Errorf("have %v, expect %v", len(tc.messages), 4)
	}
	expectCounts := []int{0, 5, 4, 1}
	for i, m := range tc.messages {
		if expectCounts[i] != m.countsSeen {
			t.Errorf("message %v has %v counts, want %v", i, m.countsSeen, expectCounts[i])
		}

	}
}
