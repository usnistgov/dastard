package dastard

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEdgeMultiParts1(t *testing.T) {
	raw := [21]RawType{0, 0, 0, 0, 0, 0, 0, 0, 10, 0, 0, 0, 10, 0, 0, 0, 0, 0, 0, 0, 0}
	iFirst := int32(1)
	iLastMax := int32(len(raw) - 1)
	threshold := int32(1)
	nmonotone := int32(1)
	maxNmonotone := int32(1)
	enableZeroThreshold := false
	iLast := iLastMax - maxNmonotone
	result := edgeMultiFindNextTriggerInd(raw[:], iFirst, iLast, threshold, nmonotone, maxNmonotone, enableZeroThreshold)
	assert.Equal(t, NextTriggerIndResult{8, true, 10}, result, "basic edgeMultiFindNextTriggerInd usage 1")
	result = edgeMultiFindNextTriggerInd(raw[:], result.nextIFirst, iLast, threshold, nmonotone, maxNmonotone, enableZeroThreshold)
	assert.Equal(t, NextTriggerIndResult{12, true, 14}, result, "basic edgeMultiFindNextTriggerInd usage 2")
	result = edgeMultiFindNextTriggerInd(raw[:], result.nextIFirst, iLast, threshold, nmonotone, maxNmonotone, enableZeroThreshold)
	assert.Equal(t, NextTriggerIndResult{0, false, iLast + 1}, result, "basic edgeMultiFindNextTriggerInd usage 3")

	s := EMTState{
		threshold: 1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: 1, nsamp: 2}
	recordSpecs := s.edgeMultiComputeRecordSpecs(raw[:], 0)
	expectRecordSpecs := [2]RecordSpec{{firstRisingFrameIndex: 8, npre: 1, nsamp: 2},
		{firstRisingFrameIndex: 12, npre: 1, nsamp: 2}}
	assert.Equal(t, expectRecordSpecs[:], recordSpecs, "edgeMultiComputeAppendRecordSpecs usage 1")
	assert.Equal(t, FrameIndex(12), s.u, "EMTState should have u==v, to indicated that u has has been recordized")
	assert.Equal(t, FrameIndex(12), s.v, "EMTState should have u==v, to indicated that u has has been recordized")

	// a record right at the boundary
	rawF := [21]RawType{0, 0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	sF := EMTState{
		threshold: 1, mode: EMTRecordsVariableLength,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecsF := [2]RecordSpec{{firstRisingFrameIndex: 5, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 8, npre: 1, nsamp: 3}}
	recordSpecsF1 := sF.edgeMultiComputeRecordSpecs(rawF[0:7], 0)
	assert.Equal(t, expectRecordSpecsF[:0], recordSpecsF1, "no triggers first go F2")
	assert.Equal(t, FrameIndex(5), sF.nextFrameIndexToInspect, "edgeMultiComputeAppendRecordSpecs usage F2")
	n0F := len(rawF) - sF.NToKeepOnTrim()
	recordSpecsF2 := sF.edgeMultiComputeRecordSpecs(rawF[n0F:], FrameIndex(n0F))
	assert.Equal(t, expectRecordSpecsF[:], recordSpecsF2, "both triggers 2nd go F2")

	// two records, boundary such that first trigger in firt go, 2nd in 2nd go
	rawG := [21]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	sG := EMTState{
		threshold: 1, mode: EMTRecordsVariableLength,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecsG := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 7, npre: 1, nsamp: 3}}
	recordSpecsG1 := sG.edgeMultiComputeRecordSpecs(rawG[0:8], 0)
	assert.Equal(t, expectRecordSpecsG[:1], recordSpecsG1, "first record appears edgeMultiComputeAppendRecordSpecs usage G2")
	assert.Equal(t, FrameIndex(7), sG.nextFrameIndexToInspect, "edgeMultiComputeAppendRecordSpecs usage G2")
	n0G := len(rawG) - sG.NToKeepOnTrim()
	recordSpecsG2 := sG.edgeMultiComputeRecordSpecs(rawG[n0G:], FrameIndex(n0G))
	assert.Equal(t, expectRecordSpecsG[1:], recordSpecsG2, "2nd record appears")

	var tests = []struct {
		raw   []RawType
		state EMTState
		want  []RecordSpec
		label string
	}{
		{
			[]RawType{0, 0, 0, 0, 10, 20, 0, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			EMTState{threshold: 1, mode: EMTRecordsFullLengthIsolated,
				nmonotone: 1, npre: 2, nsamp: 4},
			[]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
				{firstRisingFrameIndex: 8, npre: 2, nsamp: 4}},
			"edgeMultiComputeAppendRecordSpecs: 2 records far enough apart to both trigger",
		},
		{
			[]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			EMTState{threshold: 1, mode: EMTRecordsFullLengthIsolated,
				nmonotone: 1, npre: 2, nsamp: 4},
			[]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4}},
			"edgeMultiComputeAppendRecordSpecs: 2 records too close so only first should recordize",
		},
		{
			[]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			EMTState{threshold: 1, mode: EMTRecordsTwoFullLength,
				nmonotone: 1, npre: 2, nsamp: 4},
			[]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
				{firstRisingFrameIndex: 7, npre: 2, nsamp: 4}},
			"edgeMultiComputeAppendRecordSpecs: 2 records should yield two full-length records",
		},
		{
			[]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			EMTState{threshold: 1, mode: EMTRecordsVariableLength,
				nmonotone: 1, npre: 2, nsamp: 4},
			[]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
				{firstRisingFrameIndex: 7, npre: 1, nsamp: 3}},
			"edgeMultiComputeAppendRecordSpecs: 2 records should yield one full-length record, one shorter",
		},
		{
			[]RawType{100, 100, 100, 100, 90, 80, 100, 100, 90, 80, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
			EMTState{threshold: -1, mode: EMTRecordsFullLengthIsolated,
				nmonotone: 1, npre: 2, nsamp: 4},
			[]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
				{firstRisingFrameIndex: 8, npre: 2, nsamp: 4}},
			"edgeMultiComputeAppendRecordSpecs: 2 records far enough apart to trigger both, negative going",
		},
		{
			[]RawType{0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10},
			EMTState{threshold: 1, mode: EMTRecordsTwoFullLength,
				nmonotone: 1, npre: 2, nsamp: 4},
			[]RecordSpec{{firstRisingFrameIndex: 2, npre: 2, nsamp: 4},
				{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
				{firstRisingFrameIndex: 6, npre: 2, nsamp: 4},
				{firstRisingFrameIndex: 8, npre: 2, nsamp: 4},
				{firstRisingFrameIndex: 10, npre: 2, nsamp: 4},
				{firstRisingFrameIndex: 12, npre: 2, nsamp: 4},
				{firstRisingFrameIndex: 14, npre: 2, nsamp: 4},
				{firstRisingFrameIndex: 16, npre: 2, nsamp: 4}},
			"edgeMultiComputeAppendRecordSpecs: lots over overlapping full length records",
		},
	}
	for _, test := range tests {
		recordSpecs := test.state.edgeMultiComputeRecordSpecs(test.raw[:], 0)
		assert.True(t, test.state.valid(), "EMT state should be valid")
		assert.Equal(t, test.want[:], recordSpecs, test.label)
	}
}

