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

	// two records far enough apart to trigger both
	rawA := [21]RawType{0, 0, 0, 0, 10, 20, 0, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	sA := EMTState{
		threshold: 1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecsA := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 8, npre: 2, nsamp: 4}}
	recordSpecsA1 := sA.edgeMultiComputeRecordSpecs(rawA[:], 0)
	assert.Equal(t, expectRecordSpecsA[:], recordSpecsA1, "edgeMultiComputeAppendRecordSpecs usage A")

	// two trigger, too close so only first should recordize
	rawB := [21]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	sB := EMTState{
		threshold: 1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecsB := [1]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4}}
	recordSpecsB1 := sB.edgeMultiComputeRecordSpecs(rawB[:], 0)
	assert.Equal(t, expectRecordSpecsB[:], recordSpecsB1, "edgeMultiComputeAppendRecordSpecs usage B1")

	// two records, both should yield full length records
	rawC := [21]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	sC := EMTState{
		threshold: 1, mode: EMTRecordsTwoFullLength,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecsC := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 7, npre: 2, nsamp: 4}}
	recordSpecsC1 := sC.edgeMultiComputeRecordSpecs(rawC[:], 0)
	assert.Equal(t, expectRecordSpecsC[:], recordSpecsC1, "edgeMultiComputeAppendRecordSpecs usage C1")

	// two records, both should yield full length records
	rawD := [21]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	sD := EMTState{
		threshold: 1, mode: EMTRecordsTwoFullLength,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecsD := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 7, npre: 2, nsamp: 4}}
	recordSpecsD1 := sD.edgeMultiComputeRecordSpecs(rawD[:], 0)
	assert.Equal(t, expectRecordSpecsD[:], recordSpecsD1, "edgeMultiComputeAppendRecordSpecs usage D1")

	// two records, first full length, 2nd truncated in pretrigger
	rawE := [21]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	sE := EMTState{
		threshold: 1, mode: EMTRecordsVariableLength,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecsE := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 7, npre: 1, nsamp: 3}}
	recordSpecsE1 := sE.edgeMultiComputeRecordSpecs(rawE[:], 0)
	assert.Equal(t, expectRecordSpecsE[:], recordSpecsE1, "edgeMultiComputeAppendRecordSpecs usage E1")

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

	// two records far enough apart to trigger both, negative going
	rawH := [21]RawType{100, 100, 100, 100, 90, 80, 100, 100, 90, 80, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100}
	sH := EMTState{
		threshold: -1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecsH := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 8, npre: 2, nsamp: 4}}
	recordSpecsH1 := sH.edgeMultiComputeRecordSpecs(rawH[:], 0)
	assert.Equal(t, expectRecordSpecsH[:], recordSpecsH1, "edgeMultiComputeAppendRecordSpecs usage H")

	// contaminated records, trigger every 2 samples
	rawI := [21]RawType{0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10}
	sI := EMTState{
		threshold: 1, mode: EMTRecordsTwoFullLength,
		nmonotone: 1, npre: 2, nsamp: 4}
	assert.True(t, sI.valid(), "EMT state should be valid")
	expectRecordSpecsI := [8]RecordSpec{{firstRisingFrameIndex: 2, npre: 2, nsamp: 4}, {firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 6, npre: 2, nsamp: 4}, {firstRisingFrameIndex: 8, npre: 2, nsamp: 4}, {firstRisingFrameIndex: 10, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 12, npre: 2, nsamp: 4}, {firstRisingFrameIndex: 14, npre: 2, nsamp: 4}, {firstRisingFrameIndex: 16, npre: 2, nsamp: 4},
	}
	recordSpecsI1 := sI.edgeMultiComputeRecordSpecs(rawI[:], 0)
	assert.Equal(t, expectRecordSpecsI[:], recordSpecsI1, "edgeMultiComputeAppendRecordSpecs usage I")

}

func TestEdgeMultiShouldRecord(t *testing.T) {
	rspec, valid := edgeMultiShouldRecord(100, 200, 301, 50, 100, EMTRecordsFullLengthIsolated)
	assert.Equal(t, RecordSpec{firstRisingFrameIndex: 200, npre: 50, nsamp: 100}, rspec, "1")
	assert.Equal(t, true, valid, "")
	rspec, valid = edgeMultiShouldRecord(301, 401, 700, 50, 100, EMTRecordsFullLengthIsolated)
	assert.Equal(t, RecordSpec{firstRisingFrameIndex: 401, npre: 50, nsamp: 100}, rspec, "1")
	assert.Equal(t, true, valid, "")
	rspec, valid = edgeMultiShouldRecord(55, 55, 700, 50, 100, EMTRecordsFullLengthIsolated)
	assert.Equal(t, RecordSpec{firstRisingFrameIndex: -1, npre: -1, nsamp: -1}, rspec, "1")
	assert.Equal(t, false, valid, "u==t is invalid")
	rspec, valid = edgeMultiShouldRecord(55, 700, 700, 50, 100, EMTRecordsFullLengthIsolated)
	assert.Equal(t, RecordSpec{firstRisingFrameIndex: -1, npre: -1, nsamp: -1}, rspec, "1")
	assert.Equal(t, false, valid, "u==v is invalid")

	rspec, valid = edgeMultiShouldRecord(401, 460, 500, 50, 100, EMTRecordsVariableLength)
	assert.Equal(t, RecordSpec{firstRisingFrameIndex: 460, npre: 9, nsamp: 49}, rspec, "1")
	assert.Equal(t, true, valid, "variable length record are greedy on post trigger, give up pretrigger")
}
