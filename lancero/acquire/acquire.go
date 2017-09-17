package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/usnistgov/dastard/lancero"
)

type acquireOptions struct {
	period, delay     int
	length, verbosity int
	threshold         int
	nSamples          int
	mask              uint64
	output            string
	simulate          bool
}

var opt acquireOptions

func parseOptions() error {
	flag.IntVar(&opt.period, "p", 32, "line sync period, in clock cycles")
	flag.IntVar(&opt.delay, "d", 0, "data delay, in clock cycles")
	flag.IntVar(&opt.length, "l", 32, "frame length")
	flag.IntVar(&opt.verbosity, "v", 0, "verbosity level")
	flag.IntVar(&opt.threshold, "t", 1024, "threshold (in frames), fill level interrupt")
	flag.IntVar(&opt.nSamples, "n", 0, "number of samples to acquire (<=0 means run indenfinitely)")
	flag.Uint64Var(&opt.mask, "m", 0xffff, "channel mask for each of 16 channels")
	flag.StringVar(&opt.output, "o", "", "output filename")
	flag.BoolVar(&opt.simulate, "s", false, "simulate data (if false, read from fibers)")
	flag.Parse()

	switch {
	case opt.period < 16:
		return fmt.Errorf("Line sync period (%d) must be at least 16", opt.period)
	case opt.period >= 1024:
		return fmt.Errorf("Line sync period (%d) must be < 1024", opt.period)
	case opt.mask > 0xffff:
		return fmt.Errorf("Line sync period (0x%x) must be < 0xffff", opt.mask)
	case opt.delay < 0 || opt.delay >= 32:
		return fmt.Errorf("Line delay (%d) must be in [0,31]", opt.delay)
	case opt.threshold < 1:
		return fmt.Errorf("Threshold (%d) must be at least 1", opt.threshold)
	case opt.threshold < 1024:
		fmt.Printf("WARNING: Threshold (%d) is recommended to be at least 1024", opt.threshold)
	}
	return nil
}

func acquire(lan *lancero.Lancero) (bytesRead int, err error) {
	// Store output?
	var fd *os.File
	output := len(opt.output) > 0
	if output {
		fd, err = os.Create(opt.output)
		if err != nil {
			return
		}
		defer fd.Close()
	} else {
		fd = nil
	}
	fmt.Println(fd)

	// Start the adapter
	err = lan.StartAdapter(2)
	if err != nil {
		return
	}
	defer lan.StopAdapter()

	// Configure and start the collector
	err = lan.CollectorConfigure(opt.period, opt.delay, opt.mask, opt.length)
	if err != nil {
		return
	}
	err = lan.StartCollector(opt.simulate)
	if err != nil {
		return
	}
	defer lan.StopCollector()
	defer lan.InspectAdapter()

	var buffers [][]byte
	var totalBytes int
	interruptCatcher := make(chan os.Signal, 1)
	signal.Notify(interruptCatcher, os.Interrupt)
	for {
		select {
		case <-interruptCatcher:
			return
		default:
			err = lan.Wait()
			if err != nil {
				return
			}
			buffers, totalBytes, err = lan.AvailableBuffers()
			if err != nil {
				return
			}
			fmt.Printf("Found %d buffers with %d total bytes\n", len(buffers), totalBytes)
			bytesRead += totalBytes
			if opt.nSamples > 0 && opt.nSamples <= bytesRead/4 {
				return
			}

			if output {
				for _, b := range buffers {
					if len(b) > 0 {
						var n int
						n, err = fd.Write(b)
						if err != nil {
							return
						}
						if n != len(b) {
							err = fmt.Errorf("Wrote %d bytes, expected %d", n, len(b))
							return
						}
					}
				}
			}

			// Verify the simulated data, if simulated.

			lan.ReleaseBytes(totalBytes)
		}
	}
}

func main() {
	err := parseOptions()
	if err != nil {
		fmt.Println("ERROR: ", err)
		return
	}

	fmt.Println(opt)
	lan, err := lancero.NewLancero(0)
	if err != nil {
		fmt.Println("ERROR: ", err)
		return
	}
	bytesRead, _ := acquire(lan)
	fmt.Printf("Read %d bytes.\n", bytesRead)
}
