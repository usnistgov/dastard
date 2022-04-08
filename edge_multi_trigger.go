package dastard

import (
	"fmt"
	"log"
	"math"

	"gonum.org/v1/gonum/mat"
)

// EMTBackwardCompatibleRPCFields allows the old RPC messages to still work
// (consider it a temporary expedient 3/16/2022).
type EMTBackwardCompatibleRPCFields struct {
	EdgeMultiNoise                   bool
	EdgeMultiMakeShortRecords        bool
	EdgeMultiMakeContaminatedRecords bool
	EdgeMultiDisableZeroThreshold    bool
	EdgeMultiLevel                   int32
	EdgeMultiVerifyNMonotone         int
}

// The old EdgeMulti* parameters are no longer used in the triggering code,
// now being replaced with EMTState
// for now we're maintaining backwards compatibility with the old RPC calls
// so this is the point where we create an EMTState from the EdgeMulti* values
func (b EMTBackwardCompatibleRPCFields) toEMTState() (EMTState, error) {
	s := EMTState{}
	s.reset()
	if b.EdgeMultiNoise {
		return s, fmt.Errorf("EdgeMultiNoise not implemented")
	}
	if b.EdgeMultiMakeContaminatedRecords && b.EdgeMultiMakeShortRecords {
		return s, fmt.Errorf("EdgeMultiMakeContaminatedRecords and EdgeMultiMakeShortRecords are both true, which is invalid")
	}
	if b.EdgeMultiMakeContaminatedRecords {
		s.mode = EMTRecordsTwoFullLength
	} else if b.EdgeMultiMakeShortRecords {
		s.mode = EMTRecordsVariableLength
	} else {
		s.mode = EMTRecordsFullLengthIsolated
	}
	s.threshold = b.EdgeMultiLevel
	s.nmonotone = int32(b.EdgeMultiVerifyNMonotone)
	s.enableZeroThreshold = !b.EdgeMultiDisableZeroThreshold
	// if !s.valid() {
	// 	return EMTState{}, fmt.Errorf("EMTBackwardCompatibleRPCFields.toEMTState gave invalid state")
	// }
	return s, nil
}

