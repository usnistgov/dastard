package dastard

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"gonum.org/v1/gonum/mat"
)

type triggerList struct {
	channelIndex                  int
	frames                        []FrameIndex
	keyFrame                      FrameIndex
	keyTime                       time.Time
	sampleRate                    float64
	lastFrameThatWillNeverTrigger FrameIndex
}

type edgeMultiInternalSearchStateType int

const (
	searching edgeMultiInternalSearchStateType = iota
	verifying
	initial
)

// TriggerState contains all the state that controls trigger logic
type TriggerState struct {
	AutoTrigger bool
	AutoDelay   time.Duration

	LevelTrigger bool
	LevelRising  bool
	LevelLevel   RawType

	EdgeTrigger bool
	EdgeRising  bool
	EdgeFalling bool
	EdgeLevel   int32

	EdgeMulti                        bool
	EdgeMultiNoise                   bool
	EdgeMultiMakeShortRecords        bool
	EdgeMultiMakeContaminatedRecords bool
	EdgeMultiDisableZeroThreshold    bool
	EdgeMultiLevel                   int32
	EdgeMultiVerifyNMonotone         int
	edgeMultiInternalSearchState     edgeMultiInternalSearchStateType
	edgeMultiIPotential              FrameIndex
	edgeMultiILastInspected          FrameIndex

	EMTState

	// TODO: group source/rx info.
}

// modify dsp to have it start looking for triggers at sample 6
// can't be sample 0 because we look back in time by up to 6 samples
// for kink fit
func (dsp *DataStreamProcessor) edgeMultiSetInitialState() {
	dsp.edgeMultiInternalSearchState = initial
	dsp.edgeMultiILastInspected = -math.MaxInt64 / 4
	dsp.LastEdgeMultiTrigger = -math.MaxInt64 / 4
	dsp.edgeMultiIPotential = -math.MaxInt64 / 4
	dsp.EMTState.reset()
}

// create a record using dsp.NPresamples and dsp.NSamples
func (dsp *DataStreamProcessor) triggerAt(segment *DataSegment, i int) *DataRecord {
	record := dsp.triggerAtSpecificSamples(segment, i, dsp.NPresamples, dsp.NSamples)
	return record
}

// create a record with NPresamples and NSamples passed as arguments
func (dsp *DataStreamProcessor) triggerAtSpecificSamples(segment *DataSegment, i int, NPresamples int, NSamples int) *DataRecord {
	data := make([]RawType, NSamples)
	// fmt.Printf("triggerAtSpecificSamples i %v, NPresamples %v, NSamples %v, len(rawData) %v\n", i, NPresamples, NSamples, len(segment.rawData))
	copy(data, segment.rawData[i-NPresamples:i+NSamples-NPresamples])
	tf := segment.firstFrameIndex + FrameIndex(i)
	tt := segment.TimeOf(i)
	sampPeriod := float32(1.0 / dsp.SampleRate)
	record := &DataRecord{data: data, trigFrame: tf, trigTime: tt,
		channelIndex: dsp.channelIndex, signed: segment.signed,
		voltsPerArb: segment.voltsPerArb,
		presamples:  NPresamples, sampPeriod: sampPeriod}
	return record
}

func min(a int, b int) int {
	if a <= b {
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
	} else {
		return i
	}
}

type NextTriggerIndResult struct {
	triggerInd   int32 // only use if triggerFound == true
	triggerFound bool
	nextIFirst   int32
}

// edgeMultiFindNextTriggerInd
func edgeMultiFindNextTriggerInd(raw []RawType, iFirst, iLast, threshold, nmonotone, max_nmonotone int32, enableZeroThreshold bool) NextTriggerIndResult {
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
				is_monotone := (rising && raw[i+j] > raw[i+j-1]) || (falling && raw[i+j] > raw[i+j-1])
				if !is_monotone || j >= max_nmonotone {
					foundMonotone = int32(j)
					break
				}
				j += 1
			}
			if foundMonotone >= nmonotone { // add trigger
				return NextTriggerIndResult{zeroThreshold(raw, i, enableZeroThreshold), true, i + foundMonotone + 1}
			}
		}
	}
	return NextTriggerIndResult{0, false, iLast + 1}
}

type EMTState struct {
	mode                    EMTMode
	threshold               int32
	nmonotone               int32
	npre                    int32
	nsamp                   int32
	enableZeroThreshold     bool
	nextFrameIndexToInspect FrameIndex
	t                       FrameIndex
	u                       FrameIndex
	v                       FrameIndex
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
	if !s.valid() {
		panic("EMT state is invalid")
	}
	s.nextFrameIndexToInspect = 0
	s.t, s.u, s.v = 0, 0, 0
}

