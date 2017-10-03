package dastard

import (
	"fmt"
)

// DataChannel contains all the state needed to decimate, trigger, write, and publish data.
type DataChannel struct {
	Channum int
	Abort   <-chan struct{}
	DecimateState
}

// DecimateState contains all the state needed to decimate data.
type DecimateState struct {
	DecimateLevel   int
	Decimate        bool
	DecimateAvgMode bool
}

// ProcessData drains the data channel and processes whatever is found there.
func (dc *DataChannel) ProcessData(dataIn <-chan []RawType) {
	for {
		select {
		case <-dc.Abort:
			return
		case data := <-dataIn:
			fmt.Printf("Chan %d:  found %d values starting with %v\n", dc.Channum, len(data), data[:10])
			data = dc.DecimateData(data) // in-place
			fmt.Printf(" after decimate: %d values starting with %v\n", len(data), data[:10])
			// TriggerData()
			// WriteData()
			// PublishDatA()
		}
	}
}

// DecimateData decimates data in-place.
func (dc *DataChannel) DecimateData(data []RawType) []RawType {
	if !dc.Decimate || dc.DecimateLevel <= 1 {
		return data
	}
	Nin := len(data)
	Nout := (Nin - 1 + dc.DecimateLevel) / dc.DecimateLevel
	if dc.DecimateAvgMode {
		level := dc.DecimateLevel
		for i := 0; i < Nout-1; i++ {
			val := float64(data[i*level])
			for j := 1; j < level; j++ {
				val += float64(data[j+i*level])
			}
			data[i] = RawType(val/float64(level) + 0.5)
		}
		data[Nout-1] = data[(Nout-1)*level]
	} else {
		for i := 0; i < Nout; i++ {
			data[i] = data[i*dc.DecimateLevel]
		}
	}
	return data[:Nout]
}
