package dastard

import (
	"testing"
	"time"

	"github.com/usnistgov/dastard/internal/lancero"
)

// TestChannelOrder checks the map from channel to readout numbering
func TestChannelOrder(t *testing.T) {
	ls, err := NewLanceroSource()
	if err != nil {
		t.Error("NewLanceroSource failed:", err)
	}
	d0 := LanceroDevice{devnum: 0, nrows: 4, ncols: 4, cardDelay: 1}
	d1 := LanceroDevice{devnum: 1, nrows: 3, ncols: 3, cardDelay: 1}
	ls.devices[0] = &d0
	ls.devices[1] = &d1
	config := LanceroSourceConfig{FiberMask: 0xbeef, ActiveCards: []int{0, 1, 2},
		CardDelay: []int{1, 1, 1}}
	ls.Configure(&config)
	// Configure reads nrows from cringeGlobals.json, to test chan2readoutOrder we need to set nrows back to the original values
	ls.devices[0].nrows = 4
	ls.devices[1].nrows = 3

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
	// Configure reads nrows from cringeGlobals.json, to test chan2readoutOrder we need to set nrows back to the original values
	ls.devices[0].nrows = 4
	ls.devices[1].nrows = 3
	ls.updateChanOrderMap()
	for i, v := range ls.chan2readoutOrder {
		if v != expect[i] {
			t.Errorf("chan2readoutOrder[%d]=%d, want %d", i, v, expect[i])
		}
	}

	config.ActiveCards = []int{1}
	ls.Configure(&config)
	// Configure reads nrows from cringeGlobals.json, to test chan2readoutOrder we need to set nrows back to the original values
	ls.devices[0].nrows = 4
	ls.devices[1].nrows = 3
	ls.updateChanOrderMap()
	for i, v := range ls.chan2readoutOrder {
		if v != expect[i+32]-32 {
			t.Errorf("chan2readoutOrder[%d]=%d, want %d", i, v, expect[i+32]-32)
		}
	}

	// test roundint
	fs := []float64{-1.4, -.6, -.4, 0, .4, .5, .6, 1.4}
	es := []int{-1, -1, 0, 0, 0, 1, 1, 1}
	for i, f := range fs {
		if es[i] != roundint(f) {
			t.Errorf("roundint(%f)=%d, want %d", f, roundint(f), es[i])
		}
	}
}

