package dastard

import "fmt"

// PhaseUnwrapper makes phase values continous by adding integers as needed
type PhaseUnwrapper struct {
	lastVal       uint16
	offset        uint16
	fractionBits  uint // Before unwrapping, this many low bits are fractional ϕ0
	lowBitsToDrop uint // Drop this many least significant bits in each value
	upperStepLim  int16
	lowerStepLim  int16
	twoPi         uint16
	resetCount    int
	resetAfter    int // jump back to near 0 after this many
	resetOffset   uint16
	signMask      RawType // Mask off the upper bits of raw: effect = convert signed->unsigned.
	enable        bool    // are we even unwrapping at all?
}

// NewPhaseUnwrapper creates a new PhaseUnwrapper object
func NewPhaseUnwrapper(fractionBits, lowBitsToDrop uint, enable bool, biasLevel, resetAfter, pulseSign int) *PhaseUnwrapper {
	// Subtle point here: if no bits are to be dropped, then it makes no sense to perform
	// phase unwrapping. When lowBitsToDrop==0, we cannot allow enable==true (because where would you
	// put the bits set in the unwrapping process when there are no dropped bits?)
	if lowBitsToDrop == 0 && enable {
		panic("NewPhaseUnwrapper is enabled but with lowBitsToDrop=0, must be >0.")
	}

	u := new(PhaseUnwrapper)
	// data bytes representing a 2s complement integer
	// where 2^fractionBits = ϕ0 of phase.
	// so int(data[i])/2^fractionBits is a number from -0.5 to 0.5 ϕ0
	// after this function we want 2^(fractionBits-lowBitsToDrop) to be
	// exactly one single ϕ0, or 2π of phase.
	//
	// As of Jan 2021, we decided to let fractionBits = all bits for Abaco sources, so 16
	// or 32 for int16 or int32, but leave that parameter here--ROACH2 sources need it.
	u.fractionBits = fractionBits
	u.lowBitsToDrop = lowBitsToDrop
	u.signMask = ^(RawType(0xffff) << fractionBits)
	u.enable = enable

	if lowBitsToDrop > 0 && enable {
		u.twoPi = uint16(1) << (fractionBits - lowBitsToDrop)
		onePi := int16(1) << (fractionBits - lowBitsToDrop - 1)
		bias := int16(biasLevel>>lowBitsToDrop) % int16(u.twoPi)
		u.upperStepLim = bias + onePi
		u.lowerStepLim = bias - onePi

		if pulseSign > 0 {
			u.resetOffset = u.twoPi
		} else {
			u.resetOffset = uint16(-2 * int(u.twoPi))
		}
		u.offset = u.resetOffset

		u.resetAfter = resetAfter

		if resetAfter <= 0 && enable {
			panic(fmt.Sprintf("NewPhaseUnwrapper is enabled but with resetAfter=%d, expect positive", resetAfter))
		}
	}
	return u
}

// UnwrapInPlace unwraps in place
func (u *PhaseUnwrapper) UnwrapInPlace(data *[]RawType) {
	drop := u.lowBitsToDrop
	if drop == 0 {
		return
	}

	// When unwrapping is disabled, simply drop the low bits.
	if !u.enable {
		u.resetCount = 0
		for i, rawVal := range *data {
			(*data)[i] = (rawVal & u.signMask) >> drop
		}
		return
	}

	// Enter this loop only if unwrapping is enabled
	for i, rawVal := range *data {
		v := uint16(rawVal&u.signMask) >> drop
		thisstep := int16(v - u.lastVal)
		u.lastVal = v

		// Short-term unwrapping
		if thisstep > u.upperStepLim {
			u.offset -= u.twoPi
		} else if thisstep < u.lowerStepLim {
			u.offset += u.twoPi
		}

		// Long-term unwrapping means keeping baseline at same ϕ0.
		// So if the offset is unequal to the resetOffset for a long time, set it to resetOffset.
		// This will cause a one-time jump by an integer number of ϕ0 units (an integer
		// multiple of 2π in phase angle).
		if u.offset == u.resetOffset {
			u.resetCount = 0
		} else {
			u.resetCount++
			if u.resetCount > u.resetAfter {
				u.offset = u.resetOffset
				u.resetCount = 0
			}
		}
		(*data)[i] = RawType(v + u.offset)
	}
}