func TestEdgeMultiShouldRecord(t *testing.T) {
	var tests = []struct {
		tuv    []FrameIndex
		mode   EMTMode
		want   []int
		valid  bool
		errmsg string
	}{
		{[]FrameIndex{100, 200, 301}, EMTRecordsFullLengthIsolated, []int{200, 50, 100}, true, ""},
		{[]FrameIndex{301, 401, 700}, EMTRecordsFullLengthIsolated, []int{401, 50, 100}, true, ""},
		{[]FrameIndex{55, 55, 700}, EMTRecordsFullLengthIsolated, []int{-1, -1, -1}, false, "u==t is invalid"},
		{[]FrameIndex{55, 700, 700}, EMTRecordsFullLengthIsolated, []int{-1, -1, -1}, false, "u==v is invalid"},
		{[]FrameIndex{401, 460, 500}, EMTRecordsVariableLength, []int{460, 9, 49}, true,
			"variable length record are greedy on post trigger, give up pretrigger"},
	}
	for _, test := range tests {
		T := test.tuv[0]
		u := test.tuv[1]
		v := test.tuv[2]
		rspec, valid := edgeMultiShouldRecord(T, u, v, 50, 100, test.mode)
		frfi := FrameIndex(test.want[0])
		npre := int32(test.want[1])
		nsamp := int32(test.want[2])
		assert.Equal(t, RecordSpec{firstRisingFrameIndex: frfi, npre: npre, nsamp: nsamp}, rspec, "1")
		assert.Equal(t, test.valid, valid, test.errmsg)
	}
}

