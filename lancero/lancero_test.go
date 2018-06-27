package lancero

import (
	"fmt"
	"math"
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
	fmt.Printf("Found lancero devices %v\n", devs)
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
			buffer, err := lan.AvailableBuffers()
			totalBytes := len(buffer)
			fmt.Printf("waittime: %v\n", waittime)
			if err != nil {
				return
			}
			fmt.Printf("Found buffers with %9d total bytes, bytes read previously=%10d\n", totalBytes, bytesRead)
			if totalBytes > 0 {
				q, p, n, err := FindFrameBits(buffer)
				bytesPerFrame := 4 * (p - q)
				if err != nil {
					fmt.Println("Error in findFrameBits:", err)
					break
				}
				fmt.Println(q, p, bytesPerFrame, n, err)
				nrows := (p - q) / n
				fmt.Println("cols=", n, "rows=", nrows)
				periodNS := waittime.Nanoseconds() / (int64(totalBytes) / int64(bytesPerFrame))
				lsync := roundint(float64(periodNS) / float64(nrows*8))
				fmt.Printf("frame period %5d ns, lsync=%d\n", periodNS, lsync)
			}
			// Quit when read enough samples.
			bytesRead += totalBytes

			lan.ReleaseBytes(totalBytes)
		}
	}
	fmt.Println("loop DONE DONE DONE")
}

func TestOdDashTX(t *testing.T) {
	b := make([]byte, 10000)
	if s := OdDashTX(b, 15); len(s) != 281 {
		t.Errorf("have %v\n\n WRONG LENGTH, have %v, want 281", s, len(s))
	}
	for i := range b {
		b[i] = byte(i)
	}
	if s := OdDashTX(b, 15); len(s) != 1280 {
		t.Errorf("have %v\n\n WRONG LENGTH, have %v, want 1280", s, len(s))
	}
	b = make([]byte, 0)
	if s := OdDashTX(b, 15); len(s) != 181 {
		t.Errorf("have %v\n\n WRONG LENGTH, have %v, want 181", s, len(s))
	}
}

// Imperfect round to nearest integer
func roundint(x float64) int {
	return int(x + math.Copysign(0.5, x))
}
