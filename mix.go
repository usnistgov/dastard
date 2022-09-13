package dastard

import (
	"math"
)

// Mix performns the mix for lancero data and retards the raw data stream by
// one sample so it can be mixed with the appropriate error sample. This corrects
// for a poor choice in the TDM firmware design, but so it goes.
//
// fb_physical[n] refers to the feedback signal applied during tick [n]
// err_physical[n] refers to the error signal measured during tick [n]
//
// Unfortunately, the data stream pairs them up differently:
// fb_data[n] = fb_physical[n+1]
// err_data[n] = err_physical[n]
//
// At frame [n] we get data for the error measured during frame [n]
// and the feedback computed based on it, which is the feedback
// that will be _applied_ during frame [n+1].
//
// We want
// mix[n] = fb_physical[n] + errorScale * err_physical[n], so
// mix[n] = fb_data[n-1]   + errorScale * err_data[n], or
// mix[n+1] = fb_data[n]   + errorScale * err_data[n+1]
//
// Second issue: the error signal we work with is a sum of NSAMP samples from
// the ADC, but autotune's values assume that we work with the _mean_ (because it
// lets autotune communicate an NSAMP-agnostic value). So we store NOT the auto-
// tune value but the value that actually multiplies the error sum.
type Mix struct {
	errorScale float64 // Multiply this by raw error data. NSAMP is scaled out.
	lastFb     RawType
}

// MixRetardFb mixes err into fbs, alters fbs in place to contain the mixed values
// consecutive calls must be on consecutive data.
// The following ASSUMES that error signals are signed. That holds for Lancero
// TDM systems, at least, and that is the only source that uses Mix.
func (m *Mix) MixRetardFb(fbs *[]RawType, errs *[]RawType) {
	const mask = ^RawType(0x03)
	if m.errorScale == 0.0 {
		for j := 0; j < len(*fbs); j++ {
			fb := m.lastFb
			m.lastFb = (*fbs)[j] & mask
			(*fbs)[j] = fb
		}
		return
	}
	for j := 0; j < len(*fbs); j++ {
		fb := m.lastFb
		mixAmount := float64(int16((*errs)[j])) * m.errorScale
		// Be careful not to overflow!
		floatMixResult := mixAmount + float64(fb)
		m.lastFb = (*fbs)[j] & mask
		if floatMixResult >= math.MaxUint16 {
			(*fbs)[j] = math.MaxUint16
		} else if floatMixResult < 0 {
			(*fbs)[j] = 0
		} else {
			(*fbs)[j] = RawType(roundint(floatMixResult))
		}
	}
}
