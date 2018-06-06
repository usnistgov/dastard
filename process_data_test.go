package dastard

import (
	"fmt"
	"math"
	"testing"

	"gonum.org/v1/gonum/mat"
)

func matPrint(X mat.Matrix, t *testing.T) {
	fa := mat.Formatted(X, mat.Prefix(""), mat.Squeeze())
	t.Logf("%v\n", fa)
}

func TestStdDev(t *testing.T) {
	s := []float64{1.0, 1.0, 1.0}
	sStdDev := stdDev(s)
	if sStdDev != 0 {
		t.Errorf("stdDev returned incorrect result")
	}
	z := []float64{-1.0, 1.0}
	zStdDev := stdDev(z)
	if zStdDev != 1.0 {
		t.Errorf("stdDev returned incorrect result")
	}
}

// TestAnalyze tests the DataChannel.AnalyzeData computations on a very simple "pulse".
func TestAnalyze(t *testing.T) {
	d := []RawType{10, 10, 10, 10, 15, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10}
	rec := &DataRecord{data: d, presamples: 4}
	records := []*DataRecord{rec}

	dsp := &DataStreamProcessor{NPresamples: 4, NSamples: len(d)}
	dsp.AnalyzeData(records)

	expect := RTExpect{
		ResidualStdDev: 0,
		ModelCoefs:     nil,
		PTM:            10.0, Avg: 5.0, Max: 10.0, RMS: 5.84522597225006}
	testAnalyzeCheck(t, rec, expect, "Analyze A")
}

type RTExpect struct {
	Max            float64
	RMS            float64
	Avg            float64
	PTM            float64
	ModelCoefs     []float64
	ResidualStdDev float64
}

//TestAnalyzeRealtime tests the DataChannel.AnalyzeData computations on a very simple "pulse".
func TestAnalyzeRealtimeBases(t *testing.T) {
	d := []RawType{1, 2, 3, 4}
	rec := &DataRecord{data: d, presamples: 1}
	records := []*DataRecord{rec}

	dsp := &DataStreamProcessor{NPresamples: 1, NSamples: len(d)}

	// assign the projectors and basis
	nbases := 3
	projectors3 := mat.NewDense(nbases, dsp.NSamples,
		[]float64{1, 0, 0, 0,
			0, 1, 0, 0,
			0, 0, 1, 0})
	basis3 := mat.NewDense(dsp.NSamples, nbases,
		[]float64{1, 0, 0,
			0, 1, 0,
			0, 0, 1,
			0, 0, 0})
	dsp.SetProjectorsBasis(*projectors3, *basis3)
	dsp.AnalyzeData(records)

	// residual should be [0,0,0,4]
	// the uncorrected stdDeviation of this is sqrt(((0-1)^2+(0-1)^2+(0-1)^2+(4-1)^2)/4)
	expect := RTExpect{
		ResidualStdDev: 1.7320508075688772,
		ModelCoefs:     []float64{1, 2, 3},
		PTM:            1.0, Avg: 2.0, Max: 3.0, RMS: 2.1602468994692865}
	testAnalyzeCheck(t, rec, expect, "Realtime A: 3 Bases, no Trunc")

	// assign the projectors and basis
	nbases = 1
	projectors1 := mat.NewDense(nbases, dsp.NSamples,
		[]float64{1, 0, 0, 0})
	basis1 := mat.NewDense(dsp.NSamples, nbases,
		[]float64{1,
			0,
			0,
			0})
	dsp.SetProjectorsBasis(*projectors1, *basis1)
	dsp.AnalyzeData(records)
	// residual should be [0,2,3,4]
	expect = RTExpect{
		ResidualStdDev: 1.479019945774904,
		ModelCoefs:     []float64{1},
		PTM:            1.0, Avg: 2.0, Max: 3.0, RMS: 2.1602468994692865}
	testAnalyzeCheck(t, rec, expect, "Realtime B: 1 Bases, no Trunc")

	d = []RawType{1, 2, 3}
	rec = &DataRecord{data: d, presamples: 1}
	records = []*DataRecord{rec}
	dsp.RemoveProjectorsBasis()
	dsp.AnalyzeData(records)
	expect = RTExpect{
		ResidualStdDev: 0,
		ModelCoefs:     nil,
		PTM:            1.0, Avg: 1.5, Max: 2.0, RMS: 1.5811388300841898}
	testAnalyzeCheck(t, rec, expect, "Realtime C: 3 Bases, record truncated at end")

	d = []RawType{1, 2, 3}
	rec = &DataRecord{data: d, presamples: 0}
	records = []*DataRecord{rec}
	dsp.RemoveProjectorsBasis()
	dsp.AnalyzeData(records)
	expect = RTExpect{
		ResidualStdDev: 0,
		ModelCoefs:     nil,
		PTM:            math.NaN(), Avg: math.NaN(), Max: math.NaN(), RMS: math.NaN()}
	testAnalyzeCheck(t, rec, expect, "Realtime D: 3 Bases, record truncated at front")

}

