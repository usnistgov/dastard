package main

import (
	// "bytes"
	// "encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

    "github.com/usnistgov/dastard"
)

type acquireOptions struct {
	verbosity int
	threshold int
	nSamples  int
    devnum    int
	output    string
}

var opt acquireOptions

func parseOptions() error {
	flag.IntVar(&opt.verbosity, "v", 0, "verbosity level")
	flag.IntVar(&opt.threshold, "t", 1024, "threshold (in frames), fill level interrupt")
	flag.IntVar(&opt.nSamples, "n", 0, "number of samples to acquire (<=0 means run indenfinitely)")
    flag.IntVar(&opt.devnum, "d", 0, "device number for /dev/xdma0_c2h_*")
	flag.StringVar(&opt.output, "o", "", "output filename")
	flag.Parse()

	switch {
	case opt.threshold < 1:
		return fmt.Errorf("Threshold (%d) must be at least 1", opt.threshold)
	case opt.threshold < 1024:
		log.Printf("WARNING: Threshold (%d) is recommended to be at least 1024", opt.threshold)
	}
	return nil
}


func acquire(abaco *dastard.AbacoDevice) (bytesRead int, err error) {

	// Store output?
	var fd *os.File
	saveData := len(opt.output) > 0
	if saveData {
		fd, err = os.Create(opt.output)
		if err != nil {
			return
		}
		defer fd.Close()
	} else {
		fd = nil
	}

    // Start something??

	var buffer []byte

	// Trap interrupts so we can cleanly exit the program
	interruptCatcher := make(chan os.Signal, 1)
	signal.Notify(interruptCatcher, os.Interrupt)

	for {
		select {
		case <-interruptCatcher:
			return
		default:
			// _, _, err = lan.Wait()
			// if err != nil {
			// 	return
			// }
			// buffer, _, err = lan.AvailableBuffer()
			totalBytes := len(buffer)
			// if err != nil {
			// 	return
			// }
			// log.Printf("Found buffer with %d total bytes", totalBytes)
			// log.Printf(" size %d,", len(buffer))
			// log.Println()
			// lan.InspectAdapter()

			if saveData {
				bytesWritten := bytesRead
				if len(buffer) > 0 {
					var n int
					if len(buffer)+bytesWritten <= opt.nSamples*4 {
						n, err = fd.Write(buffer)
					} else {
						nwrite := opt.nSamples*4 - bytesWritten
						n, err = fd.Write(buffer[:nwrite])
					}
					if err != nil {
						return
					}
					if n != len(buffer) {
						err = fmt.Errorf("Wrote %d bytes, expected %d", n, len(buffer))
						return
					}
				}
			}

			// Quit when read enough samples.
			bytesRead += totalBytes
			if opt.nSamples > 0 && opt.nSamples <= bytesRead/4 {

				return
			}

			log.Println()
		}
	}

}

func main() {
	err := parseOptions()
	if err != nil {
		log.Println("ERROR: ", err)
		return
	}

    abaco, err := dastard.NewAbacoDevice(opt.devnum)
	if err != nil {
		log.Println("ERROR: ", err)
		return
	}
	// defer abaco.Delete()

	bytesRead, _ := acquire(abaco)
	log.Printf("Read %d bytes.\n", bytesRead)
}
