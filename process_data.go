package dastard

import (
	"fmt"
	"math"
	"time"

	"gonum.org/v1/gonum/mat"
)

// DataStreamProcessor contains all the state needed to decimate, trigger, write, and publish data.
type DataStreamProcessor struct {
	channelIndex         int
	ChannelNumber        int
	Name                 string
	Broker               *TriggerBroker
	NSamples             int
	NPresamples          int
	SampleRate           float64
	LastTrigger          FrameIndex
	LastEdgeMultiTrigger FrameIndex
	stream               DataStream
	lastTrigList         triggerList

	// Realtime analysis features. RT analysis is disabled if projectors.IsEmpty()
	// Otherwise projectors must be of size (nbases,NSamples)
	// such that projectors*data (data as a column vector) = modelCoefs
	// If not projectors.IsEmpty(), basis must be of size
	// (NSamples, nbases) such that basis*modelCoefs = modeled_data ≈ data
	projectors       *mat.Dense
	modelDescription string
	basis            *mat.Dense

	DecimateState
	TriggerState
	DataPublisher
}

// RemoveProjectorsBasis calls .Reset on projectors and basis, which disables projections in analysis
// Lock dsp.changeMutex before calling this function, it will not lock on its own.
func (dsp *DataStreamProcessor) removeProjectorsBasis() {
	dsp.projectors.Reset()
	dsp.basis.Reset()
	var s string
	dsp.modelDescription = s
}

// SetProjectorsBasis sets .projectors and .basis to the arguments, returns an error if the sizes are not right
func (dsp *DataStreamProcessor) SetProjectorsBasis(projectors *mat.Dense, basis *mat.Dense, modelDescription string) error {
	rows, cols := projectors.Dims()
	nbases := rows
	if dsp.NSamples != cols {
		return fmt.Errorf("projectors has wrong size, rows: %v, cols: %v, want cols: %v", rows, cols, dsp.NSamples)
	}
	brows, bcols := basis.Dims()
	if bcols != nbases {
		return fmt.Errorf("basis has wrong size, has cols: %v, want: %v", bcols, nbases)
	}
	if brows != dsp.NSamples {
		return fmt.Errorf("basis has wrong size, has rows: %v, want: %v", brows, dsp.NSamples)
	}
	dsp.projectors = projectors
	dsp.basis = basis
	dsp.modelDescription = modelDescription
	return nil
}

// HasProjectors return true if projectors are loaded
func (dsp *DataStreamProcessor) HasProjectors() bool {
	return (dsp.projectors != nil) && (!dsp.projectors.IsEmpty())
}

// NewDataStreamProcessor creates and initializes a new DataStreamProcessor.
func NewDataStreamProcessor(channelIndex int, broker *TriggerBroker, NPresamples int, NSamples int) *DataStreamProcessor {
	data := make([]RawType, 0, 1024)
	framesPerSample := 1
	firstFrame := FrameIndex(0)
	firstTime := time.Now()
	period := time.Duration(1 * time.Millisecond) // TODO: figure out what this ought to be, or make an argument
	stream := NewDataStream(data, framesPerSample, firstFrame, firstTime, period)
	dsp := DataStreamProcessor{channelIndex: channelIndex, Broker: broker,
		stream: *stream, NSamples: NSamples, NPresamples: NPresamples,
	}
	dsp.LastTrigger = math.MinInt64 / 4 // far in the past, but not so far we can't subtract from it
	dsp.projectors = &mat.Dense{}       // dsp.projectors is set to zero value
	dsp.basis = &mat.Dense{}            // dsp.basis is set to zero value
	dsp.EMTState.reset()                // set up edgeMulti in known state
	return &dsp
}

// DecimateState contains all the state needed to decimate data.
type DecimateState struct {
	DecimateLevel   int
	Decimate        bool
	DecimateAvgMode bool
}