func testAnalyzeCheck(t *testing.T, rec *DataRecord, expect RTExpect, name string) {
	if rec.residualStdDev != expect.ResidualStdDev {
		t.Errorf("%v: ResidualStdDev = %v, want %v", name, rec.residualStdDev, expect.ResidualStdDev)
		t.Logf("%v\n", rec)
	}
	modelCoefsCorrect := true
	for i, v := range expect.ModelCoefs {
		if i >= len(rec.modelCoefs) || v != rec.modelCoefs[i] {
			modelCoefsCorrect = false
		}
	}
	if !modelCoefsCorrect {
		t.Log(name, "\nrec.modelCoefs", rec.modelCoefs)
		t.Error("should equal expectModelCoefs", expect.ModelCoefs)
	}
	if rec.pretrigMean != expect.PTM && !(math.IsNaN(rec.pretrigMean) && math.IsNaN(expect.PTM)) {
		t.Errorf("Pretrigger mean = %v, want %v", rec.pretrigMean, expect.PTM)
		t.Logf("%v\n", rec)
	}
	if rec.pulseAverage != expect.Avg && !(math.IsNaN(rec.pulseAverage) && math.IsNaN(expect.Avg)) {
		t.Errorf("%v, Pulse average = %v, want %v", name, rec.pulseAverage, expect.Avg)
		t.Logf("%v\n", rec)
	}
	if rec.peakValue != expect.Max && !(math.IsNaN(rec.peakValue) && math.IsNaN(expect.Max)) {
		t.Errorf("%v, Peak value = %v, want %v", name, rec.peakValue, expect.Max)
		t.Logf("%v\n", rec)
	}
	if rec.pulseRMS != expect.RMS && !(math.IsNaN(rec.pulseRMS) && math.IsNaN(expect.RMS)) {
		t.Errorf("%v, Pulse RMS = %v, want %v", name, rec.pulseRMS, expect.RMS)
		t.Logf("%v\n", rec)
	}
}

func BenchmarkAnalyze(b *testing.B) {
	benchmarks := []struct {
		nsamples    int
		npresamples int
		nbases      int
	}{
		{1000, 100, 0},
		{1000, 100, 1},
		{1000, 100, 2},
		{1000, 100, 6},
		{100, 10, 0},
		{100, 10, 1},
		{100, 10, 2},
		{100, 10, 6},
	}

	for _, bm := range benchmarks {
		name := fmt.Sprintf("%vsamp,%vpre,%vbases", bm.nsamples, bm.npresamples, bm.nbases)
		b.Run(name, func(b *testing.B) {
			var d []RawType
			records := make([]*DataRecord, b.N)
			for i := range records {
				d = make([]RawType, bm.nsamples)
				for i := range d {
					d[i] = RawType(i)
				}
				records[i] = &DataRecord{data: d}
			}
			dsp := &DataStreamProcessor{NPresamples: bm.npresamples, NSamples: bm.nsamples}
			// assign the projectors and basis
			if bm.nbases > 0 {
				projectors := mat.NewDense(bm.nbases, bm.nsamples, make([]float64, bm.nbases*bm.nsamples))
				basis := mat.NewDense(bm.nsamples, bm.nbases, make([]float64, bm.nbases*bm.nsamples))
				dsp.SetProjectorsBasis(*projectors, *basis)
			}
			b.ResetTimer()
			dsp.AnalyzeData(records)
		})
	}
}

func BenchmarkMatSub(b *testing.B) {
	benchmarks := []struct {
		rA int
		cA int
	}{
		{1, 1000},
		{2, 1000},
		{3, 1000},
		{1000, 1},
		{1000, 2},
		{1000, 3},
		{1, 100},
		{2, 100},
		{3, 100},
		{100, 1},
		{100, 2},
		{100, 3},
	}

	for _, bm := range benchmarks {
		name := fmt.Sprintf("(%v,%v)-(%v,%v)", bm.rA, bm.cA, bm.rA, bm.cA)
		b.Run(name, func(b *testing.B) {
			A := mat.NewDense(bm.rA, bm.cA, make([]float64, bm.rA*bm.cA))
			B := mat.NewDense(bm.rA, bm.cA, make([]float64, bm.rA*bm.cA))
			var result mat.Dense
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result.Sub(A, B)
			}

		})
	}
}

func BenchmarkMatMul(b *testing.B) {
	benchmarks := []struct {
		rA int
		cA int
		rB int
		cB int
	}{
		{1, 1000, 1000, 1},
		{2, 1000, 1000, 1},
		{3, 1000, 1000, 1},
		{1000, 10, 10, 1},
		{1000, 3, 3, 1},
		{1000, 2, 2, 1},
		{1000, 1, 1, 1},
		{100, 3, 3, 1},
		{100, 2, 2, 1},
		{100, 1, 1, 1},
	}

	for _, bm := range benchmarks {
		name := fmt.Sprintf("(%v,%v)*(%v,%v)", bm.rA, bm.cA, bm.rB, bm.cB)
		b.Run(name, func(b *testing.B) {
			A := mat.NewDense(bm.rA, bm.cA, make([]float64, bm.rA*bm.cA))
			B := mat.NewDense(bm.rB, bm.cB, make([]float64, bm.rB*bm.cB))
			var result mat.Dense
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result.Mul(A, B)
			}

		})
	}
	for _, bm := range benchmarks {
		name := fmt.Sprintf("MulVec(%v,%v)*(%v,%v)", bm.rA, bm.cA, bm.rB, bm.cB)
		b.Run(name, func(b *testing.B) {
			if bm.cB != 1 {
				panic("cB should be 1")
			}
			A := mat.NewDense(bm.rA, bm.cA, make([]float64, bm.rA*bm.cA))
			B := mat.NewVecDense(bm.rB, make([]float64, bm.rB*bm.cB))
			var result mat.VecDense
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result.MulVec(A, B)
			}

		})
	}
}
