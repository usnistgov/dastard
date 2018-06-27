package dastard

import (
	"testing"
)

// TestChannelOrder checks the map from channel to readout numbering
func TestChannelOrder(t *testing.T) {
	ls, err := NewLanceroSource()
	if err != nil {
		t.Error("NewLanceroSource failed:", err)
	}
	d0 := LanceroDevice{devnum: 0, nrows: 4, ncols: 4}
	d1 := LanceroDevice{devnum: 0, nrows: 3, ncols: 3}
	ls.devices[0] = &d0
	ls.devices[1] = &d1
	config := LanceroSourceConfig{FiberMask: 0xbeef, ActiveCards: []int{0, 1, 2},
		CardDelay: []int{1, 1, 1}}
	ls.Configure(&config)

	if len(ls.active) > 2 {
		t.Errorf("ls.active contains %d cards, want 2", len(ls.active))
	}
	ls.updateChanOrderMap()
	expect := []int{
		0, 1, 8, 9, 16, 17, 24, 25,
		2, 3, 10, 11, 18, 19, 26, 27,
		4, 5, 12, 13, 20, 21, 28, 29,
		6, 7, 14, 15, 22, 23, 30, 31,
		32, 33, 38, 39, 44, 45,
		34, 35, 40, 41, 46, 47,
		36, 37, 42, 43, 48, 49,
	}
	for i, v := range ls.chan2readoutOrder {
		if v != expect[i] {
			t.Errorf("chan2readoutOrder[%d]=%d, want %d", i, v, expect[i])
		}
	}

	config.ActiveCards = []int{0}
	ls.Configure(&config)
	ls.updateChanOrderMap()
	for i, v := range ls.chan2readoutOrder {
		if v != expect[i] {
			t.Errorf("chan2readoutOrder[%d]=%d, want %d", i, v, expect[i])
		}
	}

	config.ActiveCards = []int{1}
	ls.Configure(&config)
	ls.updateChanOrderMap()
	for i, v := range ls.chan2readoutOrder {
		if v != expect[i+32]-32 {
			t.Errorf("chan2readoutOrder[%d]=%d, want %d", i, v, expect[i+32]-32)
		}
	}

	ls.StartRun()
	ls.stop()
	ls.Delete()

	// test roundint
	fs := []float64{-1.4, -.6, -.4, 0, .4, .5, .6, 1.4}
	es := []int{-1, -1, 0, 0, 0, 1, 1, 1}
	for i, f := range fs {
		if es[i] != roundint(f) {
			t.Errorf("roundint(%f)=%d, want %d", f, roundint(f), es[i])
		}
	}
}

func TestMix(t *testing.T) {
	data := make([]RawType, 10)
	errData := make([]RawType, len(data))
	for i := range errData {
		errData[i] = RawType(i * 4)

	}
	mix := Mix{mixFraction: 0}
	mix.MixRetardFb(&data, &errData)
	passed := true
	for i := range data {
		if data[i] != 0 {
			passed = false
		}
	}
	if !passed {
		t.Errorf("want all zeros, have\n%v", data)
	}
	mix = Mix{mixFraction: 1}
	mix.MixRetardFb(&data, &errData)
	passed = true
	for i := range data {
		if data[i] != errData[i] {
			passed = false
		}
	}
	if !passed {
		t.Errorf("want %v\n have\n%v", errData, data)
	}
	if mix.lastFb != 0 {
		t.Errorf("want 0, have %v", mix.lastFb)
	}
	mix.lastFb = 100
	mix.MixRetardFb(&data, &errData)
	passed = true
	expect := []RawType{100 + 0, (0 + 1) * 4, (1 + 2) * 4, (2 + 3) * 4,
		(3 + 4) * 4, (4 + 5) * 4, (5 + 6) * 4, (6 + 7) * 4, (7 + 8) * 4, (8 + 9) * 4}
	for i := range data {
		if data[i] != expect[i] {
			passed = false
		}
	}
	if !passed {
		t.Errorf("want %v\nhave\n%v", expect, data)
	}
	if mix.lastFb != 36 {
		t.Errorf("want 36, have %v", mix.lastFb)
	}
	// test 2 LSBs of feedback are masked off
	for i := range data {
		data[i] = RawType(i % 4)
	}
	mix = Mix{mixFraction: 0}
	mix.MixRetardFb(&data, &errData)
	passed = true
	for i := range data {
		if data[i] != 0 {
			passed = false
		}
	}
	if !passed {
		t.Errorf("have %v, want all zeros", data)
	}

	// Check that overflows are clamped to proper range
	err1 := []RawType{100, 0, 65436} // {100, 0, -100} as signed ints
	mix = Mix{mixFraction: 1.0}
	mix.lastFb = 65530
	fb := []RawType{0, 50, 50}
	expect = []RawType{65535, 0, 0}
	mix.MixRetardFb(&fb, &err1)
	for i, e := range expect {
		if fb[i] != e {
			t.Errorf("mix with overflows fb[%d]=%d, want %d", i, fb[i], e)
		}
	}
}
