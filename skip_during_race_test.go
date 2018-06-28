// +build !race

package dastard

import (
	"testing"
	"time"

	"github.com/usnistgov/dastard/lancero"
)

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
