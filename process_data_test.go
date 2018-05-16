package dastard

import (
	"math"
	"testing"

	"gonum.org/v1/gonum/mat"
)

func matPrint(X mat.Matrix, t *testing.T) {
	fa := mat.Formatted(X, mat.Prefix(""), mat.Squeeze())
	t.Logf("%v\n", fa)
}

// TestStdDev checks that DataSegment works as expected
func TestStdDev(t *testing.T) {
	s := []float64{1.0, 1.0, 1.0}
	s_stdDev := stdDev(s)
	if s_stdDev != 0 {
		t.Errorf("stdDev returned incorrect result")
	}
	z := []float64{-1.0, 1.0}
	z_stdDev := stdDev(z)
	if z_stdDev != 1.0 {
		t.Errorf("stdDev returned incorrect result")
	}
}

// TestAnalyze tests the DataChannel.AnalyzeData computations on a very simple "pulse".
func TestAnalyze(t *testing.T) {
	d := []RawType{10, 10, 10, 10, 15, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10}
	rec := &DataRecord{data: d}
	records := []*DataRecord{rec}

	dsp := &DataStreamProcessor{NPresamples: 4, NSamples: len(d)}
	dsp.AnalyzeData(records)

	expectPTM := 10.0
	if rec.pretrigMean != expectPTM {
		t.Errorf("Pretrigger mean = %f, want %f", rec.pretrigMean, expectPTM)
		t.Logf("%v\n", rec)
	}

	expectAvg := 5.0
	if rec.pulseAverage != expectAvg {
		t.Errorf("Pulse average = %f, want %f", rec.pulseAverage, expectAvg)
		t.Logf("%v\n", rec)
	}

	expectMax := 10.0
	if rec.peakValue != expectMax {
		t.Errorf("Peak value = %f, want %f", rec.peakValue, expectMax)
		t.Logf("%v\n", rec)
	}

	expectRMS := 0.0
	for i := 4; i < len(d); i++ {
		diff := float64(d[i]) - expectPTM
		expectRMS += diff * diff
	}
	expectRMS /= float64(len(d) - 4)
	expectRMS = math.Sqrt(expectRMS)
	if math.Abs(rec.pulseRMS-expectRMS) > 1e-8 {
		t.Errorf("Pulse RMS = %f, want %f to 8 digits", rec.pulseRMS, expectRMS)
		t.Logf("%v\n", rec)
	}

	// the realtime analysis did not run, so we should get the Zero value
	expectResidualStdDev := 0.0
	if rec.residualStdDev != expectResidualStdDev {
		t.Errorf("ResidualStdDev mean = %f, want %f", rec.residualStdDev, expectResidualStdDev)
		t.Logf("%v\n", rec)
	}

	// the realtime analysis did not run, so we should get the Zero value
	if rec.modelCoefs != nil {
		t.Log("rec.modelCoefs should have Zero Value, instead has", rec.modelCoefs)
		t.Logf("%v\n", rec)
		t.Fail()
	}
}