func min(a int, b int) int {
	if a <= b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

// kinkModel returns a+b(x-k) for x<k and a+c(x-k) for x>=k
func kinkModel(k float64, x float64, a float64, b float64, c float64) float64 {
	if x < k {
		return a + b*(x-k)
	}
	return a + c*(x-k)
}

// kinkModelResult given k, xdata and ydata does a X2 fit for parameters a,b,c
// takes k, an index into xdata and into ydata
// returns ymodel - the model values evaluaed at the xdata
// returns a,b,c the model parameters
// returns X2 the sum of squares of difference between ymodel and ydata
// kinkModel(x) = a+b(x-k) for x<k and a+c(x-k) for x>=k
// err is the error from SolveVec, which is used for the fitting
func kinkModelResult(k float64, xdata []float64, ydata []float64) ([]float64, float64, float64, float64, float64, error) {
	var sdxi, sdxj, sdxi2, sdxj2, sy, syidxi, syjdxj float64
	// xi - (imaginary) list of xdata values with values less than k
	// xj - (imaginary) list of xdata values with values greater than or equal to to k
	// yi - (imaginary) list of ydata values where corresponding xdata value is less than k
	// yi - (imaginary) list of ydata values where corresponding xdata value is greater than or equal to to k
	// sdxi - sum of xi, with k subtracted from each value
	// sdxj - sum of xj, with k subtracted from each value
	// other variables follow same pattern
	for i := 0; i < len(xdata); i++ {
		xd := xdata[i] - k
		yd := ydata[i]
		sy += yd
		if xd < 0 {
			sdxi += xd
			sdxi2 += xd * xd
			syidxi += yd * xd
		} else {
			sdxj += xd
			sdxj2 += xd * xd
			syjdxj += yd * xd
		}
	}
	A := mat.NewDense(3, 3,
		[]float64{float64(len(xdata)), sdxi, sdxj,
			sdxi, sdxi2, 0,
			sdxj, 0, sdxj2})
	v := mat.NewVecDense(3, []float64{sy, syidxi, syjdxj})
	var abc mat.VecDense
	err := abc.SolveVec(A, v)
	a := abc.AtVec(0)
	b := abc.AtVec(1)
	c := abc.AtVec(2)
	var ymodel []float64
	var X2 float64
	for i := 0; i < len(xdata); i++ {
		ym := kinkModel(k, xdata[i], a, b, c)
		d := ydata[i] - ym
		X2 += d * d
		ymodel = append(ymodel, ym)
	}
	return ymodel, a, b, c, X2, err
}

// kinkModelFit return the k in ks that results in the lowest X2 when passed to kinkModelResult along with xdata and ydata
// also returns X2min. Immediatley returns with the error from kinkModelResult if kinkModelResult returns a non-nil error
func kinkModelFit(xdata []float64, ydata []float64, ks []float64) (float64, float64, error) {
	var X2min float64
	X2min = math.MaxFloat64
	var kbest float64
	var returnErr error
	for i := 0; i < len(ks); i++ {
		k := ks[i]

		_, _, _, _, X2, err := kinkModelResult(k, xdata, ydata)
		if X2 < X2min {
			X2min = X2
			kbest = k
		}
		// if k < 90 {
		// 	fmt.Println("k", k, "X2", X2, "X2min", X2min, "kbest", kbest, "err", err)
		// }
		if err != nil {
			returnErr = err
		}
	}
	return kbest, X2min, returnErr

}

// use the kink model to potentially adjust the trigger point by up to 1 sample
func zeroThreshold(raw []RawType, i int32, enable bool) int32 {
	if !enable {
		return i
	}
	xdataf := [8]float64{0, 0, 0, 0, 0, 0, 0, 0}
	ydataf := [8]float64{0, 0, 0, 0, 0, 0, 0, 0}
	for j := 0; j < 8; j++ {
		xdataf[j] = float64(int32(j) + i - 4) // look at samples from i-4 to i+3
		ydataf[j] = float64(raw[int32(j)+i-4])
	}
	ifit := float64(i)
	kbest, _, err := kinkModelFit(xdataf[:], ydataf[:], []float64{ifit - 1, ifit - 0.5, ifit, ifit + 0.5, ifit + 1})
	if err == nil {
		return int32(math.Ceil(kbest))
	}
	return i
}

// NextTriggerIndResult is the result of one search for an EMT.
// It says whether one was found and (if so) at what index.
type NextTriggerIndResult struct {
	triggerInd   int32 // only use if triggerFound == true
	triggerFound bool
	nextIFirst   int32
}

// edgeMultiFindNextTriggerInd
func edgeMultiFindNextTriggerInd(raw []RawType, iFirst, iLast, threshold, nmonotone,
	maxNmonotone int32, enableZeroThreshold bool) NextTriggerIndResult {
	rising := threshold >= 1
	falling := !rising
	var foundMonotone int32
	for i := iFirst; i <= iLast; i++ {
		diff := int32(raw[i]) - int32(raw[i-1])
		if (rising && diff >= threshold) ||
			(falling && diff <= threshold) {
			// validate n monotone
			// skip ahead by how many monotone samples we find
			foundMonotone = int32(-999999) // poison value to crash if no written over
			j := int32(1)
			for {
				isMonotone := (rising && raw[i+j] > raw[i+j-1]) || (falling && raw[i+j] < raw[i+j-1])
				if !isMonotone || j >= maxNmonotone {
					foundMonotone = int32(j)
					break
				}
				j++
			}
			if foundMonotone >= nmonotone { // add trigger
				return NextTriggerIndResult{zeroThreshold(raw, i, enableZeroThreshold), true, i + foundMonotone + 1}
			}
		}
	}
	return NextTriggerIndResult{0, false, int32(max(int(iLast+1), int(iFirst)))}
}

// EMTState enables the search for EMTs by storing the needed state.
type EMTState struct {
	mode                     EMTMode
	threshold                int32
	nmonotone                int32
	npre                     int32
	nsamp                    int32
	enableZeroThreshold      bool
	iFirstCheckSentinel      bool
	nextFrameIndexToInspect  FrameIndex
	t                        FrameIndex
	u                        FrameIndex
	v                        FrameIndex
	lastSeenFrameIndexofRaw0 FrameIndex
	lastNTrimmed             int32
}

func (s EMTState) valid() bool {
	if s.enableZeroThreshold && s.npre < 4 {
		return false
	}
	if s.enableZeroThreshold && (s.nsamp-s.npre) < 4 {
		return false
	}
	if s.nmonotone > s.nsamp-s.npre {
		return false
	}
	return true
}

func (s *EMTState) reset() {
	s.nextFrameIndexToInspect = 0
	s.t, s.u, s.v = 0, 0, 0
	s.iFirstCheckSentinel = false
}

// RecordSpec is the specification for a single variable-length record.
type RecordSpec struct {
	firstRisingFrameIndex FrameIndex // The index at which the "trigger" is located
	npre                  int32      // The number of pre-trigger samples
	nsamp                 int32      // The number of samples in the record
}

// EMTMode enumerates the possible ways to handle overlapping triggers.
type EMTMode int

// Enumerates the EMTMode constants.
const (
	EMTRecordsTwoFullLength      EMTMode = iota // Generate two full-length records even if they overlap.
	EMTRecordsVariableLength                    // Generate a variable-length record when triggers are too close together.
	EMTRecordsFullLengthIsolated                // Generate no records when triggers are too close together.
)

// t index of previous trigger
// u index of current trigger
// v index of next trigger
func edgeMultiShouldRecord(t, u, v FrameIndex, npreIn, nsampIn int32, mode EMTMode) (RecordSpec, bool) {
	lastNPost := min(int(nsampIn-npreIn), int(u-t))
	npre := int32(min(int(npreIn), int(u-t-FrameIndex(lastNPost))))
	npost := int32(min(int(nsampIn-npreIn), int(v-u)))
	// nextNPre := int32(min(int(npreIn), int(v-u)-int(npost)))
	if u == 0 || u == v || u == t {
		// u is set to 0 on reset, do not trigger until we have a new value
		// u is set to v when we recordize before finding the next trigger to handle
		// the "around the corner case"
		// this will eventually lead to u == t as more triggers are inspected
		return RecordSpec{-1, -1, -1}, false
	}
	if mode == EMTRecordsVariableLength {
		// always make a record, but dont overlap with neighboring triggers
		return RecordSpec{u, npre, npre + npost}, true
	} else if mode == EMTRecordsTwoFullLength {
		// always make a record, overlap with neighboring records
		return RecordSpec{u, npreIn, nsampIn}, true
	} else if mode == EMTRecordsFullLengthIsolated {
		if npre >= npreIn && npre+npost >= nsampIn {
			return RecordSpec{u, npreIn, nsampIn}, true
		}
	}
	return RecordSpec{-1, -1, -1}, false
}

// NToKeepOnTrim returns how many samples to keep when trimming a stream.
func (s *EMTState) NToKeepOnTrim() int {
	return int(2*s.nsamp + 10)
}

func (s *EMTState) edgeMultiComputeRecordSpecs(raw []RawType, frameIndexOfraw0 FrameIndex) []RecordSpec {
	// we promise to never index into raw outside of a potential record starting
	// at iFirst or iLast
	maxLookback := s.npre            // set by npre
	maxLookahead := s.nsamp - s.npre // set by npost=npre-nsamp
	// EMTState.valid, which is checked on reset, makes sue npre and (nsamp-npre) are each 4 or greater, so the kink
	// model can always look at least 4 samples back and 4 forward
	maxNmonotone := maxLookahead
	iFirst := int32(s.nextFrameIndexToInspect - frameIndexOfraw0)
	if s.lastNTrimmed > 0 { // check for data drops and reset if we see them
		expectedFramedIndexOfRaw0 := s.lastSeenFrameIndexofRaw0 + FrameIndex(s.lastNTrimmed)
		if frameIndexOfraw0 != expectedFramedIndexOfRaw0 {
			s.reset()
		} else {
			s.lastSeenFrameIndexofRaw0 = frameIndexOfraw0
		}
	}
	recordSpecs := make([]RecordSpec, 0)
	if iFirst < maxLookback { // state has been reset
		iFirst = maxLookback
		if s.iFirstCheckSentinel {
			log.Println("reseting edge multi state unexpectedly")
		}
		s.reset()
		s.iFirstCheckSentinel = true
	}
	iLast := int32(len(raw)) - 1 - maxLookahead
	t, u, v := s.t, s.u, s.v
	for {
		x := edgeMultiFindNextTriggerInd(raw, iFirst, iLast, s.threshold, int32(s.nmonotone), maxNmonotone, s.enableZeroThreshold)
		iFirst = x.nextIFirst
		if !x.triggerFound {
			break
		}
		t, u, v = u, v, FrameIndex(x.triggerInd)+frameIndexOfraw0
		// fmt.Println(t, u, v)
		recordSpec, valid := edgeMultiShouldRecord(t, u, v, s.npre, s.nsamp, s.mode)
		if valid {
			recordSpecs = append(recordSpecs, recordSpec)
		}
	}
	// now iFirst == iLast+1, because that is always the return value when we didn't find another trigger

	// at this point we need to handle records "around the corner", v may point to a valid record
	// but we may not know how long it should be until we get more data and call this function again
	// 1. v doesn't point to a record (aka v==0)
	// v. v points to a record far from iLast, we need to recordize it now and signal that
	// to the next call of this funciton
	// 3. v points to a record near iLast, and we can't handle it until we get more data
	// we just need to make sure we perserve enough data to recordize u later
	nextFrameIndexToInspect := FrameIndex(iFirst) + frameIndexOfraw0
	if 0 < v && v < nextFrameIndexToInspect-FrameIndex(s.nsamp-s.npre) {
		// here we handle cases 1 and 2
		// the earliest possible next trigger is at nextFrameIndexToInspect, so as long as v
		// is at least 1 nsamp from there, we know we can write a full length record
		recordSpec, valid := edgeMultiShouldRecord(u, v, nextFrameIndexToInspect, s.npre, s.nsamp, s.mode)
		if valid {
			recordSpecs = append(recordSpecs, recordSpec)
		}
		u = v // we signal that this has already been recordized by setting u = v
		// edgeMultiShouldRecord has a special case to check for u==v and not recordize
	}
	// case 3 is handled by ensuring that we always have atleast npre samples left after trimming
	// trimming is handled by edgeMultiTriggerComputeAppend
	// the maximum possible value of v that hasn't recordized is nextFrameIndexToInspect-FrameIndex(s.nsamp)
	// and we will at most look back npre samples from there, so we should preserve at least nsamp+npre samples before
	// nextFrameIndexToInspect, and nextFrameIndexToInspect is npost samples away from the end of raw.... so
	// we need to preserve at least nsamp+npre+npost=2nsamp samples, +(no more than)10 for the shift from the kink model
	s.t, s.u, s.v = t, u, v
	s.nextFrameIndexToInspect = nextFrameIndexToInspect
	return recordSpecs
}

func (dsp *DataStreamProcessor) edgeMultiTriggerComputeAppend(records []*DataRecord) []*DataRecord {
	segment := &dsp.stream.DataSegment
	dsp.EMTState.nsamp = int32(dsp.NSamples)
	dsp.EMTState.npre = int32(dsp.NPresamples)
	recordSpecs := dsp.EMTState.edgeMultiComputeRecordSpecs(segment.rawData, segment.firstFrameIndex)
	for _, recordSpec := range recordSpecs {
		// here we simply discard records if they would have been out of bounds
		// we probably dont need this since we track for frame drops inside EMTState, but lets be extra defensive
		record, err := dsp.tryTriggerAtSpecificSamples(segment, int(recordSpec.firstRisingFrameIndex-segment.firstFrameIndex), int(recordSpec.npre), int(recordSpec.nsamp))
		if err == nil {
			records = append(records, record)
		} else {
			log.Println(fmt.Sprintf("channelNumber %d discarding record because it would have panic'd", dsp.ChannelNumber))
		}
	}
	n0 := len(dsp.stream.rawData)
	dsp.stream.TrimKeepingN(dsp.EMTState.NToKeepOnTrim()) // see comment at end of edgeMultiComputeAppendRecordSpecs
	nTrimmed := n0 - len(dsp.stream.rawData)
	dsp.EMTState.lastNTrimmed = int32(nTrimmed)
	return records
}
