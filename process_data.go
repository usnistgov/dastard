package dastard

import (
	"fmt"
	"math"
	"sync"
	"time"

	"gonum.org/v1/gonum/mat"
)

// DataStreamProcessor contains all the state needed to decimate, trigger, write, and publish data.
type DataStreamProcessor struct {
	Channum              int
	Name                 string
	Broker               *TriggerBroker
	NSamples             int
	NPresamples          int
	SampleRate           float64
	LastTrigger          FrameIndex
	LastEdgeMultiTrigger FrameIndex
	stream               DataStream
	projectors           mat.Dense
	// realtime analysis is disable if projectors .IsZero
	// otherwise projectors must be size (nbases,NSamples)
	// such that projectors*data (data as a column vector) = modelCoefs
	basis mat.Dense
	// if not projectors.IsZero basis must be size
	// (NSamples, nbases) such that basis*modelCoefs = modeled_data
	DecimateState
	TriggerState
	DataPublisher
	changeMutex sync.Mutex // Don't change key data without locking this.
}

// RemoveProjectorsBasis calls .Reset on projectors and basis, which disables projections in analysis
func (dsp *DataStreamProcessor) RemoveProjectorsBasis() {
	dsp.projectors.Reset()
	dsp.basis.Reset()
}

// SetProjectorsBasis sets .projectors and .basis to the arguments, returns an error if the sizes are not right
func (dsp *DataStreamProcessor) SetProjectorsBasis(projectors mat.Dense, basis mat.Dense) error {
	rows, cols := projectors.Dims()
	nbases := rows
	dsp.changeMutex.Lock()
	defer dsp.changeMutex.Unlock()
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
	return nil
}

// NewDataStreamProcessor creates and initializes a new DataStreamProcessor.
func NewDataStreamProcessor(chnum int, broker *TriggerBroker) *DataStreamProcessor {
	data := make([]RawType, 0, 1024)
	framesPerSample := 1
	firstFrame := FrameIndex(0)
	firstTime := time.Now()
	period := time.Duration(1 * time.Millisecond) // TODO: figure out what this ought to be, or make an argument
	stream := NewDataStream(data, framesPerSample, firstFrame, firstTime, period)
	nsamp := 1024 // TODO: figure out what this ought to be, or make an argument
	npre := 256   // TODO: figure out what this ought to be, or make an argument
	dsp := DataStreamProcessor{Channum: chnum, Broker: broker,
		stream: *stream, NSamples: nsamp, NPresamples: npre,
	}
	dsp.LastTrigger = math.MinInt64 / 4 // far in the past, but not so far we can't subtract from it
	dsp.projectors.Reset()
	dsp.basis.Reset()
	dsp.triggerCounter = NewTriggerCounter(1000000000, time.Now().Nanosecond())
	// dsp.basis has zero value
	// dsp.projectors has zero value
	return &dsp
}

// DecimateState contains all the state needed to decimate data.
type DecimateState struct {
	DecimateLevel   int
	Decimate        bool
	DecimateAvgMode bool
}

// ConfigurePulseLengths sets this stream's pulse length and # of presamples.
func (dsp *DataStreamProcessor) ConfigurePulseLengths(nsamp, npre int) {
	if nsamp <= npre+1 || npre < 3 {
		return
	}
	dsp.changeMutex.Lock()
	defer dsp.changeMutex.Unlock()

	dsp.NSamples = nsamp
	dsp.NPresamples = npre
}

// ConfigureTrigger sets this stream's trigger state.
func (dsp *DataStreamProcessor) ConfigureTrigger(state TriggerState) {
	dsp.changeMutex.Lock()
	defer dsp.changeMutex.Unlock()

	dsp.TriggerState = state
}

// ProcessData drains the data channel and processes whatever is found there.
func (dsp *DataStreamProcessor) ProcessData(dataIn <-chan DataSegment) {
	for segment := range dataIn {
		dsp.processSegment(&segment)
	}
}

func (dsp *DataStreamProcessor) processSegment(segment *DataSegment) {
	dsp.changeMutex.Lock()
	defer dsp.changeMutex.Unlock()

	dsp.DecimateData(segment)
	dsp.stream.AppendSegment(segment)
	// records, secondaries := dsp.TriggerData()
	records, _ := dsp.TriggerData()
	dsp.AnalyzeData(records) // add analysis results to records in-place
	// TODO: dsp.WriteData(records)
	dsp.DataPublisher.PublishData(records) // publish and save data, when enabled
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
		for i := 0; i < Nout-1; i++ {
			val := float64(data[i*level])
			for j := 1; j < level; j++ {
				val += float64(data[j+i*level])
			}
			data[i] = RawType(val/float64(level) + 0.5)
		}
		val := float64(data[(Nout-1)*level])
		count := 1.0
		for j := (Nout-1)*level + 1; j < Nin; j++ {
			val += float64(data[j])
			count++
		}
		data[Nout-1] = RawType(val/count + 0.5)
	} else {
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
		var val float64
		for i := 0; i < rec.presamples; i++ {
			val += float64(rec.data[i])
		}
		ptm := val / float64(rec.presamples)

		max := rec.data[rec.presamples]
		var sum, sum2 float64
		for i := rec.presamples; i < len(rec.data); i++ {
			val = float64(rec.data[i])
			sum += val
			sum2 += val * val
			if rec.data[i] > max {
				max = rec.data[i]
			}
		}
		rec.pretrigMean = ptm
		rec.peakValue = float64(max) - rec.pretrigMean

		N := float64(len(rec.data) - rec.presamples)
		rec.pulseAverage = sum/N - ptm
		meanSquare := sum2/N - 2*ptm*(sum/N) + ptm*ptm
		rec.pulseRMS = math.Sqrt(meanSquare)
		if !dsp.projectors.IsZero() {
			rows, cols := dsp.projectors.Dims()
			nbases := rows
			if cols != len(rec.data) {
				panic("projections for variable length records not implemented")
			}
			projectors := &dsp.projectors
			basis := &dsp.basis

			dataVec := *mat.NewVecDense(len(rec.data), make([]float64, len(rec.data)))
			for i, v := range rec.data {
				dataVec.SetVec(i, float64(v))
			}
			modelCoefs.MulVec(projectors, &dataVec)
			modelFull.MulVec(basis, &modelCoefs)
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
