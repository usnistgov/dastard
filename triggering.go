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
	channelIndex                int
	frames                      []FrameIndex
	keyFrame                    FrameIndex
	keyTime                     time.Time
	sampleRate                  float64
	firstFrameThatCannotTrigger FrameIndex
}

// used to allow the old RPC messages to still work
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

	EdgeMulti                      bool // enable EdgeMulti (actually used in triggering)
	EMTBackwardCompatibleRPCFields      // used to allow the old RPC messages to still work
	EMTState

	// TODO: group source/rx info.
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
				is_monotone := (rising && raw[i+j] > raw[i+j-1]) || (falling && raw[i+j] < raw[i+j-1])
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
	return NextTriggerIndResult{0, false, int32(max(int(iLast+1), int(iFirst)))}
}

type EMTState struct {
	mode                    EMTMode
	threshold               int32
	nmonotone               int32
	npre                    int32
	nsamp                   int32
	enableZeroThreshold     bool
	iFirstCheckSentinel     bool
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
	s.nextFrameIndexToInspect = 0
	s.t, s.u, s.v = 0, 0, 0
	s.iFirstCheckSentinel = false
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

func (s EMTState) NToKeepOnTrim() int {
	return int(2*s.nsamp + 10)
}

func (s *EMTState) edgeMultiComputeRecordSpecs(raw []RawType, frameIndexOfraw0 FrameIndex) []RecordSpec {
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
		if s.iFirstCheckSentinel {
			log.Println("reseting edge multi state unexpectedly")
		}
		s.reset()
		s.iFirstCheckSentinel = true
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
	recordSpecs := dsp.EMTState.edgeMultiComputeRecordSpecs(segment.rawData, segment.firstFrameIndex)
	for _, recordSpec := range recordSpecs {
		record := dsp.triggerAtSpecificSamples(segment, int(recordSpec.firstRisingFrameIndex-segment.firstFrameIndex), int(recordSpec.npre), int(recordSpec.nsamp))
		records = append(records, record)
	}
	dsp.stream.TrimKeepingN(dsp.EMTState.NToKeepOnTrim()) // see comment at end of edgeMultiComputeAppendRecordSpecs
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
// Returns slice of complete DataRecord objects, while dsp.lastTrigList stores
// a triggerList object just when the triggers happened.
func (dsp *DataStreamProcessor) TriggerData() (records []*DataRecord) {
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
		trigList.firstFrameThatCannotTrigger = dsp.stream.DataSegment.firstFrameIndex +
			FrameIndex(len(dsp.stream.rawData)) - FrameIndex(dsp.NSamples-dsp.NPresamples)

		dsp.lastTrigList = trigList
		return records
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

	// Step 2: prepare the primary trigger list from the DataRecord list.
	trigList := triggerList{channelIndex: dsp.channelIndex}
	trigList.frames = make([]FrameIndex, len(records))
	for i, r := range records {
		trigList.frames[i] = r.trigFrame
	}
	trigList.keyFrame = dsp.stream.DataSegment.firstFrameIndex
	trigList.keyTime = dsp.stream.DataSegment.firstTime
	trigList.sampleRate = dsp.SampleRate
	trigList.firstFrameThatCannotTrigger = dsp.stream.DataSegment.firstFrameIndex +
		FrameIndex(len(dsp.stream.rawData)) - FrameIndex(dsp.NSamples-dsp.NPresamples)

	dsp.lastTrigList = trigList
	return records
}

// TriggerDataSecondary converts a slice of secondary trigger frame numbers into a slice
// of records, the secondary trigger records.
func (dsp *DataStreamProcessor) TriggerDataSecondary(secondaryTrigList []FrameIndex) (secRecords []*DataRecord) {
	segment := &dsp.stream.DataSegment
	for _, st := range secondaryTrigList {
		secRecords = append(secRecords, dsp.triggerAt(segment, int(st-segment.firstFrameIndex)))
	}
	return secRecords
}

// RecordSlice attaches the methods of sort.Interface to slices of *DataRecords, sorting in increasing order.
type RecordSlice []*DataRecord

func (p RecordSlice) Len() int           { return len(p) }
func (p RecordSlice) Less(i, j int) bool { return p[i].trigFrame < p[j].trigFrame }
func (p RecordSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
