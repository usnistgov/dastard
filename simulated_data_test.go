package dastard

import (
	"fmt"
	"io"
	"math"
	"testing"
)

// TestTriangle checks that TriangleSource works as expected
func TestTriangle(t *testing.T) {
	nchan, rate, min, max := 4, 10000.0, 10, 15
	ts := NewTriangleSource(nchan, rate, RawType(min), RawType(max))
	ts.Configure()
	ds := DataSource(ts)

	abort := make(chan struct{})
	outputs := ds.Outputs()
	if len(outputs) != nchan {
		t.Errorf("TriangleSource.Ouputs() returns %d channels, want %d", len(outputs), nchan)
	}
	ds.Sample()
	ds.Start()
	ds.BlockingRead(abort)
	n := max - min
	for i, ch := range outputs {
		segment := <-ch
		data := segment.rawData
		if len(data) != 2*n {
			t.Errorf("TriangleSource output %d is length %d, expect %d", i, len(data), 2*n)
		}
		for j := 0; j < n; j++ {
			if data[j] != RawType(min+j) {
				t.Errorf("TriangleSource output %d has [%d]=%d, expect %d", i, j, data[j], min+j)
			}
			if data[j+n] != RawType(max-j) {
				t.Errorf("TriangleSource output %d has [%d]=%d, expect %d", i, j+n, data[j+n], max-j)
			}
		}
	}

	ds.Stop()

	// Now try a blocking read with abort.
	ds.Start()
	ds.BlockingRead(abort)
	close(abort)
	err := ds.BlockingRead(abort)
	if err != io.EOF {
		t.Errorf("TriangleSource did not return EOF on aborted BlockingRead")
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
		fmt.Printf("%v\n", rec)
	}

	expectAvg := 5.0
	if rec.pulseAverage != expectAvg {
		t.Errorf("Pulse average = %f, want %f", rec.pulseAverage, expectAvg)
		fmt.Printf("%v\n", rec)
	}

	expectMax := 10.0
	if rec.peakValue != expectMax {
		t.Errorf("Peak value = %f, want %f", rec.peakValue, expectMax)
		fmt.Printf("%v\n", rec)
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
		fmt.Printf("%v\n", rec)
	}
}
