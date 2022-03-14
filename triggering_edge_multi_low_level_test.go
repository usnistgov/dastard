package dastard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEdgeMultiParts1(t *testing.T) {
	raw := [12]RawType{0, 0, 10, 20, 0, 0, 10, 20, 0, 0, 0, 0}
	iFirst := int32(1)
	iLast_max := int32(len(raw) - 1)
	threshold := int32(1)
	nmonotone := int32(1)
	max_nmonotone := int32(1)
	iLast := iLast_max - max_nmonotone
	result := edgeMultiFindNextTriggerInd(raw[:], iFirst, iLast, threshold, nmonotone, max_nmonotone)
	assert.Equal(t, NextTriggerIndResult{2, true, 4}, result, "basic edgeMultiFindNextTriggerInd usage 1")
	result = edgeMultiFindNextTriggerInd(raw[:], result.nextIFirst, iLast, threshold, nmonotone, max_nmonotone)
	assert.Equal(t, NextTriggerIndResult{6, true, 8}, result, "basic edgeMultiFindNextTriggerInd usage 2")
	result = edgeMultiFindNextTriggerInd(raw[:], result.nextIFirst, iLast, threshold, nmonotone, max_nmonotone)
	assert.Equal(t, NextTriggerIndResult{0, false, iLast + 1}, result, "basic edgeMultiFindNextTriggerInd usage 3")

	s := EMTState{
		threshold: 1, mode: EMTRecordsFullLengthIsolated,
		nmonotone: 1, npre: 1, nsamp: 2}
	recordSpecs := s.edgeMultiComputeAppendRecordSpecs(raw[:], 0)
	expectRecordSpecs := [1]RecordSpec{{firstRisingInd: 2, npre: 1, nsamp: 2}}
	assert.Equal(t, expectRecordSpecs[:], recordSpecs, "edgeMultiComputeAppendRecordSpecs usage 1")
	assert.Equal(t, FrameIndex(6), s.v, "EMTState is ready to trigger at index 6 as well")

}
