package dastard

import "testing"

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
}
