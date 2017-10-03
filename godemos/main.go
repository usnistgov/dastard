package main

import (
	"fmt"
	"time"

	"github.com/usnistgov/dastard"
)

func main() {
	source := dastard.NewTriangleSource(4, 200000., 0, 65535)
	source.Configure()

	ts := dastard.DataSource(source)
	ts.Sample()
	ts.Start()
	dataChannels := ts.Outputs()
	abort := make(chan struct{})
	go func() {
		for {
			ts.BlockingRead(abort)

			// Drain the data.
			for chnum, ch := range dataChannels {
				x := <-ch
				fmt.Printf("Chan %d: found %d values\n", chnum, len(x))
			}
		}
	}()
	// Take data for 4 seconds
	time.Sleep(time.Second * 4)
	fmt.Println("Will quit in 2 seconds")
	close(abort)
	time.Sleep(time.Second * 2)
}
