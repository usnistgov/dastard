package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/usnistgov/dastard/lancero"
)

func main() {
	// Start the adapter
	lan, err := lancero.NewLancero(0)
	defer lan.Close()

	if err != nil {
		log.Println("ERROR: ", err)
		return
	}

	const timeoutSec = 2
	const verbosity = 0
	err = lan.StartAdapter(timeoutSec, verbosity)
	defer lan.StopAdapter()
	if err != nil {
		log.Println("Could not start adapter: ", err)
		return
	}
	lan.InspectAdapter()

	// Configure and start the collector
	err = lan.CollectorConfigure(1, 1, 0xFFFF, 1)
	if err != nil {
		return
	}
	err = lan.StartCollector(false)
	if err != nil {
		return
	}
	defer lan.StopCollector()

	var buffer []byte

	// Trap interrupts so we can cleanly exit the program
	interruptCatcher := make(chan os.Signal, 1)
	signal.Notify(interruptCatcher, os.Interrupt)

	var bytesRead int
	for bytesRead < 1000000 {
		select {
		case <-interruptCatcher:
			log.Println("caught interrupt")
			return
		default:
			_, _, err = lan.Wait()
			if err != nil {
				return
			}
			buffer, _, err = lan.AvailableBuffer()
			bytesRead += len(buffer)
			if err != nil {
				return
			}
			log.Println(lancero.OdDashTX(buffer, 10))
		}
	}
}