func TestPrepareChannels(t *testing.T) {
	ls, err := NewLanceroSource()
	if err != nil {
		t.Error("NewLanceroSource failed:", err)
	}
	d0 := LanceroDevice{devnum: 0, nrows: 4, ncols: 4, cardDelay: 1}
	d1 := LanceroDevice{devnum: 1, nrows: 3, ncols: 3, cardDelay: 1}
	ls.devices[0] = &d0
	ls.devices[1] = &d1
	ls.active = make([]*LanceroDevice, 2)
	for i, device := range ls.devices {
		ls.active[i] = device
		ls.nchan += device.ncols * device.nrows * 2
	}

	// Test normal chan numbering
	expect := make([]int, 0, 200)
	ls.PrepareChannels()
	for i := 0; i < ls.nchan/2; i++ {
		expect = append(expect, i)
		expect = append(expect, i)
	}
	for i := 0; i < len(expect); i++ {
		if ls.chanNumbers[i] != expect[i] {
			t.Errorf("channel number for index %d is %d, want %d", i, ls.chanNumbers[i], expect[i])
		}
	}

	// Test chan numbering with FirstRow > 1
	ls.firstRowChanNum = 10
	ls.PrepareChannels()
	for i := 0; i < len(expect); i++ {
		expect[i] += ls.firstRowChanNum
		if ls.chanNumbers[i] != expect[i] {
			t.Errorf("channel number for index %d is %d, want %d", i, ls.chanNumbers[i], expect[i])
		}
	}

	// Test chan numbering with chanSepCards > 1
	ls.firstRowChanNum = 1
	ls.chanSepCards = 1000
	ls.PrepareChannels()
	expect = expect[:0] // truncate
	for i := 0; i < ls.nchan/2; i++ {
		expect = append(expect, i)
		expect = append(expect, i)
	}
	for i := 2 * d0.nrows * d0.ncols; i < ls.nchan; i++ {
		expect[i] += ls.chanSepCards - d0.nrows*d0.ncols
	}
	for i := 0; i < len(expect); i++ {
		expect[i] += ls.firstRowChanNum
		if ls.chanNumbers[i] != expect[i] {
			t.Errorf("channel number for index %d is %d, want %d", i, ls.chanNumbers[i], expect[i])
		}
	}

	// Test chan numbering with chanSepCols > 1
	ls.firstRowChanNum = 1
	ls.chanSepCards = 0
	ls.chanSepColumns = 100
	ls.PrepareChannels()
	expect = expect[:0] // truncate
	for c := 0; c < d0.ncols; c++ {
		for r := 0; r < d0.ncols; r++ {
			expect = append(expect, r+c*ls.chanSepColumns)
			expect = append(expect, r+c*ls.chanSepColumns)
		}
	}
	for c := d0.ncols; c < d0.ncols+d1.ncols; c++ {
		for r := 0; r < d1.ncols; r++ {
			expect = append(expect, r+c*ls.chanSepColumns)
			expect = append(expect, r+c*ls.chanSepColumns)
		}
	}
	for i := 0; i < len(expect); i++ {
		expect[i] += ls.firstRowChanNum
		if ls.chanNumbers[i] != expect[i] {
			t.Errorf("channel number for index %d is %d, want %d", i, ls.chanNumbers[i], expect[i])
		}
	}

	// Test chan numbering with chanSepCols > 1, chanSepColumns > 1
	ls.firstRowChanNum = 1
	ls.chanSepCards = 1000
	ls.chanSepColumns = 100
	ls.PrepareChannels()
	expect = expect[:0] // truncate
	for c := 0; c < d0.ncols; c++ {
		for r := 0; r < d0.ncols; r++ {
			expect = append(expect, r+c*ls.chanSepColumns)
			expect = append(expect, r+c*ls.chanSepColumns)
		}
	}
	for c := 0; c < d1.ncols; c++ {
		for r := 0; r < d1.ncols; r++ {
			expect = append(expect, r+c*ls.chanSepColumns+ls.chanSepCards)
			expect = append(expect, r+c*ls.chanSepColumns+ls.chanSepCards)
		}
	}
	for i := 0; i < len(expect); i++ {
		expect[i] += ls.firstRowChanNum
		if ls.chanNumbers[i] != expect[i] {
			t.Errorf("channel number for index %d is %d, want %d", i, ls.chanNumbers[i], expect[i])
		}
	}

	// Make sure PrepareChannels also correctly checking chanSepCards/Columns
	type Checks struct {
		sepCards, sepColumns int
		expect               bool
	}
	checks := []Checks{
		{0, 0, true},
		{0, 2, false},
		{0, 3, false},
		{0, 4, true},
		{100, 0, true},
		{16, 0, true},
		{15, 0, false},
		{16, 4, true},
		{19, 4, true},
		{19, 5, false},
		{20, 5, true},
		{-1, 0, false},
		{0, -1, false},
	}
	for _, chk := range checks {
		ls.chanSepCards = chk.sepCards
		ls.chanSepColumns = chk.sepColumns
		err := ls.PrepareChannels()
		succeed := err == nil
		if succeed != chk.expect {
			if chk.expect {
				t.Errorf("LanceroSource.PrepareChannels() should succeed with sepCards=%d, sepColumns=%d, but returned %v",
					chk.sepCards, chk.sepColumns, err)
			} else {
				t.Errorf("LanceroSource.PrepareChannels() should fail with sepCards=%d, sepColumns=%d, but didn't",
					chk.sepCards, chk.sepColumns)
			}
		}

	}
}

