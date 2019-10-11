package dastard

// PhaseUnwrapper makes phase values continous by adding integers as needed
type PhaseUnwrapper struct {
	lastVal     int16
	offset      int16
	bitsToKeep  uint
	bitsToShift uint
	onePi       int16
	twoPi       int16
	highCount   int
	lowCount    int
	resetAfter  int // jump back to near 0 after this many
}

// NewPhaseUnwrapper creates a new PhaseUnwrapper object
func NewPhaseUnwrapper(bitsToKeep uint) *PhaseUnwrapper {
	u := new(PhaseUnwrapper)
	// as read from the Roach
	// data bytes representing a 2s complement integer
	// where 2^14 is 1 phi0
	// so int(data[i])/2^14 is a number from -0.5 to 0.5 phi0
	// after this function we want 2^12 to be 1 phi0
	// 2^12 = 4096
	// 2^14 = 16384
	u.bitsToKeep = bitsToKeep
	u.bitsToShift = 16 - bitsToKeep
	u.onePi = int16(1) << (bitsToKeep - 3)
	u.twoPi = u.onePi << 1
	u.resetAfter = 2000 // 2000 should be a setable parameter
	return u
}

// UnwrapInPlace unwraps in place
func (u *PhaseUnwrapper) UnwrapInPlace(data *[]RawType, scale RawType) {
	for i, rawVal := range *data {
		v := int16(rawVal*scale) >> u.bitsToShift // scale=2 for ABACO HACK!! FIX TO GENERALIZE
		delta := v - u.lastVal

		// short term unwrapping
		if delta > u.onePi {
			u.offset -= u.twoPi
		} else if delta < -u.onePi {
			u.offset += u.twoPi
		}

		// long term keeping baseline at same phi0
		// if the offset is nonzero for a long time, set it to zero
		if u.offset >= u.twoPi {
			u.highCount++
			u.lowCount = 0
		} else if u.offset <= -u.twoPi {
			u.lowCount++
			u.highCount = 0
		} else {
			u.lowCount = 0
			u.highCount = 0
		}
		if (u.highCount > u.resetAfter) || (u.lowCount > u.resetAfter) {
			u.offset = 0
			u.highCount = 0
			u.lowCount = 0
		}
		(*data)[i] = RawType(v + u.offset)
		u.lastVal = v
	}
}