// ConfigurePulseLengths sets this stream's pulse length and # of presamples.
// Also removes any existing projectors and basis.
func (dsp *DataStreamProcessor) ConfigurePulseLengths(nsamp, npre int) error {
	// if nsamp or npre is invalid, panic, do not silently ignore
	if dsp.NSamples != nsamp || dsp.NPresamples != npre {
		dsp.removeProjectorsBasis()
		dsp.EMTState.reset()
	}
	// we currently have two locations where we have nsamp and npre inside a dsp
	// we should fix that, but for now just keep them in sync
	dsp.NSamples = nsamp
	dsp.NPresamples = npre
	dsp.EMTState.nsamp = int32(nsamp)
	dsp.EMTState.npre = int32(npre)
	if dsp.EdgeMulti && !dsp.EMTState.valid() {
		return fmt.Errorf("dsp.EMTState in invalid")
	}
	dsp.EMTState.reset()
	return nil
}

// ConfigureTrigger sets this stream's trigger state.
func (dsp *DataStreamProcessor) ConfigureTrigger(state TriggerState) error {
	dsp.TriggerState = state
	dsp.LastTrigger = 0 // forget the Last Trigger, so that all channels will auto trigger
	// at the same starting point when you send new trigger settings

	// we currently have two locations where we have nsamp and npre inside a dsp
	// we should fix that, but for now just keep them in sync	dsp.EMTState.nsamp = int32(dsp.NSamples)
	dsp.EMTState.nsamp = int32(dsp.NSamples)
	dsp.EMTState.npre = int32(dsp.NPresamples)
	if dsp.EdgeMulti && !dsp.EMTState.valid() {
		return fmt.Errorf("dsp.EMTState in invalid")
	}
	dsp.EMTState.reset()
	return nil
}

func (dsp *DataStreamProcessor) processSegment(segment *DataSegment) {
	dsp.DecimateData(segment)
	dsp.stream.AppendSegment(segment)
	primaryRecords := dsp.TriggerData()
	dsp.AnalyzeData(primaryRecords)                                       // add analysis results to records in-place
	if err := dsp.DataPublisher.PublishData(primaryRecords); err != nil { // publish and save data, when enabled
		panic(err)
	}
}

func (dsp *DataStreamProcessor) processSecondaries(secondaryFrames []FrameIndex) {
	secondaryRecords := dsp.TriggerDataSecondary(secondaryFrames)
	dsp.AnalyzeData(secondaryRecords)                                       // add analysis results to records in-place
	if err := dsp.DataPublisher.PublishData(secondaryRecords); err != nil { // publish and save data, when enabled
		panic(err)
	}
}

// DecimateData decimates data in-place.
func (dsp *DataStreamProcessor) DecimateData(segment *DataSegment) {
	if !dsp.Decimate || dsp.DecimateLevel <= 1 {
		return
	}
	data := segment.rawData
	Nin := len(data)
	Nout := (Nin - 1 + dsp.DecimateLevel) / dsp.DecimateLevel
	if dsp.DecimateAvgMode {
		level := dsp.DecimateLevel
		cdata := make([]float64, Nout)
		if segment.signed {
			for i := 0; i < Nin; i++ {
				j := i / level
				cdata[j] += float64(int16(data[i]))
			}
		} else {
			for i := 0; i < Nin; i++ {
				j := i / level
				cdata[j] += float64(data[i])
			}
		}
		if Nout*dsp.DecimateLevel < Nin {
			extra := Nin % level
			cdata[Nout-1] *= float64(level) / float64(extra)
		}

		if segment.signed {
			// Trick for rounding to int16: don't let any numbers be negative,
			// because float->int is a truncation operation. If we removed the
			// +65536 in this loop, then 0 would become an unwanted "rounding attractor".
			for i := 0; i < Nout; i++ {
				data[i] = RawType(int16(cdata[i]/float64(level) + 65536 + 0.5))
			}

		} else {
			for i := 0; i < Nout; i++ {
				data[i] = RawType(cdata[i]/float64(level) + 0.5)
			}
		}

	} else {
		// Decimate by dropping data
		for i := 0; i < Nout; i++ {
			data[i] = data[i*dsp.DecimateLevel]
		}
	}
	segment.rawData = data[:Nout]
	segment.framesPerSample *= dsp.DecimateLevel
	return
}

