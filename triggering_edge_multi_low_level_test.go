package dastard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEdgeMultiParts1(t *testing.T) {
	raw := [21]RawType{0, 0, 0, 0, 0, 0, 0, 0, 10, 0, 0, 0, 10, 0, 0, 0, 0, 0, 0, 0, 0}
	iFirst := int32(1)
	iLast_max := int32(len(raw) - 1)
	threshold := int32(1)
	nmonotone := int32(1)
	max_nmonotone := int32(1)
	enableZeroThreshold := false
	iLast := iLast_max - max_nmonotone
	result := edgeMultiFindNextTriggerInd(raw[:], iFirst, iLast, threshold, nmonotone, max_nmonotone, enableZeroThreshold)
	assert.Equal(t, NextTriggerIndResult{8, true, 10}, result, "basic edgeMultiFindNextTriggerInd usage 1")
	result = edgeMultiFindNextTriggerInd(raw[:], result.nextIFirst, iLast, threshold, nmonotone, max_nmonotone, enableZeroThreshold)
	assert.Equal(t, NextTriggerIndResult{12, true, 14}, result, "basic edgeMultiFindNextTriggerInd usage 2")
	result = edgeMultiFindNextTriggerInd(raw[:], result.nextIFirst, iLast, threshold, nmonotone, max_nmonotone, enableZeroThreshold)
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
	raw_A := [21]RawType{0, 0, 0, 0, 10, 20, 0, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	s_A := EMTState{
		threshold: 1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecs_A := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 8, npre: 2, nsamp: 4}}
	recordSpecs_A1 := s_A.edgeMultiComputeRecordSpecs(raw_A[:], 0)
	assert.Equal(t, expectRecordSpecs_A[:], recordSpecs_A1, "edgeMultiComputeAppendRecordSpecs usage _A")

	// two trigger, too close so only first should recordize
	raw_B := [21]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	s_B := EMTState{
		threshold: 1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecs_B := [1]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4}}
	recordSpecs_B1 := s_B.edgeMultiComputeRecordSpecs(raw_B[:], 0)
	assert.Equal(t, expectRecordSpecs_B[:], recordSpecs_B1, "edgeMultiComputeAppendRecordSpecs usage _B1")

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
	raw_D := [21]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	s_D := EMTState{
		threshold: 1, mode: EMTRecordsTwoFullLength,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecs_D := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 7, npre: 2, nsamp: 4}}
	recordSpecs_D1 := s_D.edgeMultiComputeRecordSpecs(raw_D[:], 0)
	assert.Equal(t, expectRecordSpecs_D[:], recordSpecs_D1, "edgeMultiComputeAppendRecordSpecs usage _D1")

	// two records, first full length, 2nd truncated in pretrigger
	raw_E := [21]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	s_E := EMTState{
		threshold: 1, mode: EMTRecordsVariableLength,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecs_E := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 7, npre: 1, nsamp: 3}}
	recordSpecs_E1 := s_E.edgeMultiComputeRecordSpecs(raw_E[:], 0)
	assert.Equal(t, expectRecordSpecs_E[:], recordSpecs_E1, "edgeMultiComputeAppendRecordSpecs usage _E1")

	// a record right at the boundary
	raw_F := [21]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	s_F := EMTState{
		threshold: 1, mode: EMTRecordsVariableLength,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecs_F := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 7, npre: 1, nsamp: 3}}
	recordSpecs_F1 := s_F.edgeMultiComputeRecordSpecs(raw_F[0:8], 0)
	assert.Equal(t, expectRecordSpecs_F[:0], recordSpecs_F1, "no triggers first go _F2")
	assert.Equal(t, FrameIndex(7), s_F.nextFrameIndexToInspect, "edgeMultiComputeAppendRecordSpecs usage _F2")
	n0_F := len(raw_F) - s_F.NToKeepOnTrim()
	recordSpecs_F2 := s_F.edgeMultiComputeRecordSpecs(raw_F[n0_F:], FrameIndex(n0_F))
	assert.Equal(t, expectRecordSpecs_F[:], recordSpecs_F2, "both triggers 2nd go _F2")

	// two records, boundary such that first trigger in firt go, 2nd in 2nd go
	raw_G := [21]RawType{0, 0, 0, 0, 10, 20, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	s_G := EMTState{
		threshold: 1, mode: EMTRecordsVariableLength,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecs_G := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 7, npre: 1, nsamp: 3}}
	recordSpecs_G1 := s_G.edgeMultiComputeRecordSpecs(raw_G[0:10], 0)
	assert.Equal(t, expectRecordSpecs_G[:1], recordSpecs_G1, "first record appears edgeMultiComputeAppendRecordSpecs usage _G2")
	assert.Equal(t, FrameIndex(10), s_G.nextFrameIndexToInspect, "edgeMultiComputeAppendRecordSpecs usage _G2")
	n0_G := len(raw_G) - s_G.NToKeepOnTrim()
	recordSpecs_G2 := s_G.edgeMultiComputeRecordSpecs(raw_G[n0_G:], FrameIndex(n0_G))
	assert.Equal(t, expectRecordSpecs_G[1:], recordSpecs_G2, "2nd record appears")

	// two records far enough apart to trigger both, negative going
	raw_H := [21]RawType{100, 100, 100, 100, 90, 80, 100, 100, 90, 80, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100}
	s_H := EMTState{
		threshold: -1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: 2, nsamp: 4}
	expectRecordSpecs_H := [2]RecordSpec{{firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 8, npre: 2, nsamp: 4}}
	recordSpecs_H1 := s_H.edgeMultiComputeRecordSpecs(raw_H[:], 0)
	assert.Equal(t, expectRecordSpecs_H[:], recordSpecs_H1, "edgeMultiComputeAppendRecordSpecs usage _H")

	// contaminated records, trigger every 2 samples
	raw_I := [21]RawType{0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10}
	s_I := EMTState{
		threshold: 1, mode: EMTRecordsTwoFullLength,
		nmonotone: 1, npre: 2, nsamp: 4}
	assert.True(t, s_I.valid(), "EMT state should be valid")
	expectRecordSpecs_I := [8]RecordSpec{{firstRisingFrameIndex: 2, npre: 2, nsamp: 4}, {firstRisingFrameIndex: 4, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 6, npre: 2, nsamp: 4}, {firstRisingFrameIndex: 8, npre: 2, nsamp: 4}, {firstRisingFrameIndex: 10, npre: 2, nsamp: 4},
		{firstRisingFrameIndex: 12, npre: 2, nsamp: 4}, {firstRisingFrameIndex: 14, npre: 2, nsamp: 4}, {firstRisingFrameIndex: 16, npre: 2, nsamp: 4},
	}
	recordSpecs_I1 := s_I.edgeMultiComputeRecordSpecs(raw_I[:], 0)
	assert.Equal(t, expectRecordSpecs_I[:], recordSpecs_I1, "edgeMultiComputeAppendRecordSpecs usage _I")

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
