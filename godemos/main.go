package main

import (
	"fmt"
	"io"
	"time"

	"github.com/usnistgov/dastard"
)

func main() {
	source := dastard.NewTriangleSource(4, 200000., 0, 65535)
	source.Configure()

	ts := dastard.DataSource(source)
	ts.Sample()
	ts.Start()
	allOutputs := ts.Outputs()
	abort := make(chan struct{})

	// Drain the data produced by the DataSource.
	for chnum, ch := range allOutputs {
		dc := &dastard.DataChannel{Abort: abort, Channum: chnum}
		dc.Decimate = true
		dc.DecimateLevel = 3
		dc.DecimateAvgMode = true
		dc.LevelTrigger = true
		dc.LevelLevel = 3000
		dc.NPresamples = 100
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

	// Take data for 4 seconds
	time.Sleep(time.Second * 4)
	fmt.Println("Will quit in 2 seconds")
	close(abort)
	time.Sleep(time.Second * 2)
}
