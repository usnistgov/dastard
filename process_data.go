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
	Channum     int
	Publisher   chan<- []*DataRecord
	Broker      *TriggerBroker
	NSamples    int
	NPresamples int
	SampleRate  float64
	LastTrigger FrameIndex
	stream      DataStream
	projectors  mat.Dense
	// realtime analysis is disable if projectors .IsZero
	// otherwise projectors must be size (nbases,NSamples)
	// such that projectors*data (data as a column vector) = modelCoefs
	basis mat.Dense
	// if not projectors.IsZero basis must be size
	// (NSamples, nbases) such that basis*modelCoefs = modeled_data
	DecimateState
	TriggerState
	changeMutex sync.Mutex // Don't change key data without locking this.
}

func (dsp *DataStreamProcessor) SetProjectorsBasis(projectors mat.Dense, basis mat.Dense) {
	fmt.Println("setting")
	rows, cols := projectors.Dims()
	nbases := rows
	if dsp.NSamples != cols {
		fmt.Println("projectors has wrong size, rows: ", rows, " cols: ", cols)
		fmt.Println("should have cols: ", dsp.NSamples)
		panic("")
	}
	brows, bcols := basis.Dims()
	if bcols != nbases {
		fmt.Println("basis has wrong size, has cols: ", bcols, "should have cols: ", nbases)
		panic("")
	}
	if brows != dsp.NSamples {
		fmt.Println("basis has wrong size, has rows: ", brows, "should have rows: ", dsp.NSamples)
		panic("")
	}
	dsp.projectors = projectors
	dsp.basis = basis
}

// NewDataStreamProcessor creates and initializes a new DataStreamProcessor.
func NewDataStreamProcessor(channum int, publisher chan<- []*DataRecord,
	broker *TriggerBroker) *DataStreamProcessor {
	data := make([]RawType, 0, 1024)
	framesPerSample := 1
	firstFrame := FrameIndex(0)
	firstTime := time.Now()
	period := time.Duration(1 * time.Millisecond) // TODO: figure out what this ought to be, or make an argument
	stream := NewDataStream(data, framesPerSample, firstFrame, firstTime, period)
	nsamp := 1024 // TODO: figure out what this ought to be, or make an argument
	npre := 256   // TODO: figure out what this ought to be, or make an argument
	dsp := DataStreamProcessor{Channum: channum, Publisher: publisher, Broker: broker,
		stream: *stream, NSamples: nsamp, NPresamples: npre,
	}
	dsp.LastTrigger = math.MinInt64 / 4 // far in the past, but not so far we can't subtract from it
	dsp.projectors.Reset()
	dsp.basis.Reset()
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

	// TODO:  Noise info.
	// TODO: group source/rx info.
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
	records, secondaries := dsp.TriggerData()
	if len(records)+len(secondaries) > 0 {
		// fmt.Printf("Chan %d Found %d triggered records, %d secondary records.\n",
		// 	dsp.Channum, len(records), len(secondaries))
	}
	dsp.AnalyzeData(records) // add analysis results to records in-place
	// TODO: dsp.WriteData(records)
	dsp.PublishData(records)
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
	for _, rec := range records {
		var val float64
		for i := 0; i < dsp.NPresamples; i++ {
			val += float64(rec.data[i])
		}
		ptm := val / float64(dsp.NPresamples)

		max := rec.data[dsp.NPresamples]
		var sum, sum2 float64
		for i := dsp.NPresamples; i < dsp.NSamples; i++ {
			val = float64(rec.data[i])
			sum += val
			sum2 += val * val
			if rec.data[i] > max {
				max = rec.data[i]
			}
		}
		rec.pretrigMean = ptm
		rec.peakValue = float64(max) - rec.pretrigMean

		N := float64(dsp.NSamples - dsp.NPresamples)
		rec.pulseAverage = sum/N - ptm
		meanSquare := sum2/N - 2*ptm*(sum/N) + ptm*ptm
		rec.pulseRMS = math.Sqrt(meanSquare)
		if dsp.projectors.IsZero() {
		} else {
			rows, cols := dsp.projectors.Dims()
			nbases := rows
			fmt.Println("running analysis", rows, cols)
			if cols != len(rec.data) {
				panic("wrong size")
			}
			var modelCoefs mat.Dense
			var modelFull mat.Dense
			var residual mat.Dense
			data64 := convertSliceRawTypeToFloat64(rec.data)
			dataMat := mat.NewDense(len(rec.data), 1, data64)
			modelCoefs.Product(&dsp.projectors, dataMat)
			modelFull.Product(&dsp.basis, &modelCoefs)
			residual.Sub(dataMat, &modelFull)
			mrow, mcol := modelCoefs.Dims()
			rrow, rcol := residual.Dims()
			fmt.Println("modelCoefs", mrow, mcol)
			fmt.Println("residual", rrow, rcol)

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

// PublishData sends the slice of DataRecords to be published.
func (dsp *DataStreamProcessor) PublishData(records []*DataRecord) {
	dsp.Publisher <- records
}

func convertSliceRawTypeToFloat64(ar []RawType) []float64 {
	newar := make([]float64, len(ar))
	var v RawType
	var i int
	for i, v = range ar {
		newar[i] = float64(v)
	}
	return newar
}

func stdDev(a []float64) float64 {
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

// func modelCoefsToFloat64Slice(src *mat.Dense) []float64 {
// 	rows,cols := src.Dims()
// 	newar := make([]float64, rows)
// 	for i range rows {
// 		newar[i] = src.get
// 	}
// }
