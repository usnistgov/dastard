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
	recordSpecs := s.edgeMultiComputeAppendRecordSpecs(raw[:], 0)
	expectRecordSpecs := [2]RecordSpec{{firstRisingFrameIndex: 8, npre: 1, nsamp: 2},
		{firstRisingFrameIndex: 12, npre: 1, nsamp: 2}}
	assert.Equal(t, expectRecordSpecs[:], recordSpecs, "edgeMultiComputeAppendRecordSpecs usage 1")
	assert.Equal(t, FrameIndex(12), s.u, "EMTState should have u==v, to indicated that u has has been recordized")
	assert.Equal(t, FrameIndex(12), s.v, "EMTState should have u==v, to indicated that u has has been recordized")

	rawA := [21]RawType{0, 0, 0, 0, 0, 0, 0, 0, 10, 20, 0, 0, 10, 20, 0, 0, 0, 0, 0, 0, 0}
	sA := EMTState{
		threshold: 1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: 2, nsamp: 4}
	recordSpecsA1 := sA.edgeMultiComputeAppendRecordSpecs(rawA[:], 0)
	assert.Equal(t, expectRecordSpecs[:], recordSpecsA1, "edgeMultiComputeAppendRecordSpecs usage 1")

}
