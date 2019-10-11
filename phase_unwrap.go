package dastard

// PhaseUnwrapper makes phase values continous by adding integers as needed
type PhaseUnwrapper struct {
	lastVal       int16
	offset        int16
	lowBitsToDrop uint // Drop this many least significant bits in each value
	onePi         int16
	twoPi         int16
	highCount     int
	lowCount      int
	resetAfter    int // jump back to near 0 after this many
}

// NewPhaseUnwrapper creates a new PhaseUnwrapper object
func NewPhaseUnwrapper(lowBitsToDrop uint) *PhaseUnwrapper {
	u := new(PhaseUnwrapper)
	// as read from the Roach
	// data bytes representing a 2s complement integer
	// where 2^14 is 1 phi0
	// so int(data[i])/2^14 is a number from -0.5 to 0.5 phi0
	// after this function we want 2^12 to be 1 phi0
	// 2^12 = 4096
	// 2^14 = 16384
	u.lowBitsToDrop = lowBitsToDrop
	u.onePi = int16(1) << (13 - lowBitsToDrop) // TODO: why this 13? Should be settable?
	u.twoPi = u.onePi << 1
	u.resetAfter = 2000 // TODO: this should be a settable parameter
	return u
}

// UnwrapInPlace unwraps in place
func (u *PhaseUnwrapper) UnwrapInPlace(data *[]RawType) {
	for i, rawVal := range *data {
		v := int16(rawVal) >> u.lowBitsToDrop
		delta := v - u.lastVal

		// Short-term unwrapping
		if delta > u.onePi {
			u.offset -= u.twoPi
		} else if delta < -u.onePi {
			u.offset += u.twoPi
		}

		// Long-term unwrapping = keeping baseline at same phi0.
		// So if the offset is nonzero for a long time, set it to zero.
		// This will cause a one-time jump by an integer number of wraps.
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
