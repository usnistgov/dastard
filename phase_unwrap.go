package dastard

// PhaseUnwrapper makes phase values continous by adding integers as needed
type PhaseUnwrapper struct {
	lastVal       int16
	offset        int16
	fractionBits  uint // Before unwrapping, this many low bits are fractional ϕ0
	lowBitsToDrop uint // Drop this many least significant bits in each value
	onePi         int16
	twoPi         int16
	highCount     int
	lowCount      int
	resetAfter    int // jump back to near 0 after this many
}

// NewPhaseUnwrapper creates a new PhaseUnwrapper object
func NewPhaseUnwrapper(fractionBits, lowBitsToDrop uint) *PhaseUnwrapper {
	u := new(PhaseUnwrapper)
	// as read from the Roach
	// data bytes representing a 2s complement integer
	// where 2^fractionBits = ϕ0 of phase.
	// so int(data[i])/2^fractionBits is a number from -0.5 to 0.5 ϕ0
	// after this function we want 2^(fractionBits-lowBitsToDrop) to be
	// exactly one single ϕ0, or 2π of phase.
	u.fractionBits = fractionBits
	u.lowBitsToDrop = lowBitsToDrop
	u.twoPi = int16(1) << (fractionBits - lowBitsToDrop)
	u.onePi = u.twoPi >> 1
	u.resetAfter = 20000 // TODO: this should be a settable parameter
	return u
}

// UnwrapInPlace unwraps in place
func (u *PhaseUnwrapper) UnwrapInPlace(data *[]RawType) {
	for i, rawVal := range *data {
		v := int16(rawVal) >> u.lowBitsToDrop
		delta := v - u.lastVal
		u.lastVal = v

		// Short-term unwrapping
		if delta > u.onePi {
			u.offset -= u.twoPi
		} else if delta < -u.onePi {
			u.offset += u.twoPi
		}

		// Long-term unwrapping = keeping baseline at same ϕ0.
		// So if the offset is nonzero for a long time, set it to zero.
		// This will cause a one-time jump by an integer number of wraps.
		switch  {
		case u.offset >= u.twoPi:
			u.highCount++
			u.lowCount = 0
			if u.highCount > u.resetAfter {
				u.offset = 0
				u.highCount = 0
			}
			(*data)[i] = RawType(v + u.offset)

		case u.offset <= -u.twoPi:
			u.lowCount++
			u.highCount = 0
			if u.lowCount > u.resetAfter {
				u.offset = 0
				u.lowCount = 0
			}
			(*data)[i] = RawType(v + u.offset)

		default:
			u.lowCount = 0
			u.highCount = 0
			(*data)[i] = RawType(v)
		}
	}
}
