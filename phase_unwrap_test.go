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
	enables := []bool{true, false}

	shouldFail1 := func() {
		// Fail because reset time is negative
		NewPhaseUnwrapperStreams(13, bits2drop, true, biaslevel, -1, pulsesign)
	}
	shouldFail2 := func() {
		// Fail because unwrap is enabled but with 0 bits to drop
		NewPhaseUnwrapperStreams(13, 0, true, biaslevel, 100, pulsesign)
	}
	shouldFail3 := func() {
		// Fail because unwrap is enabled but with 0 bits to drop
		NewPhaseUnwrapper(13, 0, true, biaslevel, pulsesign)
	}
	assertPanic(t, shouldFail1)
	assertPanic(t, shouldFail2)
	assertPanic(t, shouldFail3)

	NewPhaseUnwrapperStreams(13, bits2drop, false, biaslevel, -1, pulsesign)
	NewPhaseUnwrapperStreams(13, bits2drop, true, biaslevel, 100, pulsesign)
	NewPhaseUnwrapper(13, bits2drop, true, biaslevel, pulsesign)

	for fractionbits := uint(13); fractionbits <= 16; fractionbits++ {
		for _, enable := range enables {
			const resetAfter = 20000
			resetValue := RawType(0)
			if enable {
				resetValue = RawType(1) << (fractionbits - bits2drop)
			}
			const ndata = 16
			data1 := make([]RawType, ndata)
			data2 := make([]RawType, ndata)
			target := make([]RawType, ndata)
			pu1 := NewPhaseUnwrapper(fractionbits, bits2drop, enable, biaslevel, pulsesign)
			pu2 := NewPhaseUnwrapperStreams(fractionbits, bits2drop, enable, biaslevel, resetAfter, pulsesign)

			// Test unwrap when no change is expected
			pu1.UnwrapInPlace(&data1)
			pu2.UnwrapInPlace(&data2)
			for i := 0; i < ndata; i++ {
				if data1[i] != resetValue {
					t.Errorf("pu(%d) enable=%v: data1[%d] = %d, want %d", fractionbits, enable, i, data1[i], resetValue)
				}
				if data2[i] != resetValue {
					t.Errorf("pu(%d) enable=%v: data2[%d] = %d, want %d", fractionbits, enable, i, data2[i], resetValue)
				}
			}
			// Test basic unwrap
			twopi := RawType(1) << fractionbits // this is a jump of 2π
			for i := 0; i < ndata; i++ {
				data1[i] = 100
				data2[i] = 100
				if i > 5 && i < 10 {
					data1[i] += twopi
					data2[i] += twopi
				}
				if enable {
					target[i] = (100 >> bits2drop) + resetValue
				} else {
					target[i] = (data1[i] >> bits2drop)
				}
			}
			pu1.UnwrapInPlace(&data1)
			pu2.UnwrapInPlace(&data2)

			for i, want := range target {
				if data1[i] != want {
					t.Errorf("unwrap: %t, data1[%d] = %d, want %d", enable, i, data1[i], want)
				}
				if data2[i] != want {
					t.Errorf("unwrap: %t, data2[%d] = %d, want %d", enable, i, data2[i], want)
				}
			}
			// Test unwrap on sawtooth of 4 steps
			// Result should be a line.
			step := 1 << (fractionbits - 2)
			mod := step * 4
			for i := 0; i < ndata; i++ {
				data1[i] = RawType((i * step) % mod)
				data2[i] = RawType((i * step) % mod)
				if enable {
					target[i] = RawType(i*(step>>bits2drop)) + resetValue
				} else {
					target[i] = data1[i] >> bits2drop
				}
			}
			pu1.UnwrapInPlace(&data1)
			pu2.UnwrapInPlace(&data2)
			for i, want := range target {
				if data1[i] != want {
					t.Errorf("unwrap: %t, (%d,%d) data1[%d] = %d, want %d", enable, fractionbits,
						bits2drop, i, data1[i], want)
				}
				if data2[i] != want {
					t.Errorf("unwrap: %t, (%d,%d) data2[%d] = %d, want %d", enable, fractionbits,
						bits2drop, i, data2[i], want)
				}
			}
		}
	}

	// Test biased unwrapping. Range that does NOT trigger an unwrap should be [-22768,42768]
	biasX := 10000
	pu1 := NewPhaseUnwrapper(16, bits2drop, true, 0, pulsesign)
	pu2 := NewPhaseUnwrapper(16, bits2drop, true, biasX, pulsesign)
	pu3 := NewPhaseUnwrapperStreams(16, bits2drop, true, 0, 100, pulsesign)
	pu4 := NewPhaseUnwrapperStreams(16, bits2drop, true, biasX, 100, pulsesign)
	// In order, have big steps that overflow both, overflow just the unbiased, negative that overflows just
	// the biased, and then negative that overflows both.
	steps := []int{80, 40, -20, 0, 44000, 0, 40000, 0, -28000, 0, -40000}
	expectsteps1 := []int{20, 10, -5, 0, 11000 - 16384, 0, 10000 - 16384, 0, -7000, 0, 6384}
	expectsteps2 := []int{20, 10, -5, 0, 11000 - 16384, 0, 10000, 0, 16384 - 7000, 0, 6384}

	input1 := make([]RawType, 1+len(steps))
	input2 := make([]RawType, 1+len(steps))
	input3 := make([]RawType, 1+len(steps))
	input4 := make([]RawType, 1+len(steps))
	input1[0] = 20000
	input2[0] = 20000
	input3[0] = 20000
	input4[0] = 20000
	for i, val := range steps {
		input1[i+1] = input1[i] + RawType(val)
		input2[i+1] = input2[i] + RawType(val)
		input3[i+1] = input3[i] + RawType(val)
		input4[i+1] = input4[i] + RawType(val)
	}
	pu1.UnwrapInPlace(&input1)
	pu2.UnwrapInPlace(&input2)
	pu3.UnwrapInPlace(&input3)
	pu4.UnwrapInPlace(&input4)
	for i, expect := range expectsteps1 {
		step := input1[i+1] - input1[i]
		if step != RawType(expect) {
			t.Errorf("PU step[%d]=0x%x-0x%x step %d with bias=0, want %d", i, input1[i+1], input1[i], step, RawType(expect))
		}
		step = input3[i+1] - input3[i]
		if step != RawType(expect) {
			t.Errorf("PUS step[%d]=0x%x-0x%x step %d with bias=0, want %d", i, input3[i+1], input3[i], step, RawType(expect))
		}
	}
	for i, expect := range expectsteps2 {
		step := input2[i+1] - input2[i]
		if step != RawType(expect) {
			t.Errorf("PU step[%d]=0x%x-0x%x step %d with bias=10000, want %d", i, input2[i+1], input2[i], step, RawType(expect))
		}
		step = input4[i+1] - input4[i]
		if step != RawType(expect) {
			t.Errorf("PUS step[%d]=0x%x-0x%x step %d with bias=10000, want %d", i, input4[i+1], input4[i], step, RawType(expect))
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
	for fractionbits := uint(13); fractionbits <= 16; fractionbits++ {
		const enable = true
		const resetAfter = 20000
		pu := NewPhaseUnwrapperStreams(fractionbits, bits2drop, enable, bias, resetAfter, pulsesign)
		for i := 0; i < b.N; i++ {
			pu.UnwrapInPlace(&data)
		}
	}
}