type RiseSpec struct {
	i    int
	n    int
	step int
}
type EndPoints struct {
	a, b int
}

type SignalSpec struct {
	nsamp            int
	npre             int
	risespecs        []RiseSpec
	rawlen           int
	baseline         int
	segmentEndPoints []EndPoints
}

func (ss SignalSpec) Signal() []RawType {
	return emtTestSignal(ss.rawlen, ss.baseline, ss.risespecs)
}

func emtTestSignal(rawlen int, baseline int, risespecs []RiseSpec) []RawType {
	raw := make([]RawType, rawlen)
	for i := range raw {
		raw[i] = RawType(baseline)
	}
	for _, rs := range risespecs {
		for j := 0; j < rs.n; j++ {
			raw[rs.i+j] += RawType((j + 1) * rs.step)
		}
	}
	return raw
}

func emtTestWithSubAndSignal(t *testing.T, ss SignalSpec, expected []FrameIndex, checkExpected bool, emtstate EMTState, testname string) {
	raw := ss.Signal()
	testEMTSubroutine(t, raw, ss.segmentEndPoints, testname, expected, checkExpected, emtstate)
}

func testEMTSubroutine(t *testing.T, raw []RawType, segmentEndPoints []EndPoints,
	trigname string, expectedFrames []FrameIndex, checkExpected bool, emtstate EMTState) ([]*DataRecord, []*DataRecord) {
	broker := NewTriggerBroker(1)
	dsp := NewDataStreamProcessor(0, broker, int(emtstate.npre), int(emtstate.nsamp))
	dsp.EMTState = emtstate
	dsp.EdgeMulti = true

	samplePeriod := time.Duration(float64(time.Second) / dsp.SampleRate)
	t0 := time.Now()
	var primaries, secondaries []*DataRecord
	for _, ab := range segmentEndPoints {
		a, b := ab.a, ab.b
		segment := NewDataSegment(raw[a:b], 1, FrameIndex(a), t0.Add(time.Duration(float64(samplePeriod)*float64(a))), samplePeriod)
		dsp.stream.AppendSegment(segment)

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
	if checkExpected {
		assert.Equal(t, expectedFramesInts, pTrigFramesInts, fmt.Sprintf("%s: expected trigger frames do not match found frames", trigname))
		if len(primaries) != len(expectedFrames) {
			t.Errorf("%s: have %v triggers, want %v triggers", trigname, len(primaries), len(expectedFrames))
			fmt.Print("have ")
			for _, p := range primaries {
				fmt.Printf("%v,", p.trigFrame)
			}
			fmt.Println()
			fmt.Print("want ")
			for _, v := range expectedFrames {
				fmt.Printf("%v,", v)
			}
			fmt.Println()
		}
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
	}
	return primaries, secondaries
}

func TestEMTComplexA(t *testing.T) {
	rawlen := 100
	ss := SignalSpec{
		nsamp: 10, npre: 5, rawlen: rawlen, baseline: 100, segmentEndPoints: []EndPoints{{0, rawlen}},
		risespecs: []RiseSpec{{i: 10, n: 5, step: 10}},
	}
	expected := []FrameIndex{10}
	emtstate := EMTState{threshold: 1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: int32(ss.npre), nsamp: int32(ss.nsamp)}
	emtTestWithSubAndSignal(t, ss, expected, true, emtstate, "TestEMTComplex one trigger very simple")
}

func TestEMTComplexB5TriggerOneSegment(t *testing.T) {
	rawlen := 120
	ss := SignalSpec{
		nsamp: 10, npre: 5, rawlen: rawlen, baseline: 100, segmentEndPoints: []EndPoints{{0, rawlen}},
		risespecs: []RiseSpec{{i: 10, n: 5, step: 10}, {i: 30, n: 5, step: 10}, {i: 50, n: 5, step: 10}, {i: 70, n: 5, step: 10}, {i: 90, n: 5, step: 10}},
	}
	expected := []FrameIndex{10, 30, 50, 70, 90}
	emtstate := EMTState{threshold: 1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: int32(ss.npre), nsamp: int32(ss.nsamp)}
	emtTestWithSubAndSignal(t, ss, expected, true, emtstate, "TestEMTComplex 5 trigger, one segment")
}

func TestEMTComplexC5TriggerManySegment(t *testing.T) {
	rawlen := 100
	ss := SignalSpec{
		nsamp: 10, npre: 5, rawlen: rawlen, baseline: 100, segmentEndPoints: []EndPoints{{0, rawlen}},
		risespecs: []RiseSpec{{i: 10, n: 5, step: 10}, {i: 30, n: 5, step: 10}, {i: 50, n: 5, step: 10}, {i: 70, n: 5, step: 10}, {i: 90, n: 5, step: 10}},
	}
	expected := []FrameIndex{10, 30, 50, 70, 90}
	emtstate := EMTState{threshold: 1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: int32(ss.npre), nsamp: int32(ss.nsamp)}
	emtTestWithSubAndSignal(t, ss, expected, true, emtstate, "A")
	ss.segmentEndPoints = []EndPoints{{0, 10}, {10, 20}, {20, 30}, {30, 40}, {40, 50}, {50, 60}, {60, 70}, {70, 80}, {80, 90}, {90, rawlen}}
	emtTestWithSubAndSignal(t, ss, expected, true, emtstate, "B")
	ss.segmentEndPoints = []EndPoints{{0, 70}, {70, 85}, {85, rawlen}}
	emtTestWithSubAndSignal(t, ss, expected, true, emtstate, "C")
	for i := 1; i < rawlen; i++ {
		for j := i + 1; j < rawlen; j++ {
			ss.segmentEndPoints = []EndPoints{{0, i}, {i, j}, {j, rawlen}}
			emtTestWithSubAndSignal(t, ss, expected, true, emtstate, fmt.Sprintf("i %d, j %d", i, j))
		}
	}
}

func TestEMTComplexD5TriggerManySegmentDrops(t *testing.T) {
	rawlen := 100
	ss := SignalSpec{
		nsamp: 10, npre: 5, rawlen: rawlen, baseline: 100, segmentEndPoints: []EndPoints{{0, rawlen}},
		risespecs: []RiseSpec{{i: 10, n: 5, step: 10}, {i: 30, n: 5, step: 10}, {i: 50, n: 5, step: 10}, {i: 70, n: 5, step: 10}, {i: 90, n: 5, step: 10}},
	}
	expected := []FrameIndex{10, 30, 50, 70, 90}
	emtstate := EMTState{threshold: 1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: int32(ss.npre), nsamp: int32(ss.nsamp)}
	emtTestWithSubAndSignal(t, ss, expected, true, emtstate, "A")
	emtstate.reset()
	ss.segmentEndPoints = []EndPoints{{0, 10}, {20, rawlen}}
	emtTestWithSubAndSignal(t, ss, expected[1:], true, emtstate, "B")
	emtstate.reset()
	ss.segmentEndPoints = []EndPoints{{0, 16}, {22, rawlen}} // found to panic via loop test
	emtTestWithSubAndSignal(t, ss, expected, true, emtstate, "C")

	// drop variable size, just look for panics?
	for i := 1; i < rawlen; i += 2 {
		for j := i + 1; j < rawlen; j += 2 {
			for k := j + 1; k < rawlen; k += 10 {
				ss.segmentEndPoints = []EndPoints{{0, i}, {j, k}, {k, rawlen}}
				// pass checkExpected = false so we don't spam test failures, we're just looking for panics
				emtTestWithSubAndSignal(t, ss, expected, false, emtstate, fmt.Sprintf("i %d, j %d, k %d", i, j, k))
			}
		}
	}
	for i := 1; i < rawlen; i++ {
		for j := i + 1; j < rawlen; j++ {
			ss.segmentEndPoints = []EndPoints{{0, i}, {j, rawlen}}
			// pass checkExpected = false so we don't spam test failures, we're just looking for panics
			emtTestWithSubAndSignal(t, ss, expected, false, emtstate, fmt.Sprintf("i %d, j %d", i, j))
		}
	}

}
