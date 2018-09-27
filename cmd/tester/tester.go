package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/usnistgov/dastard/ljh"
)

const flushWithinBlock = false
const flushAfterBlocks = true

func main() {
	dirname := "/data/testertmp/"
	fmt.Println(dirname)
	recordLength := 300
	N := 500
	recordsPerChanPerTick := 10
	writers := make([]ljh.Writer, N)
	abortChan := make(chan struct{})
	for i := range writers {
		writers[i] = ljh.Writer{FileName: fmt.Sprintf("%v%v.ljh", dirname, i), ChannelIndex: i, Samples: recordLength}
	}
	go func() {
		signalChan := make(chan os.Signal)
		signal.Notify(signalChan, os.Interrupt)
		<-signalChan
		close(abortChan)
	}()

	tickDuration := 50 * time.Millisecond
	ticker := time.NewTicker(tickDuration)
	z := 0
	tLast := time.Now()
	fmt.Printf("recordsPerChanPerTick %v, Chans %v, tickDuration %v\n", recordsPerChanPerTick, N, tickDuration)
	fmt.Printf("records/second/chan %v, records/second total %v\n", float64(recordsPerChanPerTick)/tickDuration.Seconds(), float64(recordsPerChanPerTick*N)/tickDuration.Seconds())
	fmt.Printf("megabytes/second total %v\n", float64(recordLength*2+8+8)*float64(recordsPerChanPerTick*N)/tickDuration.Seconds()*1e-6)
	fmt.Printf("flushWithinBlock %v, flushAfterBlocks %v\n", flushWithinBlock, flushAfterBlocks)
	for {
		z++
		select {
		case <-abortChan:
			fmt.Println("clean exit")
			return
		case <-ticker.C:
			var wg sync.WaitGroup
			writeDurations := make([]time.Duration, N)
			flushDurations := make([]time.Duration, N)
			for i, w := range writers {
				records := make([]Record, recordsPerChanPerTick)
				for j := range records {
					records[j] = Record{framecount: int64(z*10000 + j), timestamp: int64(z*10000 + i), data: make([]uint16, recordLength)}
				}
				wg.Add(1)
				go func(w ljh.Writer, records []Record, i int) {
					tStart := time.Now()
					defer wg.Done()
					for _, record := range records {
						if !w.HeaderWritten {
							err := w.CreateFile()
							if err != nil {
								panic(fmt.Sprintf("failed create file: %v\n", err))
							}
							w.WriteHeader(time.Now())
						}
						w.WriteRecord(record.framecount, record.timestamp, record.data)
					}
					tWrite := time.Now()
					if flushWithinBlock {
						w.Flush() // Flush here for terrible performance
					}
					writeDurations[i] = tWrite.Sub(tStart)
					flushDurations[i] = time.Now().Sub(tWrite)
				}(w, records, i)
			}
			wg.Wait()
			for _, w := range writers {
				if flushAfterBlocks {
					w.Flush() // Flush here or not at all for reasonable performance
				}
			}
			var writeSum time.Duration
			var flushSum time.Duration
			for i := range writeDurations {
				writeSum += writeDurations[i]
				flushSum += flushDurations[i]
			}
			if z%100 == 0 || time.Now().Sub(tLast) > 75*time.Millisecond {
				fmt.Printf("z %v, time.Now().Sub(tLast) %v\n", z, time.Now().Sub(tLast))
				fmt.Printf("writeSum %v, flushSum %v\n", writeSum, flushSum)
			}
			tLast = time.Now()
		}
	}

}

// Record does stuff
type Record struct {
	framecount int64
	timestamp  int64
	data       []uint16
}
