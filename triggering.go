package dastard

import (
	"fmt"
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

type edgeMultiStateType int

const (
	searching edgeMultiStateType = iota
	verifying
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
	EdgeMultiMakeShortRecords        bool
	EdgeMultiMakeContaminatedRecords bool
	EdgeMultiVerifyNMonotone         int
	edgeMultiState                   edgeMultiStateType
	edgeMultiIPotential              FrameIndex
	edgeMultiILastInspected          FrameIndex

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
	tf := segment.firstFramenum + FrameIndex(i)
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

func (dsp *DataStreamProcessor) edgeMultiTriggerComputeAppend(records []*DataRecord) []*DataRecord {
	if !dsp.EdgeTrigger {
		return records
	}
	segment := &dsp.stream.DataSegment
	raw := segment.rawData
	ndata := len(raw)

	var triggerInds []int
	var iPotential, iLast, iFirst int
	iPotential = int(dsp.edgeMultiIPotential - segment.firstFramenum)
	iLast = ndata + dsp.NPresamples - dsp.NSamples
	if dsp.edgeMultiState == verifying {
		iFirst = int(dsp.edgeMultiILastInspected-segment.firstFramenum) + 1
	} else {
		iFirst = dsp.NPresamples + 1 // +1 to account for maximum shift from kink model
	}
	// fmt.Println()
	// fmt.Println("dsp.channelIndex", dsp.channelIndex, "segment.firstFramenum", segment.firstFramenum, "dsp.edgeMultiIPotential", dsp.edgeMultiIPotential)
	// fmt.Println("dsp.edgeMultiState", dsp.edgeMultiState, "dsp.edgeMultiILastInspected", dsp.edgeMultiILastInspected, "dsp.EdgeMultiVerifyNMonotone", dsp.EdgeMultiVerifyNMonotone)
	// fmt.Println("iPotential", iPotential, "iFirst", iFirst, "iLast", iLast, "len(raw)", len(raw), "dsp.NPresamples", dsp.NPresamples)
	if dsp.EdgeMultiVerifyNMonotone+3 > dsp.NSamples-dsp.NPresamples {
		panic(fmt.Sprintf("%v %v %v", dsp.EdgeMultiVerifyNMonotone, dsp.NSamples, dsp.NPresamples))
	}
	for i := iFirst; i <= iLast; i++ {
		// fmt.Printf("i=%v, i_frame=%v\n", i, i+int(segment.firstFramenum))
		switch dsp.edgeMultiState {
		case searching:
			diff := int32(raw[i]) - int32(raw[i-1])
			if (dsp.EdgeRising && diff >= dsp.EdgeLevel) ||
				(dsp.EdgeFalling && diff <= -dsp.EdgeLevel) {
				iPotential = i
				dsp.edgeMultiState = verifying
				// fmt.Println("increase")
			}
		case verifying:
			// now we have a potenial trigger
			// require following samples to each be greater than the last
			if raw[i] <= raw[i-1] { // here we observe a decrease
				// fmt.Println("decrease")
				nMonotone := i - iPotential
				if nMonotone >= dsp.EdgeMultiVerifyNMonotone {
					// now attempt to refine the trigger using the kink model
					xdataf := make([]float64, 10)
					ydataf := make([]float64, 10)
					for j := 0; j < 10; j++ {
						xdataf[j] = float64(j + iPotential - 6) // look at samples from i-6 to i+3
						ydataf[j] = float64(raw[j+iPotential-6])
					}
					ifit := float64(iPotential)
					kbest, _, err := kinkModelFit(xdataf, ydataf, []float64{ifit - 1, ifit - 0.5, ifit, ifit + 0.5, ifit + 1})
					var iTrigger int
					if err == nil {
						iTrigger = int(math.Ceil(kbest))
					} else {
						iTrigger = iPotential
					}
					triggerInds = append(triggerInds, iTrigger)
				}
				dsp.edgeMultiState = searching
			} else if i-iPotential >= dsp.NSamples { // if it has been rising for a whole record, that won't be a useful pulse, so just to back to searching
				dsp.edgeMultiState = searching
			}
		}
		// dsp.edgeMultiILastInspected = FrameIndex(i) + segment.firstFramenum // dont do in loop, left as the model for correctness
	}
	if iLast >= iFirst {
		dsp.edgeMultiILastInspected = FrameIndex(iLast) + segment.firstFramenum
	}
	dsp.edgeMultiIPotential = FrameIndex(iPotential) + segment.firstFramenum // dont need to condition this on EdgeMultiState because it only matters in state verifying
	// fmt.Println("triggerInds", triggerInds)
	var t, u, v, tFirst int
	// t index of previous trigger
	// u index of current trigger
	// v index of next trigger
	if dsp.LastEdgeMultiTrigger > 0 {
		tFirst = int(dsp.LastEdgeMultiTrigger - segment.firstFramenum)
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
		lastNPost := min(dsp.NSamples-dsp.NPresamples, int(u-t))
		npre := min(dsp.NPresamples, int(u-t-lastNPost))
		npost := min(dsp.NSamples-dsp.NPresamples, int(v-u))
		// fmt.Println("ch", dsp.channelIndex, "i", i, "npre", npre, "npost", npost, "t", t,
		// 	"u", u, "v", v, "lastNPost", lastNPost, "firstFramenum", segment.firstFramenum, "iLast", iLast)
		if dsp.EdgeMultiMakeShortRecords {
			newRecord := dsp.triggerAtSpecificSamples(segment, u, npre, npre+npost)
			records = append(records, newRecord)
		} else if dsp.EdgeMultiMakeContaminatedRecords {
			newRecord := dsp.triggerAtSpecificSamples(segment, u, dsp.NPresamples, dsp.NSamples)
			records = append(records, newRecord)
		} else if npre >= dsp.NPresamples && npre+npost >= dsp.NSamples {
			newRecord := dsp.triggerAtSpecificSamples(segment, u, dsp.NPresamples, dsp.NSamples)
			records = append(records, newRecord)
		}

	}

	if len(records) > 0 {
		dsp.LastEdgeMultiTrigger = records[len(records)-1].trigFrame
	}
	if iLast >= iFirst {
		if dsp.edgeMultiState == verifying {
			// fmt.Println("dsp.NPresamples", dsp.NPresamples, "ndata", ndata, "iPotential", iPotential)
			extraSamplesToKeep := (ndata - iPotential) + 1 // +1 to account for maximum shift from kink model
			if extraSamplesToKeep < 0 {
				panic("negaive extraSamplestoKeep")
			}
			dsp.stream.TrimKeepingN(dsp.NPresamples + extraSamplesToKeep)
		} else {
			dsp.stream.TrimKeepingN(dsp.NPresamples + 1) // +1 to account for maximum shift from kink model
		}
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
		nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFramenum
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
				nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFramenum
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
		nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFramenum
	}

	// dsp.LastTrigger stores the frame of the last trigger found by the most recent invocation of TriggerData
	nextPotentialTrig := dsp.LastTrigger - segment.firstFramenum + delaySamples
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
				nextFoundTrig = records[idxNextTrig].trigFrame - segment.firstFramenum
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
		trigList.keyFrame = dsp.stream.DataSegment.firstFramenum
		trigList.keyTime = dsp.stream.DataSegment.firstTime
		trigList.sampleRate = dsp.SampleRate
		trigList.lastFrameThatWillNeverTrigger = dsp.stream.DataSegment.firstFramenum +
			FrameIndex(len(dsp.stream.rawData)) - FrameIndex(dsp.NSamples-dsp.NPresamples)

		// Step 2b: send the primary list to the group trigger broker; receive the secondary list.
		dsp.Broker.PrimaryTrigs <- trigList
		secondaryTrigList := <-dsp.Broker.SecondaryTrigs[dsp.channelIndex]
		segment := &dsp.stream.DataSegment
		for _, st := range secondaryTrigList {
			secondaries = append(secondaries, dsp.triggerAt(segment, int(st-segment.firstFramenum)))
		}
		return
	}

	// Step 1: compute where the primary triggers are, one pass per trigger type.
	// Step 1a: compute all edge triggers on a first pass. Separated by at least 1 record length
	records = dsp.edgeTriggerComputeAppend(records)
	// Step 1b: compute all level triggers on a second pass. Only insert them
	// in the list of triggers if they are properly separated from the edge triggers.
	records = dsp.levelTriggerComputeAppend(records)

	// Step 1c: compute all auto triggers, wherever they fit in between edge+level.
	records = dsp.autoTriggerComputeAppend(records)

	// Step 1.5: note the last trigger for the next invocation of TriggerData
	if len(records) > 0 {
		dsp.LastTrigger = records[len(records)-1].trigFrame
	}

	// TODO Step 1d: compute all noise triggers, wherever they fit in between edge+level.
	//

	// Step 2: send the primary trigger list to the group trigger broker and await its
	// answer about when the secondary triggers are.

	// Step 2a: prepare the primary trigger list from the DataRecord list
	trigList := triggerList{channelIndex: dsp.channelIndex}
	trigList.frames = make([]FrameIndex, len(records))
	for i, r := range records {
		trigList.frames[i] = r.trigFrame
	}
	trigList.keyFrame = dsp.stream.DataSegment.firstFramenum
	trigList.keyTime = dsp.stream.DataSegment.firstTime
	trigList.sampleRate = dsp.SampleRate
	trigList.lastFrameThatWillNeverTrigger = dsp.stream.DataSegment.firstFramenum +
		FrameIndex(len(dsp.stream.rawData)) - FrameIndex(dsp.NSamples-dsp.NPresamples)

	// Step 2b: send the primary list to the group trigger broker; receive the secondary list.
	dsp.Broker.PrimaryTrigs <- trigList
	secondaryTrigList := <-dsp.Broker.SecondaryTrigs[dsp.channelIndex]
	segment := &dsp.stream.DataSegment
	for _, st := range secondaryTrigList {
		secondaries = append(secondaries, dsp.triggerAt(segment, int(st-segment.firstFramenum)))
	}

	// leave one full possible trigger in the stream
	// trigger algorithms should not inspect the last NSamples samples
	// fmt.Printf("Trimmed. %7d samples remain (requested %7d)\n", dsp.stream.TrimKeepingN(dsp.NSamples), dsp.NSamples)
	dsp.stream.TrimKeepingN(dsp.NSamples)
	return
}

// RecordSlice attaches the methods of sort.Interface to slices, sorting in increasing order.
type RecordSlice []*DataRecord

func (p RecordSlice) Len() int           { return len(p) }
func (p RecordSlice) Less(i, j int) bool { return p[i].trigFrame < p[j].trigFrame }
func (p RecordSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
