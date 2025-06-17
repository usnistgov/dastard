package lancero

import (
	"testing"
)

func TestNoHardware(t *testing.T) {
	var ncolsSet, nrowsSet, linePeriodSet int
	ncolsSet = 8
	nrowsSet = 8
	linePeriodSet = 20
	lan, err := NewNoHardware(ncolsSet, nrowsSet, linePeriodSet)
	if err != nil {
		t.Error(err)
	}
	ncols, nrows, linePeriod, err := testLanceroerSubroutine(lan, t)
	if err != nil {
		t.Error(err)
	}
	if ncols != ncolsSet {
		t.Errorf("want %v, have %v", ncolsSet, ncols)
	}
	if nrows != nrowsSet {
		t.Errorf("want %v, have %v", nrowsSet, nrows)
	}
	if linePeriod != linePeriodSet {
		t.Errorf("want %v, have %v", linePeriodSet, linePeriod)
	}
}
