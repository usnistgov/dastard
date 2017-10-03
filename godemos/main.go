package main

import (
	"fmt"

	"github.com/usnistgov/dastard"
)

func main() {
	ts := dastard.NewTriangleSource(4, 200000., 0, 65535)
	ts.Configure()
	ts.Start()
	dataChannels := ts.Outputs()
	for i := 0; i < 50; i++ {
		ts.BlockingRead()
		for chnum, ch := range dataChannels {
			x := <-ch
			fmt.Printf("Chan %d: found %d values\n", chnum, len(x))
		}
	}
}
