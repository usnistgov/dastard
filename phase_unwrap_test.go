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
	var bias int16
	enables := []bool{true, false}

	shouldFail1 := func() {
		NewPhaseUnwrapper(13, bits2drop, bias, true, -1)
	}
	shouldFail2 := func() {
		NewPhaseUnwrapper(13, 0, bias, true, -1)
	}
	assertPanic(t, shouldFail1)
	assertPanic(t, shouldFail2)

	NewPhaseUnwrapper(13, bits2drop, bias, false, -1)
	NewPhaseUnwrapper(13, bits2drop, bias, true, 100)

	for fractionbits := uint(13); fractionbits <= 16; fractionbits++ {
		for _, enable := range enables {
			const resetAfter = 20000
			pu := NewPhaseUnwrapper(fractionbits, bits2drop, bias, enable, resetAfter)
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

	// Test biased unwrapping. Range that does NOT trigger an unwrap should be [-22768,42768]
	biasX := int16(10000)
	pu1 := NewPhaseUnwrapper(16, bits2drop, 0, true, 100)
	pu2 := NewPhaseUnwrapper(16, bits2drop, biasX, true, 100)
	// In order, have big steps that overflow both, overflow just the unbiased, negative that overflows just
	// the biased, and then negative that overflows both.
	steps := []int{80, 40, -20, 0, 44000, 0, 40000, 0, -28000, 0, -40000}
	expectsteps1 := []int{20, 10, -5, 0, 11000 - 16384, 0, 10000 - 16384, 0, -7000, 0, 6384}
	expectsteps2 := []int{20, 10, -5, 0, 11000 - 16384, 0, 10000, 0, 16384 - 7000, 0, 6384}

	input1 := make([]RawType, 1+len(steps))
	input2 := make([]RawType, 1+len(steps))
	for i, val := range steps {
		input1[i+1] = input1[i] + RawType(val)
		input2[i+1] = input2[i] + RawType(val)
	}
	pu1.UnwrapInPlace(&input1)
	pu2.UnwrapInPlace(&input2)
	for i, expect := range expectsteps1 {
		step := input1[i+1] - input1[i]
		if step != RawType(expect) {
			t.Errorf("step[%d]=0x%x-0x%x step %d with bias=0, want %d", i, input1[i+1], input1[i], step, RawType(expect))
		}
	}
	for i, expect := range expectsteps2 {
		step := input2[i+1] - input2[i]
		if step != RawType(expect) {
			t.Errorf("step[%d]=0x%x-0x%x step %d with bias=10000, want %d", i, input2[i+1], input2[i], step, RawType(expect))
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
	const bias = 0
	for fractionbits := uint(13); fractionbits <= 16; fractionbits++ {
		const enable = true
		const resetAfter = 20000
		pu := NewPhaseUnwrapper(fractionbits, bits2drop, bias, enable, resetAfter)
		for i := 0; i < b.N; i++ {
			pu.UnwrapInPlace(&data)
		}
	}
}
