package lancero

import (
	"fmt"
	"os"
	"os/signal"
	"testing"
)

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags
	devs, err := EnumerateLanceroDevices()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if len(devs) < 1 {
		fmt.Println("found zero lancero devices")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLancero(t *testing.T) {
	devs, err := EnumerateLanceroDevices()
	t.Logf("Lancero devices: %v\n", devs)
	if err != nil {
		t.Errorf("EnumerateLanceroDevices() failed with err=%s", err.Error())
	}
	devnum := devs[0]
	lan, err := NewLancero(devnum)
	defer lan.Close()
	if err != nil {
		t.Errorf("%v", err)
	}
	if err1 := lan.StartAdapter(2); err1 != nil {
		t.Error("failed to start lancero (driver problem):", err1)
	}
	lan.InspectAdapter()
	defer lan.StopAdapter()
	linePeriod := 1 // use dummy values for things we will learn
	dataDelay := 1
	channelMask := uint32(0xffff)
	frameLength := 1
	err = lan.CollectorConfigure(linePeriod, dataDelay, channelMask, frameLength)
	if err != nil {
		t.Errorf("CollectorConfigure err, %v", err)
	}
	simulate := false
	err = lan.StartCollector(simulate)
	defer lan.StopCollector()
	if err != nil {
		t.Errorf("StartCollector err, %v", err)
	}
	interruptCatcher := make(chan os.Signal, 1)
	signal.Notify(interruptCatcher, os.Interrupt)

	var bytesRead int
	for {
		if 100000000 <= bytesRead {
			break
		}
		select {
		case <-interruptCatcher:
			return
		default:
			_, waittime, err := lan.Wait()
			if err != nil {
				return
			}
			buffers, totalBytes, err := lan.AvailableBuffers()
			fmt.Printf("waittime: %v\n", waittime)
			if err != nil {
				return
			}
			fmt.Printf("Found %d buffers with %d total bytes, bytesRead=%d\n", len(buffers), totalBytes, bytesRead)
			if len(buffers) > 0 {
				for _, b := range buffers {
					q, p, n, err := FindFrameBits(b)
					bytesPerFrame := 4 * (p - q)
					if err != nil {
						fmt.Println("Error in findFrameBits:", err)
						break
					}
					fmt.Println(q, p, bytesPerFrame, n, err)
					fmt.Println("cols=", n, "rows=", (p-q)/n)
					fmt.Println("frame period nanoseconds", waittime.Nanoseconds()/(int64(totalBytes)/int64(bytesPerFrame)))
				}
			}
			// Quit when read enough samples.
			bytesRead += totalBytes

			lan.ReleaseBytes(totalBytes)
		}
	}
	fmt.Println("loop DONE DONE DONE")
}
