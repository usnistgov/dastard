package dastard

import (
	"testing"
)

func TestUnwrap(t *testing.T) {
	const bits2drop = 2

	for fractionbits := uint(13); fractionbits <= 14; fractionbits++ {
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
		// Test unwrap on sawtooth of 4 steps
		// Result should be a line.
		step := 1 << (fractionbits - 2)
		mod := step * 4
		for i := 0; i < ndata; i++ {
			data[i] = RawType((i * step) % mod)
		}
		pu.UnwrapInPlace(&data)
		for i := 0; i < ndata; i++ {
			want := RawType(i * (step >> bits2drop))
			if data[i] != want {
				t.Errorf("(%d,%d) data[%d] = %d, want %d", fractionbits, bits2drop, i, data[i], want)
			}
		}
	}

}
