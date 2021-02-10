package dastard

import "fmt"

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
	resetAfter    int  // jump back to near 0 after this many
	enable        bool // are we even unwrapping at all?
}

// NewPhaseUnwrapper creates a new PhaseUnwrapper object
func NewPhaseUnwrapper(fractionBits, lowBitsToDrop uint, enable bool, resetAfter int) *PhaseUnwrapper {
	u := new(PhaseUnwrapper)
	// as read from the Roach
	// data bytes representing a 2s complement integer
	// where 2^fractionBits = ϕ0 of phase.
	// so int(data[i])/2^fractionBits is a number from -0.5 to 0.5 ϕ0
	// after this function we want 2^(fractionBits-lowBitsToDrop) to be
	// exactly one single ϕ0, or 2π of phase.
	//
	// As of Jan 2021, we decided to let fractionBits = all bits, so 16
	// or 32 for int16 or int32, but leave that parameter here...for now.
	u.fractionBits = fractionBits
	u.lowBitsToDrop = lowBitsToDrop
	u.twoPi = int16(1) << (fractionBits - lowBitsToDrop)
	u.onePi = u.twoPi >> 1
	u.resetAfter = resetAfter
	u.enable = enable
	if resetAfter <= 0 && enable {
		panic(fmt.Sprintf("NewPhaseUnwrapper is enabled but with resetAfter=%d, expect positive", resetAfter))
	}
	return u
}

// UnwrapInPlace unwraps in place
func (u *PhaseUnwrapper) UnwrapInPlace(data *[]RawType) {
	drop := u.lowBitsToDrop

	// When unwrapping is disabled, simply drop the low bits.
	if !u.enable {
		u.lowCount = 0
		u.highCount = 0
		for i, rawVal := range *data {
			(*data)[i] = rawVal >> drop
		}
		return
	}

	// Enter this loop only if unwrapping is enabled
	for i, rawVal := range *data {
		v := int16(rawVal) >> drop
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
		switch {
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
