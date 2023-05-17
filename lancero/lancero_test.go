package lancero

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// lanceroFBOffset gives the location of the frame bit is in bytes 2, 6, 10...
const lanceroFBOffset int = 2

func TestLancero(t *testing.T) {
	// call flag.Parse() here if TestMain uses flags
	devs, err := EnumerateLanceroDevices()
	if err != nil {
		t.Error(err)
	}
	if len(devs) < 1 {
		log.Println("found zero lancero devices")
		return
	}
	log.Printf("Found lancero devices %v\n", devs)
	devnum := devs[0]
	lan, err := NewLancero(devnum)
	if err != nil {
		t.Error(err)
	}
	defer lan.Close()
	testLanceroerSubroutine(lan, t)
}

func testLanceroerSubroutine(lan Lanceroer, t *testing.T) (int, int, int, error) {
	// devs, err := EnumerateLanceroDevices()
	// t.Logf("Lancero devices: %v\n", devs)
	// if err != nil {
	// 	t.Errorf("EnumerateLanceroDevices() failed with err=%s", err.Error())
	// }
	//
	// defer lan.Close()
	// if err != nil {
	// 	t.Errorf("%v", err)
	// }
	var nrows, ncols, linePeriod int
	const timeoutSec = 2
	const verbosityIsIgnored = 0
	if err := lan.StartAdapter(timeoutSec, verbosityIsIgnored); err != nil {
		t.Error("failed to start lancero (driver problem):", err)
	}
	lan.InspectAdapter()
	defer lan.StopAdapter()
	linePeriodSet := 1 // use dummy values for things we will learn
	dataDelay := 1
	channelMask := uint32(0xffff)
	frameLength := 1
	err := lan.CollectorConfigure(linePeriodSet, dataDelay, channelMask, frameLength)
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
			return 0, 0, 0, fmt.Errorf("interruptCatcher")
		default:
			_, waittime, err := lan.Wait()
			if err != nil {
				return 0, 0, 0, fmt.Errorf("lan.Wait: %v", err)
			}
			buffer, _, err := lan.AvailableBuffer()
			totalBytes := len(buffer)
			// log.Printf("waittime: %v\n", waittime)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("lan.AvailableBuffers: %v", err)
			}
			// log.Printf("Found buffers with %9d total bytes, bytes read previously=%10d\n", totalBytes, bytesRead)
			if totalBytes > 0 {
				q, p, n, err := FindFrameBits(buffer, lanceroFBOffset)
				bytesPerFrame := 4 * (p - q)
				if err != nil {
					log.Println("Error in findFrameBits:", err)
					spew.Println(buffer)
					log.Println(q, p, n)
					return 0, 0, 0, err
				}
				// log.Println(q, p, bytesPerFrame, n, err)
				nrows = (p - q) / n
				ncols = n
				// log.Println("cols=", n, "rows=", nrows)
				periodNS := waittime.Nanoseconds() / (int64(totalBytes) / int64(bytesPerFrame))
				linePeriod = roundint(float64(periodNS) / float64(nrows*8)) // 8 is nanoseconds per row
				// log.Printf("frame period %5d ns, linePeriod=%d\n", periodNS, linePeriod)
			}
			// Quit when read enough samples.
			bytesRead += totalBytes

			lan.ReleaseBytes(totalBytes)
		}
	}
	return ncols, nrows, linePeriod, nil
}

func TestOdDashTX(t *testing.T) {
	b := make([]byte, 10000)
	expect := 354
	if s := OdDashTX(b, 15); len(s) != expect {
		t.Errorf("have %v\n\n WRONG LENGTH, have %v, want %d", s, len(s), expect)
	}
	for i := range b {
		b[i] = byte(i)
	}
	expect = 1353
	if s := OdDashTX(b, 15); len(s) != expect {
		t.Errorf("have %v\n\n WRONG LENGTH, have %v, want %d", s, len(s), expect)
	}
	b = make([]byte, 0)
	expect = 254
	if s := OdDashTX(b, 15); len(s) != expect {
		t.Errorf("have %v\n\n WRONG LENGTH, have %v, want %d", s, len(s), expect)
	}
	b = []byte{0xef, 0xbe, 0xad, 0xde, 0xef, 0xbe, 0xad, 0xde, 0xef, 0xbe, 0xad, 0xde, 0xef, 0xbe, 0xad, 0xde, 0xef, 0xbe, 0xad, 0xde, 0xef, 0xbe, 0xad, 0xde,
		0xef, 0xbe, 0xad, 0xde, 0xef, 0xbe, 0xad, 0xde, 0xef, 0xbe, 0xad, 0xde, 0xef, 0xbe, 0xad, 0xde, 0xef, 0xbe, 0xad, 0xde, 0xef, 0xbe, 0xad, 0xde}
	if s := OdDashTX(b, 15); strings.Compare("deadbeef", s[255:263]) != 0 {
		t.Errorf("have %v, want %v", s[255:263], "deadbeef")
	}
}

