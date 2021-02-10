package dastard

import (
	"testing"
)

func assertPanic(t *testing.T, f func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	f()
}

func TestUnwrap(t *testing.T) {
	const bits2drop = 2
	enables := []bool{true, false}

	shouldFail := func() {
		NewPhaseUnwrapper(13, bits2drop, true, -1)
	}
	assertPanic(t, shouldFail)
	NewPhaseUnwrapper(13, bits2drop, false, -1)
	NewPhaseUnwrapper(13, bits2drop, true, 100)

	for fractionbits := uint(13); fractionbits <= 16; fractionbits++ {
		for _, enable := range enables {
			const resetAfter = 20000
			pu := NewPhaseUnwrapper(fractionbits, bits2drop, enable, resetAfter)
			const ndata = 16
			data := make([]RawType, ndata)
			original := make([]RawType, ndata)

			// Test unwrap when no change is expected
			pu.UnwrapInPlace(&data)
			for i := 0; i < ndata; i++ {
				if data[i] != 0 {
					t.Errorf("data[%d] = %d, want 0", i, data[i])
				}
			}
			// Test basic unwrap
			data[6] = RawType(1) << fractionbits // this is a jump of 2π
			data[7] = data[6]
			data[8] = data[7]
			data[9] = data[8]
			for i := 0; i < ndata; i++ {
				original[i] = data[i] >> bits2drop
			}
			pu.UnwrapInPlace(&data)

			for i, want := range original {
				if enable {
					want = 0
				}
				if data[i] != want {
					t.Errorf("unwrap: %t, data[%d] = %d, want %d", enable, i, data[i], want)
				}
			}
			// Test unwrap on sawtooth of 4 steps
			// Result should be a line.
			step := 1 << (fractionbits - 2)
			mod := step * 4
			for i := 0; i < ndata; i++ {
				data[i] = RawType((i * step) % mod)
				original[i] = data[i] >> bits2drop
			}
			pu.UnwrapInPlace(&data)
			for i, want := range original {
				if enable {
					want = RawType(i * (step >> bits2drop))
				}
				if data[i] != want {
					t.Errorf("unwrap: %t, (%d,%d) data[%d] = %d, want %d", enable, fractionbits,
						bits2drop, i, data[i], want)
				}
			}
		}
	}
}

func BenchmarkPhaseUnwrap(b *testing.B) {
	Nsamples := 5000000
	data := make([]RawType, Nsamples)
	for i := 0; i < Nsamples; i++ {
		data[i] = RawType(i % 50000)
	}

	const bits2drop = 2
	for fractionbits := uint(13); fractionbits <= 16; fractionbits++ {
		const enable = true
		const resetAfter = 20000
		pu := NewPhaseUnwrapper(fractionbits, bits2drop, enable, resetAfter)
		for i := 0; i < b.N; i++ {
			pu.UnwrapInPlace(&data)
		}
	}
}
