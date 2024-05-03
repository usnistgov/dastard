package asyncbufio

import (
	"bufio"
	"io"
	"sync"
	"time"
)

// Writer provides asynchronous writing to an underlying io.Writer using buffered channels.
type Writer struct {
	writer        *bufio.Writer  // Buffered writer: this does the writing
	flushNow      chan struct{}  // Channel to signal the underlying writer to flush itself
	databuffer    chan []byte    // Channel to hold data before writing it
	flushInterval time.Duration  // Interval for flushing the writer periodically
	wg            sync.WaitGroup // WaitGroup to synchronize end of writing
}

// NewWriter creates a new Writer instance.
func NewWriter(w io.Writer, bufferSize int, flushInterval time.Duration) *Writer {
	aw := &Writer{
		writer:        bufio.NewWriter(w),
		databuffer:    make(chan []byte, bufferSize),
		flushNow:      make(chan struct{}),
		flushInterval: flushInterval, // Set the flush interval
	}

	aw.wg.Add(1)
	go aw.writeLoop()
	return aw
}

// Write stores data to the Writer's buffer for later writing.
func (aw *Writer) Write(p []byte) (int, error) {
	select {
	case aw.databuffer <- p:
		return len(p), nil
	default:
		return 0, io.ErrShortWrite // Return an error if buffer is full
	}
}

// Flush flushes any remaining data in the buffer to the underlying writer.
func (aw *Writer) Flush() error {
	aw.flushNow <- struct{}{}
	return nil
}

// Close closes the Writer, flushing remaining data and waiting for the writeLoop to finish.
// It is possible to cause a panic by calling Write(p) or Flush() after Close()--we don't
// test for that case.
func (aw *Writer) Close() {
	close(aw.flushNow) // Closing the flushNow channel signals the writeLoop to exit
	aw.wg.Wait()       // Wait until writing is complete
}

// writeLoop is a goroutine that continuously moves data from the buffer to the writer.
func (aw *Writer) writeLoop() {
	defer aw.wg.Done() // Decrement WaitGroup when writing is complete

	ticker := time.NewTicker(aw.flushInterval) // Ticker to flush periodically
	defer ticker.Stop()                        // Stop the ticker when the writeLoop exits

	for {
		breakFromLoop := false
		select {
		case data := <-aw.databuffer:
			aw.writer.Write(data) // Write data from the buffer to the writer

		case _, ok := <-aw.flushNow:
			breakFromLoop = !ok
			aw.flush()
		case <-ticker.C:
			aw.flush()
		}
		if breakFromLoop {
			return
		}
	}
}

func (aw *Writer) flush() {
	defer aw.writer.Flush()
	for {
		select {
		case data := <-aw.databuffer:
			aw.writer.Write(data)
		default:
			return
		}
	}
}
