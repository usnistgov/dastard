package main

import (
	"fmt"
	"io"
	"time"

	"github.com/usnistgov/dastard"
)

var pubRecPort = 32002

func main() {
	// source := dastard.NewTriangleSource(4, 200000., 0, 65535)
	// source.Configure()
	source := new(dastard.SimPulseSource)
	source.Configure(4, 200000.0, 5000.0, 10000.0, 65536)

	ts := dastard.DataSource(source)
	ts.Sample()
	ts.Start()
	allOutputs := ts.Outputs()
	abort := make(chan struct{})

	dataToPub := make(chan []*dastard.DataRecord, 500)
	go dastard.PublishRecords(dataToPub, abort, pubRecPort)

	// Launch goroutines to drain the data produced by the DataSource.
	for chnum, ch := range allOutputs {
		dc := dastard.NewDataChannel(chnum, abort, dataToPub)
		dc.Decimate = false
		dc.DecimateLevel = 3
		dc.DecimateAvgMode = true
		dc.LevelTrigger = true
		dc.LevelLevel = 5500
		dc.NPresamples = 200
		dc.NSamples = 1000
		go dc.ProcessData(ch)
	}

	// Have the DataSource produce data until graceful stop.
	go func() {
		for {
			if err := ts.BlockingRead(abort); err == io.EOF {
				fmt.Printf("BlockingRead returns EOF\n")
				return
			} else if err != nil {
				fmt.Printf("BlockingRead returns Error\n")
				return
			}
		}
	}()

	// Take data for 4 seconds, stop, and wait 2 additional seconds.
	time.Sleep(time.Second * 4)
	fmt.Println("Stopping data acquisition. Will quit in 2 seconds")
	close(abort)
	time.Sleep(time.Second * 2)
}
