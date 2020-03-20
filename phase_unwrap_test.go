package dastard

import (
	"testing"
)

func TestUnwrap(t *testing.T) {
	const fractionbits = 14
	const bits2drop = 2
	pu := NewPhaseUnwrapper(fractionbits, bits2drop)
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
	data[8] = RawType(1) << fractionbits // this is a jump of 2π
	data[9] = data[8]
	pu.UnwrapInPlace(&data)
	for i := 0; i < ndata; i++ {
		if data[i] != 0 {
			t.Errorf("data[%d] = %d, want 0", i, data[i])
		}
	}
	// Test unwrap on sawtooth
	for i := 0; i < ndata; i++ {
		data[i] = RawType((i * 4096) % 16384)
	}
	pu.UnwrapInPlace(&data)
	for i := 0; i < ndata; i++ {
		want := RawType(i * (4096 >> bits2drop))
		if data[i] != want {
			t.Errorf("data[%d] = %d, want %d", i, data[i], want)
		}
	}
}