type RecordSpec struct {
	firstRisingFrameIndex FrameIndex
	npre                  int32
	nsamp                 int32
}

type EMTMode int

const (
	EMTRecordsTwoFullLength EMTMode = iota
	EMTRecordsVariableLength
	EMTRecordsFullLengthIsolated
)

// t index of previous trigger
// u index of current trigger
// v index of next trigger
func edgeMultiShouldRecord(t, u, v FrameIndex, npreIn, nsampIn int32, mode EMTMode) (RecordSpec, bool) {
	// lastNPost := min(nsampIn-npreIn, int(u-t))
	npre := int32(min(int(npreIn), int(u-t)))
	npost := int32(min(int(nsampIn-npreIn), int(v-u)))
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

func (s EMTState) NToKeepOnTrim() int {
	return int(2*s.nsamp + 10)
}

func (s *EMTState) edgeMultiComputeAppendRecordSpecs(raw []RawType, frameIndexOfraw0 FrameIndex) []RecordSpec {
	// we promise to never index into raw outside of a potential record starting
	// at iFirst or iLast
	max_lookback := s.npre            // set by npre
	max_lookahead := s.nsamp - s.npre // set by npost=npre-nsamp
	// EMTState.valid, which is checked on reset, makes sue npre and (nsamp-npre) are each 4 or greater, so the kink
	// model can always look at least 4 samples back and 4 forward
	max_nmonotone := max_lookahead
	iFirst := int32(s.nextFrameIndexToInspect - frameIndexOfraw0)
	recordSpecs := make([]RecordSpec, 0)
	if iFirst < max_lookback { // state has been reset
		iFirst = max_lookback
		s.reset()
		fmt.Println("reseting edge multi state")
	}
	iLast := int32(len(raw)) - 1 - max_lookahead
	t, u, v := s.t, s.u, s.v
	for {
		x := edgeMultiFindNextTriggerInd(raw, iFirst, iLast, s.threshold, int32(s.nmonotone), max_nmonotone, s.enableZeroThreshold)
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
	if 0 < v && v < nextFrameIndexToInspect-FrameIndex(s.nsamp) {
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
	dsp.EMTState.nmonotone = int32(dsp.EdgeMultiVerifyNMonotone)
	dsp.EMTState.npre = int32(dsp.NPresamples)
	dsp.EMTState.nsamp = int32(dsp.NSamples)
	dsp.EMTState.threshold = dsp.EdgeMultiLevel
	dsp.EMTState.enableZeroThreshold = !dsp.EdgeMultiDisableZeroThreshold
	if dsp.EdgeMultiMakeShortRecords {
		dsp.EMTState.mode = EMTRecordsVariableLength
	} else if dsp.EdgeMultiMakeContaminatedRecords {
		dsp.EMTState.mode = EMTRecordsTwoFullLength
	} else {
		dsp.EMTState.mode = EMTRecordsFullLengthIsolated
	}
	recordSpecs := dsp.EMTState.edgeMultiComputeAppendRecordSpecs(segment.rawData, segment.firstFrameIndex)
	for _, recordSpec := range recordSpecs {
		record := dsp.triggerAtSpecificSamples(segment, int(recordSpec.firstRisingFrameIndex-segment.firstFrameIndex), int(recordSpec.npre), int(recordSpec.nsamp))
		records = append(records, record)
	}
	dsp.stream.TrimKeepingN(dsp.EMTState.NToKeepOnTrim()) // see comment at end of edgeMultiComputeAppendRecordSpecs
	return records
}

// func edgeMultiRecordizeAtTriggerIndsAndAppend(raw []int, triggerInds []int32, records []*DataRecord) []*DataRecord {
// 	var t, u, v, tFirst int
// 	// t index of previous trigger
// 	// u index of current trigger
// 	// v index of next trigger
// 	if dsp.LastEdgeMultiTrigger > 0 {
// 		tFirst = int(dsp.LastEdgeMultiTrigger - segment.firstFramenum)
// 	} else {
// 		tFirst = -dsp.NSamples
// 	}
// 	for i := 0; i < len(triggerInds); i++ {
// 		u = triggerInds[i]
// 		if i == len(triggerInds)-1 {
// 			v = iLast
// 		} else {
// 			v = triggerInds[i+1]
// 		}
// 		if i == 0 {
// 			t = tFirst

// 		} else {
// 			t = triggerInds[i-1]
// 		}
// 		// fmt.Printf("dsp.LastEdgeMultiTrigger %v, tFirst %v\n", dsp.LastEdgeMultiTrigger, tFirst)
// 		lastNPost := min(dsp.NSamples-dsp.NPresamples, int(u-t))
// 		npre := min(dsp.NPresamples, int(u-t-lastNPost))
// 		npost := min(dsp.NSamples-dsp.NPresamples, int(v-u))
// 		// fmt.Println("ch", dsp.channelIndex, "i", i, "npre", npre, "npost", npost, "t", t,
// 		// 	"u", u, "v", v, "lastNPost", lastNPost, "firstFramenum", segment.firstFramenum, "iLast", iLast)
// 		if dsp.EdgeMultiMakeShortRecords {
// 			// fmt.Printf("short trigger at u %v\n", u)
// 			// fmt.Println("ch", dsp.channelIndex, "i", i, "npre", npre, "npost", npost, "t", t,
// 			// 	"u", u, "v", v, "lastNPost", lastNPost, "firstFramenum", segment.firstFramenum, "iLast", iLast)
// 			newRecord := dsp.triggerAtSpecificSamples(segment, u, npre, npre+npost)
// 			records = append(records, newRecord)
// 		} else if dsp.EdgeMultiMakeContaminatedRecords {
// 			newRecord := dsp.triggerAtSpecificSamples(segment, u, dsp.NPresamples, dsp.NSamples)
// 			records = append(records, newRecord)
// 			if len(records) >= (len(raw)/dsp.NSamples)/2+1 {
// 				log.Println("limiting recordization rate of EdgeMultiMakeContaminatedRecords")
// 				break
// 			}
// 		} else if npre >= dsp.NPresamples && npre+npost >= dsp.NSamples {
// 			newRecord := dsp.triggerAtSpecificSamples(segment, u, dsp.NPresamples, dsp.NSamples)
// 			records = append(records, newRecord)
// 		}
// 	}
// }

// edgeMultiTriggerComputeAppend computes the EdgeMulti Trigger
// There are two modes
// 1. EdgeMulti: requires: EdgeMulti, EdgeMultiLevel, EdgeMultiVerifyNMonotone
// This mode looks for successive samples that differ by EdgeMultiLevel or more. These become potential triggers
// there are EdgeMultiVerifyNMonotone succesive samples with a positive difference
// A new trigger can be found  Immediatley after a local maximum has been found
// records are generated according to
// EdgeMultiMakeShortRecords  -> variable length records
// EdgeMultiMakeContaminatedRecords -> records that may have another pulse in them
// Neither -> Fixed length records without any other pulses in them (as if you already did postpeak deriv cut)
// 2. EdgeMultiNoise: requires: EdgeMulti, EdgeMulitNoise, EdgeMultiLevel, EdgeMultiVerifyNMonotone, AutoDelay
// will not produce pulse containing records, just the autotrigger that fit in around them
// negative values for EdgeMultiLevel look for negative going edges
func (dsp *DataStreamProcessor) edgeMultiTriggerComputeAppendOld(records []*DataRecord) []*DataRecord {
	segment := &dsp.stream.DataSegment
	raw := segment.rawData
	ndata := len(raw)

	var triggerInds []int
	var iPotential, iLast, iFirst int
	iPotential = int(dsp.edgeMultiIPotential - segment.firstFrameIndex)
	iLast = ndata + dsp.NPresamples - dsp.NSamples
	if dsp.edgeMultiILastInspected == 0 && segment.firstFrameIndex == 0 {
		if dsp.edgeMultiInternalSearchState != initial {
			dsp.edgeMultiInternalSearchState = initial
			for i := 0; i < 10; i++ {
				fmt.Printf("channelIndex %v needed edgeMultiInternalSearchState reset to initial\n", dsp.channelIndex)
			}
		}
		// I havent' figure out why this is neccesary but maybe it helps because of race conditions?
		// helps avoid iFirst <6
	}
	switch dsp.edgeMultiInternalSearchState {
	case verifying:
		iFirst = int(dsp.edgeMultiILastInspected-segment.firstFrameIndex) + 1
	case searching:
		iFirst = int(dsp.edgeMultiILastInspected-segment.firstFrameIndex) + 1
	case initial:
		iFirst = dsp.NPresamples
		if iFirst < 6 { // kink model looks at i-6
			iFirst = 6
		}
		if iFirst < dsp.NPresamples { // recordization looks at i-dsp.NPresamples
			iFirst = dsp.NPresamples
		}
	}

	// fmt.Println()
	// fmt.Println("dsp.channelIndex", dsp.channelIndex, "segment.firstFramenum", segment.firstFramenum, "dsp.edgeMultiIPotential", dsp.edgeMultiIPotential)
	// fmt.Println("dsp.edgeMultiInternalSearchState", dsp.edgeMultiInternalSearchState, "dsp.edgeMultiILastInspected", dsp.edgeMultiILastInspected, "dsp.EdgeMultiVerifyNMonotone", dsp.EdgeMultiVerifyNMonotone)
	// fmt.Println("iPotential", iPotential, "iFirst", iFirst, "iLast", iLast, "len(raw)", len(raw), "dsp.NPresamples", dsp.NPresamples)
	if dsp.EdgeMultiVerifyNMonotone+3 > dsp.NSamples-dsp.NPresamples {
		panic(fmt.Sprintf("dsp.EdgeMultiVerifyNMonotone %v, dsp.NSamples %v, dsp.NPreSamples %v", dsp.EdgeMultiVerifyNMonotone, dsp.NSamples, dsp.NPresamples))
	}
	if iFirst < 6 {
		panic(fmt.Sprintf("channel %v, iFirst %v<6!! segment.firstFramenum %v, dsp.edgeMultiILastInspected %v, dsp.edgeMultiInernalSearchState %v, dsp.NPresamples %v, dsp.NSamples %v, iLast %v, len(raw) %v, iPotential %v",
			dsp.channelIndex, iFirst, segment.firstFrameIndex, dsp.edgeMultiILastInspected, dsp.edgeMultiInternalSearchState, dsp.NPresamples, dsp.NSamples, iLast, len(raw), iPotential))
	}
	rising := dsp.EdgeMultiLevel >= 0
	falling := !rising
	for i := iFirst; i <= iLast; i++ {
		// fmt.Printf("i=%v, i_frame=%v\n", i, i+int(segment.firstFramenum))
		// dsp.edgeMultiILastInspected = FrameIndex(i) + segment.firstFramenum
		switch dsp.edgeMultiInternalSearchState {
		case initial:
			dsp.edgeMultiInternalSearchState = searching
		case searching:
			diff := int32(raw[i]) - int32(raw[i-1])
			if (rising && diff >= dsp.EdgeMultiLevel) ||
				(falling && diff <= dsp.EdgeMultiLevel) {
				iPotential = i
				dsp.edgeMultiInternalSearchState = verifying
			}
		case verifying:
			// now we have a potenial trigger
			// require following samples to each be greater than the last (for rising)
			if (rising && raw[i] <= raw[i-1]) || (falling && raw[i] >= raw[i-1]) { // here we see non-monotone sample, eg a fall on a rising edge
				nMonotone := i - iPotential
				if nMonotone >= dsp.EdgeMultiVerifyNMonotone {
					var iTrigger int
					if !dsp.EdgeMultiDisableZeroThreshold {
						// refine the trigger using the kink model
						xdataf := make([]float64, 10)
						ydataf := make([]float64, 10)
						for j := 0; j < 10; j++ {
							// fmt.Printf("j %v, iPotential %v, j+iPotential-6 %v, i %v\n", j, iPotential, j+iPotential-6, i)
							xdataf[j] = float64(j + iPotential - 6) // look at samples from i-6 to i+3
							ydataf[j] = float64(raw[j+iPotential-6])
						}
						ifit := float64(iPotential)
						kbest, _, err := kinkModelFit(xdataf, ydataf, []float64{ifit - 1, ifit - 0.5, ifit, ifit + 0.5, ifit + 1})
						if err == nil {
							iTrigger = int(math.Ceil(kbest))
						} else {
							iTrigger = iPotential
						}
					} else {
						iTrigger = iPotential
					}
					triggerInds = append(triggerInds, iTrigger)
				}
				dsp.edgeMultiInternalSearchState = searching
			} else if i-iPotential >= dsp.NSamples { // if it has been monotone for a whole record, that won't be a useful pulse, go back to searching
				dsp.edgeMultiInternalSearchState = searching
			}
		}
	}
	if iLast >= iFirst {
		dsp.edgeMultiILastInspected = FrameIndex(iLast) + segment.firstFrameIndex
	}

	dsp.edgeMultiIPotential = FrameIndex(iPotential) + segment.firstFrameIndex // dont need to condition this on EdgeMultiState because it only matters in state verifying

	var lastIThatCantEdgeTrigger int
	if dsp.edgeMultiInternalSearchState == searching {
		lastIThatCantEdgeTrigger = int(dsp.edgeMultiILastInspected-segment.firstFrameIndex) - 1
	} else if dsp.edgeMultiInternalSearchState == verifying {
		lastIThatCantEdgeTrigger = iPotential - 1
	}

	if !dsp.EdgeMultiNoise {
		var t, u, v, tFirst int
		// t index of previous trigger
		// u index of current trigger
		// v index of next trigger
		if dsp.LastEdgeMultiTrigger > 0 {
			tFirst = int(dsp.LastEdgeMultiTrigger - segment.firstFrameIndex)
		} else {
			tFirst = -dsp.NSamples
		}
		for i := 0; i < len(triggerInds); i++ {
			u = triggerInds[i]
			if i == len(triggerInds)-1 {
				v = iLast
			} else {
				v = triggerInds[i+1]
			}
			if i == 0 {
				t = tFirst

			} else {
				t = triggerInds[i-1]
			}
			// fmt.Printf("dsp.LastEdgeMultiTrigger %v, tFirst %v\n", dsp.LastEdgeMultiTrigger, tFirst)
			lastNPost := min(dsp.NSamples-dsp.NPresamples, int(u-t))
			npre := min(dsp.NPresamples, int(u-t-lastNPost))
			npost := min(dsp.NSamples-dsp.NPresamples, int(v-u))
			// fmt.Println("ch", dsp.channelIndex, "i", i, "npre", npre, "npost", npost, "t", t,
			// 	"u", u, "v", v, "lastNPost", lastNPost, "firstFramenum", segment.firstFramenum, "iLast", iLast)
			if dsp.EdgeMultiMakeShortRecords {
				// fmt.Printf("short trigger at u %v\n", u)
				// fmt.Println("ch", dsp.channelIndex, "i", i, "npre", npre, "npost", npost, "t", t,
				// 	"u", u, "v", v, "lastNPost", lastNPost, "firstFramenum", segment.firstFramenum, "iLast", iLast)
				newRecord := dsp.triggerAtSpecificSamples(segment, u, npre, npre+npost)
				records = append(records, newRecord)
			} else if dsp.EdgeMultiMakeContaminatedRecords {
				newRecord := dsp.triggerAtSpecificSamples(segment, u, dsp.NPresamples, dsp.NSamples)
				records = append(records, newRecord)
				if len(records) >= (len(raw)/dsp.NSamples)/2+1 {
					log.Println("limiting recordization rate of EdgeMultiMakeContaminatedRecords")
					break
				}
			} else if npre >= dsp.NPresamples && npre+npost >= dsp.NSamples {
				newRecord := dsp.triggerAtSpecificSamples(segment, u, dsp.NPresamples, dsp.NSamples)
				records = append(records, newRecord)
			}
		}
		if iLast >= iFirst {
			if dsp.edgeMultiInternalSearchState == verifying {
				// fmt.Println("dsp.NPresamples", dsp.NPresamples, "ndata", ndata, "iPotential", iPotential)
				extraSamplesToKeep := (ndata - iPotential) + 1 // +1 to account for maximum shift from kink model
				if extraSamplesToKeep < 0 {
					panic("negaive extraSamplestoKeep")
				}
				dsp.stream.TrimKeepingN(2*dsp.NSamples + extraSamplesToKeep)
			} else {
				dsp.stream.TrimKeepingN(2*dsp.NSamples + 1) // +1 to account for maximum shift from kink model
			}
		}
	}
	if dsp.EdgeMultiNoise {
		// fmt.Println("triggerInds")
		// spew.Dump(triggerInds)
		// fmt.Printf("iLast %v, iFirst %v, dsp.EdgeMultiNoise %v, segment.firstFramenum %v, len(raw) %v, dsp.LastEdgeMultiTrigger %v\n",
		// iLast, iFirst, dsp.EdgeMultiNoise, segment.firstFramenum, len(raw), dsp.LastEdgeMultiTrigger)
		// AutoTrigger
		if dsp.EdgeMultiNoise && iLast >= iFirst {
			delaySamples := roundint(dsp.AutoDelay.Seconds() * dsp.SampleRate)
			if delaySamples < dsp.NSamples {
				delaySamples = dsp.NSamples
			}
			idxNextTrig := 0
			nFoundTrigs := len(triggerInds)
			nextFoundTrig := math.MaxInt64
			if nFoundTrigs > 0 {
				nextFoundTrig = triggerInds[idxNextTrig]
			}

			// dsp.LastTrigger stores the frame of the last trigger found by the most recent invocation of TriggerData
			nextPotentialTrig := int(dsp.LastEdgeMultiTrigger-segment.firstFrameIndex) + delaySamples
			// fmt.Printf("nextPotentialTrig %v = dsp.LastEdgeMultiTrigger %v - segment.firstFramenum %v + delaySamples %v\n",
			// nextPotentialTrig, dsp.LastEdgeMultiTrigger, segment.firstFramenum, delaySamples)
			if nextPotentialTrig < dsp.NPresamples+1 {
				nextPotentialTrig = dsp.NPresamples + 1
			}

			// Loop through all potential trigger times.
			lastSampleOfNextPotentialTrig := nextPotentialTrig + dsp.NSamples - dsp.NPresamples - 1
			// fmt.Println("lastSampleOfNextPotentialTrig", lastSampleOfNextPotentialTrig, "lastIThatCantEdgeTrigger", lastIThatCantEdgeTrigger)
			for lastSampleOfNextPotentialTrig <= lastIThatCantEdgeTrigger { // can't go all the way to ndata
				// fmt.Printf("iLast %v, iFirst %v, nextPotentialTrig %v, delaySamples %v, nextFoundTrig %v, lastIThatCantEdgeTrigger %v, segment.firstFramenum %v, len(raw) %v\n",
				// 	iLast, iFirst, nextPotentialTrig, delaySamples, nextFoundTrig, lastIThatCantEdgeTrigger, segment.firstFramenum, len(raw))
				// fmt.Printf("potential trigger at %v, i=%v-%v, FrameIndex=%v-%v\n", nextPotentialTrig,
				// nextPotentialTrig-dsp.NPresamples, nextPotentialTrig+dsp.NSamples-dsp.NPresamples-1,
				// nextPotentialTrig-dsp.NPresamples+int(segment.firstFramenum), nextPotentialTrig+dsp.NSamples-dsp.NPresamples+int(segment.firstFramenum)-1)
				if nextPotentialTrig+dsp.NSamples <= nextFoundTrig {
					// auto trigger is allowed: no conflict with previously found non-auto triggers
					newRecord := dsp.triggerAt(segment, nextPotentialTrig)
					records = append(records, newRecord)
					// fmt.Println("trigger accepted")
					// fmt.Printf("trigger at %v, i=%v-%v, FrameIndex=%v-%v\n", nextPotentialTrig,
					// 	newRecord.FirstSampleFrame()-segment.firstFramenum, newRecord.LastSampleFrame()-segment.firstFramenum,
					// 	newRecord.FirstSampleFrame(), newRecord.LastSampleFrame())
					nextPotentialTrig += delaySamples
				} else {
					// fmt.Println("trigger rejected")
					// auto trigger not allowed: conflict with previously found non-auto triggers
					nextPotentialTrig = nextFoundTrig + delaySamples
					idxNextTrig++
					if nFoundTrigs > idxNextTrig {
						// fmt.Println("increment from next edgeTrigger")
						nextFoundTrig = triggerInds[idxNextTrig]
					} else {
						// fmt.Println("no more edgeTriggers")
						nextFoundTrig = math.MaxInt64
					}
				}
				lastSampleOfNextPotentialTrig = nextPotentialTrig + dsp.NSamples - dsp.NPresamples - 1
				// fmt.Println("lastSampleOfNextPotentialTrig", lastSampleOfNextPotentialTrig)
			}
			if iLast >= iFirst {
				dsp.stream.TrimKeepingN(2*dsp.NSamples + 1) // +1 to account for maximum shift from kink model
			}
		}

	}
	if len(records) > 0 {
		dsp.LastEdgeMultiTrigger = records[len(records)-1].trigFrame
	}
	// fmt.Printf("return %v of %v possible record. len(dsp.stream.rawData) %v\n", len(records), len(triggerInds), len(dsp.stream.rawData))
	return records
}

func (dsp *DataStreamProcessor) edgeTriggerComputeAppend(records []*DataRecord) []*DataRecord {
	if !dsp.EdgeTrigger {
		return records
	}
	segment := &dsp.stream.DataSegment
	raw := segment.rawData
	ndata := len(raw)

	// Solve the problem of signed data by shifting all values up by 2^15
	if dsp.stream.signed {
		raw = make([]RawType, ndata)
		copy(raw, segment.rawData)
		for i := 0; i < ndata; i++ {
			raw[i] += 32768
		}
	}

	for i := dsp.NPresamples; i < ndata+dsp.NPresamples-dsp.NSamples; i++ {
		diff := int32(raw[i]) + int32(raw[i-1]) - int32(raw[i-2]) - int32(raw[i-3])
		if (dsp.EdgeRising && diff >= dsp.EdgeLevel) ||
			(dsp.EdgeFalling && diff <= -dsp.EdgeLevel) {
			newRecord := dsp.triggerAt(segment, i)
			records = append(records, newRecord)
			i += dsp.NSamples
		}
	}
	return records
}

func (dsp *DataStreamProcessor) levelTriggerComputeAppend(records []*DataRecord) []*DataRecord {
	if !dsp.LevelTrigger {
		return records
	}
	segment := &dsp.stream.DataSegment
	raw := segment.rawData
	ndata := len(raw)
	nsamp := FrameIndex(dsp.NSamples)

	idxNextTrig := 0
	nFoundTrigs := len(records)
	nextFoundTrig := FrameIndex(math.MaxInt64)
	if nFoundTrigs > 0 {
		nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFrameIndex
	}

	// Solve the problem of signed data by shifting all values up by 2^15
	threshold := dsp.LevelLevel
	if dsp.stream.signed {
		threshold += 32768
		raw = make([]RawType, ndata)
		copy(raw, segment.rawData)
		for i := 0; i < ndata; i++ {
			raw[i] += 32768
		}
	}

	// Normal loop through all samples in triggerable range
	for i := dsp.NPresamples; i < ndata+dsp.NPresamples-dsp.NSamples; i++ {

		// Now skip over 2 record's worth of samples (minus 1) if an edge trigger is too soon in future.
		// Notice how this works: edge triggers get priority, vetoing (1 record minus 1 sample) into the past
		// and 1 record into the future.
		if FrameIndex(i)+nsamp > nextFoundTrig {
			i = int(nextFoundTrig) + dsp.NSamples - 1
			idxNextTrig++
			if nFoundTrigs > idxNextTrig {
				nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFrameIndex
			} else {
				nextFoundTrig = math.MaxInt64
			}
			continue
		}

		// If you get here, a level trigger is permissible. Check for it.
		if (dsp.LevelRising && raw[i] >= threshold && raw[i-1] < threshold) ||
			(!dsp.LevelRising && raw[i] <= threshold && raw[i-1] > threshold) {
			newRecord := dsp.triggerAt(segment, i)
			records = append(records, newRecord)
		}
	}
	sort.Sort(RecordSlice(records))
	return records
}

func (dsp *DataStreamProcessor) autoTriggerComputeAppend(records []*DataRecord) []*DataRecord {
	if !dsp.AutoTrigger {
		return records
	}
	segment := &dsp.stream.DataSegment
	raw := segment.rawData
	ndata := len(raw)
	nsamp := FrameIndex(dsp.NSamples)
	npre := FrameIndex(dsp.NPresamples)

	delaySamples := FrameIndex(dsp.AutoDelay.Seconds()*dsp.SampleRate + 0.5)
	if delaySamples < nsamp {
		delaySamples = nsamp
	}
	idxNextTrig := 0
	nFoundTrigs := len(records)
	nextFoundTrig := FrameIndex(math.MaxInt64)
	if nFoundTrigs > 0 {
		nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFrameIndex
	}

	// dsp.LastTrigger stores the frame of the last trigger found by the most recent invocation of TriggerData
	nextPotentialTrig := dsp.LastTrigger - segment.firstFrameIndex + delaySamples
	if nextPotentialTrig < npre {
		nextPotentialTrig = npre
	}

	// Loop through all potential trigger times.
	for nextPotentialTrig+nsamp-npre < FrameIndex(ndata) {
		if nextPotentialTrig+nsamp <= nextFoundTrig {
			// auto trigger is allowed: no conflict with previously found non-auto triggers
			newRecord := dsp.triggerAt(segment, int(nextPotentialTrig))
			records = append(records, newRecord)
			nextPotentialTrig += delaySamples

		} else {
			// auto trigger not allowed: conflict with previously found non-auto triggers
			nextPotentialTrig = nextFoundTrig + delaySamples
			idxNextTrig++
			if nFoundTrigs > idxNextTrig {
				nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFrameIndex
			} else {
				nextFoundTrig = math.MaxInt64
			}
		}
	}
	sort.Sort(RecordSlice(records))
	return records
}

// TriggerData analyzes a DataSegment to find and generate triggered records.
// All edge triggers are found, then level triggers, then auto and noise triggers.
func (dsp *DataStreamProcessor) TriggerData() (records []*DataRecord, secondaries []*DataRecord) {
	if dsp.EdgeMulti {
		// EdgeMulti does not play nice with other triggers!!
		records = dsp.edgeMultiTriggerComputeAppend(records)
		trigList := triggerList{channelIndex: dsp.channelIndex}
		trigList.frames = make([]FrameIndex, len(records))
		for i, r := range records {
			trigList.frames[i] = r.trigFrame
		}
		trigList.keyFrame = dsp.stream.DataSegment.firstFrameIndex
		trigList.keyTime = dsp.stream.DataSegment.firstTime
		trigList.sampleRate = dsp.SampleRate
		trigList.lastFrameThatWillNeverTrigger = dsp.stream.DataSegment.firstFrameIndex +
			FrameIndex(len(dsp.stream.rawData)) - FrameIndex(dsp.NSamples-dsp.NPresamples)

		// Step 2b: send the primary list to the group trigger broker; receive the secondary list.
		dsp.Broker.PrimaryTrigs <- trigList
		secondaryTrigList := <-dsp.Broker.SecondaryTrigs[dsp.channelIndex]
		segment := &dsp.stream.DataSegment
		for _, st := range secondaryTrigList {
			secondaries = append(secondaries, dsp.triggerAt(segment, int(st-segment.firstFrameIndex)))
		}
		return records, secondaries
	}

	// Step 1: compute where the primary triggers are, one pass per trigger type.

	// Step 1a: compute all edge triggers on a first pass. Separated by at least 1 record length
	records = dsp.edgeTriggerComputeAppend(records)

	// Step 1b: compute all level triggers on a second pass. Only insert them
	// in the list of triggers if they are properly separated from the edge triggers.
	records = dsp.levelTriggerComputeAppend(records)

	// Step 1c: compute all auto triggers, wherever they fit in between edge+level.
	records = dsp.autoTriggerComputeAppend(records)

	// TODO Step 1d: compute all noise triggers, wherever they fit in between edge+level.
	//

	// Step 1e: note the last trigger for the next invocation of TriggerData
	if len(records) > 0 {
		dsp.LastTrigger = records[len(records)-1].trigFrame
	}

	// Step 2: send the primary trigger list to the group trigger broker and await its
	// answer about when the secondary triggers are.

	// Step 2a: prepare the primary trigger list from the DataRecord list
	trigList := triggerList{channelIndex: dsp.channelIndex}
	trigList.frames = make([]FrameIndex, len(records))
	for i, r := range records {
		trigList.frames[i] = r.trigFrame
	}
	trigList.keyFrame = dsp.stream.DataSegment.firstFrameIndex
	trigList.keyTime = dsp.stream.DataSegment.firstTime
	trigList.sampleRate = dsp.SampleRate
	trigList.lastFrameThatWillNeverTrigger = dsp.stream.DataSegment.firstFrameIndex +
		FrameIndex(len(dsp.stream.rawData)) - FrameIndex(dsp.NSamples-dsp.NPresamples)

	// Step 2b: send the primary list to the group trigger broker; receive the secondary list.
	dsp.Broker.PrimaryTrigs <- trigList
	secondaryTrigList := <-dsp.Broker.SecondaryTrigs[dsp.channelIndex]
	segment := &dsp.stream.DataSegment
	for _, st := range secondaryTrigList {
		secondaries = append(secondaries, dsp.triggerAt(segment, int(st-segment.firstFrameIndex)))
	}

	// leave one full possible trigger in the stream
	// trigger algorithms should not inspect the last NSamples samples
	// fmt.Printf("Trimmed. %7d samples remain (requested %7d)\n", dsp.stream.TrimKeepingN(dsp.NSamples), dsp.NSamples)
	dsp.stream.TrimKeepingN(dsp.NSamples)
	return records, secondaries
}

// RecordSlice attaches the methods of sort.Interface to slices of DataRecords, sorting in increasing order.
type RecordSlice []*DataRecord

func (p RecordSlice) Len() int           { return len(p) }
func (p RecordSlice) Less(i, j int) bool { return p[i].trigFrame < p[j].trigFrame }
func (p RecordSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