func TestNoHardwareSource(t *testing.T) {
	var ncolsSet, nrowsSet, linePeriodSet, nLancero int
	ncolsSet = 1
	nrowsSet = 4
	linePeriodSet = 1000 // don't make this too low, or test -race will fail
	nLancero = 3
	source := new(LanceroSource)
	source.readPeriod = time.Millisecond
	source.devices = make(map[int]*LanceroDevice, nLancero)
	source.buffersChan = make(chan BuffersChanType, 25)
	cardDelay := []int{0} // a single card delay value works for multiple cards
	activeCards := make([]int, nLancero)
	for i := 0; i < nLancero; i++ {
		lan, err := lancero.NewNoHardware(ncolsSet, nrowsSet, linePeriodSet)
		if err != nil {
			t.Error(err)
		}
		dev := LanceroDevice{card: lan, devnum: i}
		source.devices[i] = &dev
		activeCards[i] = i
		source.ncards++
	}
	// above is essentially NewLanceroSource

	config := LanceroSourceConfig{CardDelay: cardDelay,
		ActiveCards: make([]int, nLancero)}
	if err := source.Configure(&config); err == nil && nLancero > 1 {
		t.Error("expected error for re-using a device")
	}
	config = LanceroSourceConfig{CardDelay: cardDelay,
		ActiveCards: activeCards, FirstRow: 1}
	if err := source.Configure(&config); err != nil {
		t.Error(err)
	}
	for i := 0; i < nLancero; i++ {
		if config.ActiveCards[i] != config.DastardOutput.AvailableCards[i] {
			t.Errorf("AvailableCards not populated correctly. AvailableCards %v", config.DastardOutput.AvailableCards)
		}
	}

	// Start will call sampleCard
	// sampleCard will calculate nrows and lsync
	// if nrows doesn't match, it will error
	// if lsync doesn't match, it will give a warning
	// since these values from from cringeGlobals.json, we need to manully set them to match before this test
	for _, dev := range source.devices {
		dev.nrows = nrowsSet
		dev.lsync = linePeriodSet // we we calculate different values for lsync here, don't freak out over warnings
	}
	if err := Start(source, nil, 256, 1024); err != nil {
		//source.Stop() // source.Stop should work here, but it fails and we don't see the error message from t.Fatal
		t.Fatal(err)
	}
	defer source.Stop()

	if source.chanNumbers[3] != 2 {
		t.Errorf("LanceroSource.chanNumbers[3] has %v, want 2", source.chanNumbers[3])
	}
	if source.chanNames[3] != "chan2" {
		t.Errorf("LanceroSource.chanNames[3] has %v, want chan2", source.chanNames[3])
	}
	if source.chanNames[2] != "err2" {
		t.Errorf("LanceroSource.chanNames[2] %v, want err2", source.chanNames[3])
	}
	mfo := MixFractionObject{ChannelIndices: []int{0}, MixFractions: []float64{1.0}}
	if _, err := source.ConfigureMixFraction(&mfo); err == nil {
		t.Error("expected error for mixing on even channel")
	}
	mfo.ChannelIndices[0] = 1
	mix, err := source.ConfigureMixFraction(&mfo)
	if err != nil {
		t.Error(err)
	} else if mix[1] != 1.0 {
		t.Errorf("source.ConfigureMixFraction returns [%f], want %f", mix[0], 1.0)
	}
	time.Sleep(20 * time.Millisecond) // wait long enough for some data to be processed
	// these tests get lsync right at first, but while data is being processed
	// the lsync is not right. about half the time I run the test there are 3 blocking reads
	// at which point the lancero source shuts down due to it seeing lsync change
	// then Stop() will error but the err value is not checked so it doesn't cause a test failure
}

func TestMix(t *testing.T) {
	data := make([]RawType, 10)
	errData := make([]RawType, len(data))
	for i := range errData {
		errData[i] = RawType(i * 4)

	}
	mix := Mix{errorScale: 0}
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
	mix = Mix{errorScale: 1}
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
	mix = Mix{errorScale: 0}
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
	mix = Mix{errorScale: 1.0}
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
