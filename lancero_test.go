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