// used in TestFindFrameBits
type frameBitFindTestDataMaker struct {
	frames       int
	nrows        int
	ncols        int
	leadingWords int
}

// used in TestFindFrameBits
func (f frameBitFindTestDataMaker) bytes() []byte {
	var buf bytes.Buffer
	for i := 0; i < f.leadingWords; i++ {
		buf.Write([]byte{0x00, 0x00, 0x00, 0x00})
	}
	for i := 0; i < f.frames; i++ { // i counts frames
		for row := 0; row < f.nrows; row++ {
			for col := 0; col < f.ncols; col++ {
				if row == 0 {
					// first row has frame bit
					buf.Write([]byte{0x00, 0x00, 0x01, 0x00})
				} else {
					// all data is zeros
					buf.Write([]byte{0x00, 0x00, 0x00, 0x00})
				}
			}
		}
	}
	return buf.Bytes()
}

func TestFindFrameBits(t *testing.T) {
	b := frameBitFindTestDataMaker{frames: 100, nrows: 8, ncols: 2, leadingWords: 0}.bytes()
	firstWord, secondWord, nConsecutive, err := FindFrameBits(b, lanceroFBOffset)
	if err != nil {
		t.Error(err)
	}
	if firstWord != 16 {
		t.Errorf("have %v, want 16", firstWord)
	}
	if secondWord != 16+16 {
		t.Errorf("have %v, want 32", secondWord)
	}
	if nConsecutive != 2 {
		t.Errorf("have %v, want 2", nConsecutive)
	}
	b = frameBitFindTestDataMaker{frames: 100, nrows: 8, ncols: 2, leadingWords: 0}.bytes()
	firstWord, secondWord, nConsecutive, err = FindFrameBits(b[4:len(b)-1], lanceroFBOffset)
	// start mid frame bits
	if err != nil {
		t.Error(err)
	}
	if firstWord != 15 {
		t.Errorf("have %v, want 15", firstWord)
	}
	if secondWord != 15+16 {
		t.Errorf("have %v, want 31", secondWord)
	}
	if nConsecutive != 2 {
		t.Errorf("have %v, want 2", nConsecutive)
	}
	b = frameBitFindTestDataMaker{frames: 100, nrows: 8, ncols: 2, leadingWords: 10}.bytes()
	firstWord, secondWord, nConsecutive, err = FindFrameBits(b, lanceroFBOffset)
	if err != nil {
		t.Error(err)
	}
	if firstWord != 10 {
		t.Errorf("have %v, want 10", firstWord)
	}
	if secondWord != 10+16 {
		t.Errorf("have %v, want 26", secondWord)
	}
	if nConsecutive != 2 {
		t.Errorf("have %v, want 2", nConsecutive)
	}
	b = frameBitFindTestDataMaker{frames: 100, nrows: 8, ncols: 8, leadingWords: 0}.bytes()
	firstWord, secondWord, nConsecutive, err = FindFrameBits(b, lanceroFBOffset)
	if err != nil {
		t.Error(err)
	}
	if firstWord != 64 {
		t.Errorf("have %v, want 10", firstWord)
	}
	if secondWord != 64+64 {
		t.Errorf("have %v, want 26", secondWord)
	}
	if nConsecutive != 8 {
		t.Errorf("have %v, want 2", nConsecutive)
	}
}

// Imperfect round to nearest integer
func roundint(x float64) int {
	return int(x + math.Copysign(0.5, x))
}
