package dastard

import (
	"testing"
	"time"

	"github.com/usnistgov/dastard/lancero"
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

func TestNoHardwareSource(t *testing.T) {
	var ncolsSet, nrowsSet, linePeriodSet, nLancero int
	ncolsSet = 1
	nrowsSet = 4
	linePeriodSet = 20
	nLancero = 3
	source := new(LanceroSource)
	source.devices = make(map[int]*LanceroDevice, nLancero)
	cardDelay := []int{0} // a single card delay value works for multiple cards
	activeCards := make([]int, nLancero)
	for i := 0; i < nLancero; i++ {
		lan, err := lancero.NewNoHardware(ncolsSet, nrowsSet, linePeriodSet)
		if err != nil {
			t.Error(err)
		}
		dev := LanceroDevice{card: lan}
		dev.devnum = i
		source.devices[i] = &dev
		activeCards[i] = i
		source.ncards++
	}
	// above is essentially NewLanceroSource

	config := LanceroSourceConfig{ClockMhz: 125, CardDelay: cardDelay,
		ActiveCards: make([]int, nLancero)}
	if err := source.Configure(&config); err == nil && nLancero > 1 {
		t.Error("expected error for re-using a device")
	}
	config = LanceroSourceConfig{ClockMhz: 125, CardDelay: cardDelay,
		ActiveCards: activeCards}
	if err := source.Configure(&config); err != nil {
		t.Error(err)
	}
	for i := 0; i < nLancero; i++ {
		if config.ActiveCards[i] != config.AvailableCards[i] {
			t.Errorf("AvailableCards not populated corrects. AvailableCards %v", config.AvailableCards)
		}
	}

	if err := Start(source); err != nil {
		source.Stop()
		t.Fatal(err)
	}
	defer source.Stop()

	// end Start(source)
	if err := source.ConfigureMixFraction(0, 1.0); err == nil {
		t.Error("expected error for mixing on even channel")
	}
	if err := source.ConfigureMixFraction(1, 1.0); err != nil {
		t.Error(err)
	}
	time.Sleep(20 * time.Millisecond) // wait long enough for some data to be processed

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
		t.Errorf("want all zeros, have\n%v", data)
	}

}
