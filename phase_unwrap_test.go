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
	var biaslevel int
	const pulsesign = 1
	const dontInvert = false
	enables := []bool{true, false}

	shouldFail1 := func() {
		NewPhaseUnwrapper(13, bits2drop, true, biaslevel, -1, pulsesign, dontInvert)
	}
	shouldFail2 := func() {
		NewPhaseUnwrapper(13, 0, true, biaslevel, -1, pulsesign, dontInvert)
	}
	assertPanic(t, shouldFail1)
	assertPanic(t, shouldFail2)

	NewPhaseUnwrapper(13, bits2drop, false, biaslevel, -1, pulsesign, dontInvert)
	NewPhaseUnwrapper(13, bits2drop, true, biaslevel, 100, pulsesign, dontInvert)

	expectA := map[uint]RawType{13: 0x1fff, 14: 0x3fff, 15: 0x7fff, 16: 0xffff}
	for fractionbits, expectMask := range expectA {
		pu := NewPhaseUnwrapper(fractionbits, bits2drop, true, 0, 20000, pulsesign, dontInvert)
		if expectMask != pu.signMask {
			t.Errorf("PhaseUnwrapper.signMask=%x, want %x", pu.signMask, expectMask)
		}
	}

	// Check inversion happens, even with unwrap disabled and 0 bits to drop
	puInverter := NewPhaseUnwrapper(14, 0, false, 0, 20000, pulsesign, true)
	data := make([]RawType, 0xffff)
	for i := RawType(0); i < 0xffff; i++ {
		data[i] = i
	}
	puInverter.UnwrapInPlace(&data)
	for i := RawType(0); i < 0xffff; i++ {
		if data[i]+i != 0xffff {
			t.Errorf("unwrapping with inversion 0x%4.4x -> 0x%4.4x, want 0x%4.4x", i,
				data[i], 0xffff-i)
		}
	}

	for fractionbits := uint(13); fractionbits <= 16; fractionbits++ {
		for _, enable := range enables {
			const resetAfter = 20000
			resetValue := RawType(0)
			if enable {
				resetValue = RawType(1) << (fractionbits - bits2drop)
			}
			pu := NewPhaseUnwrapper(fractionbits, bits2drop, enable, biaslevel, resetAfter, pulsesign, dontInvert)
			const ndata = 16
			data := make([]RawType, ndata)
			target := make([]RawType, ndata)

			// Test unwrap when no change is expected
			pu.UnwrapInPlace(&data)
			for i := 0; i < ndata; i++ {
				if data[i] != resetValue {
					t.Errorf("data[%d] = %d, want %d", i, data[i], resetValue)
				}
			}
			// Test basic unwrap
			twopi := RawType(1) << fractionbits // this is a jump of 2π
			baseline := RawType(100)
			for i := 0; i < ndata; i++ {
				data[i] = baseline
				if i > 5 && i < 10 {
					data[i] += twopi
				}
				target[i] = (baseline >> bits2drop) + resetValue
			}
			pu.UnwrapInPlace(&data)

			for i, want := range target {
				if data[i] != want {
					t.Errorf("unwrap fb=%d error: enable=%t, data[%d] = %d, want %d", fractionbits, enable, i, data[i], want)
				}
			}
			// Test unwrap on sawtooth of 4 steps
			// Result should be a line.
			step := 1 << (fractionbits - 2)
			mod := step * 4
			for i := 0; i < ndata; i++ {
				data[i] = RawType((i * step) % mod)
				if enable {
					target[i] = RawType(i*(step>>bits2drop)) + resetValue
				} else {
					target[i] = data[i] >> bits2drop
				}
			}
			pu.UnwrapInPlace(&data)
			for i, want := range target {
				if data[i] != want {
					t.Errorf("unwrap: %t, (%d,%d) data[%d] = %d, want %d", enable, fractionbits,
						bits2drop, i, data[i], want)
				}
			}
		}
	}

	// Test biased unwrapping. Range that does NOT trigger an unwrap should be [-22768,42768]
	biasX := 10000
	pu1 := NewPhaseUnwrapper(16, bits2drop, true, 0, 100, pulsesign, dontInvert)
	pu2 := NewPhaseUnwrapper(16, bits2drop, true, biasX, 100, pulsesign, dontInvert)
	// In order, have big steps that overflow both, overflow just the unbiased, negative that overflows just
	// the biased, and then negative that overflows both.
	steps := []int{80, 40, -20, 0, 44000, 0, 40000, 0, -28000, 0, -40000}
	expectsteps1 := []int{20, 10, -5, 0, 11000 - 16384, 0, 10000 - 16384, 0, -7000, 0, 6384}
	expectsteps2 := []int{20, 10, -5, 0, 11000 - 16384, 0, 10000, 0, 16384 - 7000, 0, 6384}

	input1 := make([]RawType, 1+len(steps))
	input2 := make([]RawType, 1+len(steps))
	input1[0] = 20000
	input2[0] = 20000
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
	const pulsesign = +1
	const invertData = false
	for fractionbits := uint(13); fractionbits <= 16; fractionbits++ {
		const enable = true
		const resetAfter = 20000
		pu := NewPhaseUnwrapper(fractionbits, bits2drop, enable, bias, resetAfter, pulsesign, invertData)
		for i := 0; i < b.N; i++ {
			pu.UnwrapInPlace(&data)
		}
	}
}
