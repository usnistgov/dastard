package dastard

import (
	"testing"

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
	rawF := [21]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	sF := EMTState{
		threshold: 1, mode: EMTRecordsVariableLength,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecsF := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 7, npre: 1, nsamp: 3}}
	recordSpecsF1 := sF.edgeMultiComputeRecordSpecs(rawF[0:8], 0)
	assert.Equal(t, expectRecordSpecsF[:0], recordSpecsF1, "no triggers first go F2")
	assert.Equal(t, FrameIndex(7), sF.nextFrameIndexToInspect, "edgeMultiComputeAppendRecordSpecs usage F2")
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
	recordSpecsG1 := sG.edgeMultiComputeRecordSpecs(rawG[0:10], 0)
	assert.Equal(t, expectRecordSpecsG[:1], recordSpecsG1, "first record appears edgeMultiComputeAppendRecordSpecs usage G2")
	assert.Equal(t, FrameIndex(10), sG.nextFrameIndexToInspect, "edgeMultiComputeAppendRecordSpecs usage G2")
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