// AnalyzeData computes pulse-analysis values in-place for all elements of a
// slice of DataRecord values.
func (dsp *DataStreamProcessor) AnalyzeData(records []*DataRecord) {
	var modelCoefs mat.VecDense
	var modelFull mat.VecDense
	var residual mat.VecDense
	for _, rec := range records {
		dataVec := *mat.NewVecDense(len(rec.data), make([]float64, len(rec.data)))
		if rec.signed {
			for i, v := range rec.data {
				dataVec.SetVec(i, float64(int16(v)))
			}
		} else {
			for i, v := range rec.data {
				dataVec.SetVec(i, float64(v))
			}
		}

		var val float64        // used to calculate pretrigger mean, then reused in the next loop
		var valPTDelta float64 // used to calculate pretrigger delta
		// slope = dot(x,y.-y[0])/z where z = dot(x,x) and x = [0, 1, 2, ..., N-1]/(N-1), where .-y[0] is elementwise subtraction of the first element
		npre := rec.presamples
		d0 := dataVec.AtVec(0)
		xmean := float64(npre-1) * 0.5
		for i := 0; i < npre; i++ {
			val += dataVec.AtVec(i)
			valPTDelta += (dataVec.AtVec(i) - d0) * (float64(i) - xmean)
		}
		ptm := val / float64(npre)
		rec.pretrigMean = ptm
		if npre <= 1 {
			rec.pretrigDelta = math.NaN()
		} else {
			rec.pretrigDelta = valPTDelta * 12.0 / float64(npre*(npre+1))
		}

		max := ptm
		var sum, sum2 float64
		for i := rec.presamples; i < len(rec.data); i++ {
			val = dataVec.AtVec(i)
			sum += val
			sum2 += val * val
			if val > max {
				max = val
			}
		}
		rec.peakValue = max - ptm

		N := float64(len(rec.data) - rec.presamples)
		rec.pulseAverage = sum/N - ptm
		meanSquare := sum2/N - 2*ptm*(sum/N) + ptm*ptm
		rec.pulseRMS = math.Sqrt(meanSquare)
		if dsp.HasProjectors() {
			rows, cols := dsp.projectors.Dims()
			nbases := rows
			if cols != len(rec.data) {
				panic("projections for variable length records not implemented")
			}

			modelCoefs.MulVec(dsp.projectors, &dataVec)
			modelFull.MulVec(dsp.basis, &modelCoefs)
			residual.SubVec(&dataVec, &modelFull)

			// copy modelCoefs into rec.modelCoefs
			rec.modelCoefs = make([]float64, nbases)
			mat.Col(rec.modelCoefs, 0, &modelCoefs)

			// calculate and asign StdDev
			residualSlice := make([]float64, len(rec.data))
			mat.Col(residualSlice, 0, &residual)
			rec.residualStdDev = stdDev(residualSlice)
		}
	}
}

// TrimStream trims a DataStreamProcessor's stream to contain only one record's worth of old
// samples. That should more than suffice to extract triggers from future data.
func (dsp *DataStreamProcessor) TrimStream() {
	// Leave one full possible trigger in the stream, because trigger algorithms
	// should not inspect the last NSamples samples
	dsp.stream.TrimKeepingN(dsp.NSamples)
}

// return the uncorrected std deviation of a float slice
func stdDev(a []float64) float64 {
	if len(a) == 0 {
		return math.NaN()
	}
	s, s2 := 0.0, 0.0
	for _, v := range a {
		s += v
	}
	mean := s / float64(len(a))
	for _, v := range a {
		x := v - mean
		s2 += x * x
	}
	return math.Sqrt(s2 / float64(len(a)))
}
