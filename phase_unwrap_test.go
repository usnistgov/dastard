package dastard

import (
	"testing"
)

func TestUnwrap(t *testing.T) {
	pu := NewPhaseUnwrapper(roachBitsToKeep)
	data := make([]RawType, 12)
	pu.UnwrapInPlace(&data, roachScale)
}