// TestAnalyzeRealtime tests the DataChannel.AnalyzeData computations on a very simple "pulse".
func TestAnalyzeRealtime(t *testing.T) {
	d := []RawType{1, 2, 3, 4}
	rec := &DataRecord{data: d}
	records := []*DataRecord{rec}

	dsp := &DataStreamProcessor{NPresamples: 1, NSamples: len(d)}

	// assign the projectors and basis
	nbases := 3
	projectors := mat.NewDense(nbases, dsp.NSamples,
		[]float64{1, 0, 0, 0,
			0, 1, 0, 0,
			0, 0, 1, 0})
	basis := mat.NewDense(dsp.NSamples, nbases,
		[]float64{1, 0, 0,
			0, 1, 0,
			0, 0, 1,
			0, 0, 0})
	dsp.SetProjectorsBasis(*projectors, *basis)
	dsp.AnalyzeData(records)

	if false {
		t.Log("projectors")
		matPrint(&dsp.projectors, t)
		t.Log(dsp.projectors.Dims())
		t.Log("basis")
		matPrint(&dsp.basis, t)
		t.Log(dsp.basis.Dims())
		t.Log("modelCoefs", rec.modelCoefs)
		t.Log("residualStd", rec.residualStdDev)
	}

	// residual should be [0,0,0,4]
	// the corrected stdDeviation of this is sqrt(((0-1)^2+(0-1)^2+(0-1)^2+(4-1)^2)/4)
	expectResidualStdDev := 1.7320508075688772
	if rec.residualStdDev != expectResidualStdDev {
		t.Errorf("ResidualStdDev mean = %f, want %f", rec.residualStdDev, expectResidualStdDev)
		t.Logf("%v\n", rec)
	}

	expectModelCoefs := []float64{1, 2, 3}
	modelCoefsCorrect := true
	for i, v := range expectModelCoefs {
		if v != rec.modelCoefs[i] {
			modelCoefsCorrect = false
		}
	}

	if !modelCoefsCorrect {
		t.Log("rec.modelCoefs", rec.modelCoefs)
		t.Log("should equal expectModelCoefs", expectModelCoefs)
		t.Fail()
	}

	expectPTM := 1.0
	if rec.pretrigMean != expectPTM {
		t.Errorf("Pretrigger mean = %f, want %f", rec.pretrigMean, expectPTM)
		t.Logf("%v\n", rec)
	}

	expectAvg := 2.0
	if rec.pulseAverage != expectAvg {
		t.Errorf("Pulse average = %f, want %f", rec.pulseAverage, expectAvg)
		t.Logf("%v\n", rec)
	}

	expectMax := 3.0
	if rec.peakValue != expectMax {
		t.Errorf("Peak value = %f, want %f", rec.peakValue, expectMax)
		t.Logf("%v\n", rec)
	}

	expectRMS := 0.0
	for i := 4; i < len(d); i++ {
		diff := float64(d[i]) - expectPTM
		expectRMS += diff * diff
	}
	expectRMS /= float64(len(d) - 4)
	expectRMS = math.Sqrt(expectRMS)
	if math.Abs(rec.pulseRMS-expectRMS) > 1e-8 {
		t.Errorf("Pulse RMS = %f, want %f to 8 digits", rec.pulseRMS, expectRMS)
		t.Logf("%v\n", rec)
	}
}

func benchmarkAnalyzeData(nsamples int, npresamples int, nbases int, b *testing.B) {

	d := make([]RawType, nsamples)
	for i, _ := range d {
		d[i] = RawType(i)
	}
	records := make([]*DataRecord, b.N)
	for i, _ := range records {
		records[i] = &DataRecord{data: d}
	}
	dsp := &DataStreamProcessor{NPresamples: npresamples, NSamples: nsamples}
	// assign the projectors and basis
	if nbases > 0 {
		projectors := mat.NewDense(nbases, nsamples, make([]float64, nbases*nsamples))
		basis := mat.NewDense(nsamples, nbases, make([]float64, nbases*nsamples))
		dsp.SetProjectorsBasis(*projectors, *basis)
	}
	b.ResetTimer()
	dsp.AnalyzeData(records)
}

func BenchmarkAnalyzeData1000Samples0Bases(b *testing.B) {
	benchmarkAnalyzeData(1000, 100, 0, b)
}
func BenchmarkAnalyzeData1000Samples3Bases(b *testing.B) {
	benchmarkAnalyzeData(1000, 100, 3, b)
}
func BenchmarkAnalyzeData1000Samples6Bases(b *testing.B) {
	benchmarkAnalyzeData(1000, 100, 6, b)
}
func BenchmarkAnalyzeData100Samples0Bases(b *testing.B) {
	benchmarkAnalyzeData(1000, 100, 0, b)
}
func BenchmarkAnalyzeData100Samples3Bases(b *testing.B) {
	benchmarkAnalyzeData(1000, 100, 3, b)
}
func BenchmarkAnalyzeData100Samples6Bases(b *testing.B) {
	benchmarkAnalyzeData(1000, 100, 6, b)
}
