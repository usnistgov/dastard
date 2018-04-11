package main

import (
	"fmt"
	"io"
	"math"
	"testing"
)

// TestTriangle checks that TriangleSource works as expected
func TestTriangle(t *testing.T) {
	ts := NewTriangleSource()
	nchan, samplerate, min, max := 4, 10000.0, 10, 15
	ts.Configure(nchan, samplerate, RawType(min), RawType(max))
	ds := DataSource(ts)
	if ds.Running() {
		t.Errorf("TriangleSource.Running() says true before first start.")
	}

	if err := Start(ds); err != nil {
		t.Fatalf("TriangleSource could not be started")
	}
	outputs := ds.Outputs()
	if len(outputs) != nchan {
		t.Errorf("TriangleSource.Ouputs() returns %d channels, want %d", len(outputs), nchan)
	}
	ds.BlockingRead()
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
	if err := Start(ds); err != nil {
		t.Fatalf("TriangleSource could not be started")
	}
	ds.BlockingRead()
	ds.Stop()
	err := ds.BlockingRead()
	if err != io.EOF {
		t.Errorf("TriangleSource did not return EOF on aborted BlockingRead")
	}

	// Check that Running() is correct
	if ds.Running() {
		t.Errorf("SimPulseSource.Running() says true before started.")
	}
	if err := Start(ds); err != nil {
		t.Fatalf("TriangleSource could not be started")
	}
	if !ds.Running() {
		t.Errorf("SimPulseSource.Running() says false after started.")
	}
	ds.Stop()
	if ds.Running() {
		t.Errorf("SimPulseSource.Running() says true after stopped.")
	}
}

func TestSimPulse(t *testing.T) {
	ps := NewSimPulseSource()
	config := &SimPulseSourceConfig{
		Nchan:      4,
		SampleRate: 150000.0,
		Pedestal:   1000.0,
		Amplitude:  10000.0,
		Nsamp:      16000,
	}
	ps.Configure(config)
	ds := DataSource(ps)
	if ds.Running() {
		t.Errorf("SimPulseSource.Running() says true before first start.")
	}

	if err := Start(ds); err != nil {
		t.Fatalf("SimPulseSource could not be started")
	}
	outputs := ds.Outputs()
	if len(outputs) != config.Nchan {
		t.Errorf("SimPulseSource.Ouputs() returns %d channels, want %d", len(outputs), config.Nchan)
	}
	ds.BlockingRead()
	for i, ch := range outputs {
		segment := <-ch
		data := segment.rawData
		if len(data) != config.Nsamp {
			t.Errorf("SimPulseSource output %d is length %d, expect %d", i, len(data), config.Nsamp)
		}
		min, max := RawType(65535), RawType(0)
		for j := 0; j < config.Nsamp; j++ {
			if data[j] < min {
				min = data[j]
			}
			if data[j] > max {
				max = data[j]
			}
		}
		if min != RawType(config.Pedestal+0.5) {
			t.Errorf("SimPulseSource minimum value is %d, expect %d", min, RawType(config.Pedestal+0.5))
		}
		if max <= RawType(config.Pedestal+config.Amplitude*0.5) {
			t.Errorf("SimPulseSource minimum value is %d, expect > %d", max, RawType(config.Pedestal+config.Amplitude*0.5))
		}
	}
	ds.Stop()

	// Now try a blocking read with abort.
	if err := Start(ds); err != nil {
		t.Fatalf("SimPulseSource could not be started")
	}
	ds.BlockingRead()
	ds.Stop()
	err := ds.BlockingRead()
	if err != io.EOF {
		t.Errorf("SimPulseSource did not return EOF on aborted BlockingRead")
	}

	// Check that Running() is correct
	if ds.Running() {
		t.Errorf("SimPulseSource.Running() says true before started.")
	}
	if err := Start(ds); err != nil {
		t.Fatalf("SimPulseSource could not be started")
	}
	if !ds.Running() {
		t.Errorf("SimPulseSource.Running() says false after started.")
	}
	ds.Stop()
	if ds.Running() {
		t.Errorf("SimPulseSource.Running() says true after stopped.")
	}

	// Now configure a 0-channel source and make sure it fails
	config.Nchan = 0
	ps.Configure(config)
	ds = DataSource(ps)
	if err := Start(ds); err == nil {
		t.Errorf("SimPulseSource starts without error when configured with 0 channels.")
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
