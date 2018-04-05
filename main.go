package main

import (
	"fmt"
	"io"
	"time"
)

var pubRecPort = 5556

func main() {
	NChan := 4
	source := new(SimPulseSource)
	source.Configure(NChan, 200000.0, 5000.0, 10000.0, 65536)

	ts := DataSource(source)
	ts.Sample()
	ts.Start()
	allOutputs := ts.Outputs()
	abort := make(chan struct{})

	dataToPub := make(chan []*DataRecord, 500)
	go PublishRecords(dataToPub, abort, pubRecPort)

	broker := NewTriggerBroker(NChan)
	go broker.Run(abort)

	// Launch goroutines to drain the data produced by the DataSource.
	for chnum, ch := range allOutputs {
		dsp := NewDataStreamProcessor(chnum, abort, dataToPub, broker)
		dsp.Decimate = false
		dsp.DecimateLevel = 3
		dsp.DecimateAvgMode = true
		dsp.LevelTrigger = true
		dsp.LevelLevel = 5500
		dsp.NPresamples = 200
		dsp.NSamples = 1000
		go dsp.ProcessData(ch)
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
	time.Sleep(time.Second * 44)
	fmt.Println("Stopping data acquisition. Will quit in 2 seconds")
	close(abort)
	time.Sleep(time.Second * 2)
}
