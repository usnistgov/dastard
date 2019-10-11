package dastard

import (
	"testing"
)

func TestUnwrap(t *testing.T) {
	const bits2drop = 2
	pu := NewPhaseUnwrapper(bits2drop)
	const ndata = 16
	data := make([]RawType, ndata)

	// Test unwrap when no change is expected
	pu.UnwrapInPlace(&data)
	for i := 0; i < ndata; i++ {
		if data[i] != 0 {
			t.Errorf("data[%d] = %d, want 0", i, data[i])
		}
	}
	// Test basic unwrap
	data[8] = 16384 // this is a jump of 2π
	data[9] = 16384
	pu.UnwrapInPlace(&data)
	for i := 0; i < ndata; i++ {
		if data[i] != 0 {
			t.Errorf("data[%d] = %d, want 0", i, data[i])
		}
	}
	// Test unwrap on sawtooth
	for i := 0; i < ndata; i++ {
		data[i] = RawType((i * 2048) % 16384)
	}
	pu.UnwrapInPlace(&data)
	for i := 0; i < ndata; i++ {
		want := RawType(i * (2048 >> bits2drop))
		if data[i] != want {
			t.Errorf("data[%d] = %d, want %d", i, data[i], want)
		}
	}
}
