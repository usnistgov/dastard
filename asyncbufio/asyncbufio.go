package asyncbufio

import (
	"bufio"
	"io"
	"time"
)

// Writer provides asynchronous writing to an underlying io.Writer using buffered channels.
type Writer struct {
	writer        *bufio.Writer // Buffered writer: this does the writing
	flushNow      chan struct{} // Channel to signal the underlying writer to flush itself
	flushComplete chan struct{} // Channel to signal underlying writer flush is complete
	databuffer    chan []byte   // Channel to hold data before writing it
	flushInterval time.Duration // Interval for flushing the writer periodically
}

// NewWriter creates a new Writer instance.
func NewWriter(w io.Writer, bufferSize int, flushInterval time.Duration) *Writer {
	aw := &Writer{
		writer:        bufio.NewWriter(w),
		databuffer:    make(chan []byte, bufferSize),
		flushNow:      make(chan struct{}),
		flushComplete: make(chan struct{}),
		flushInterval: flushInterval, // Set the flush interval
	}

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

// WriteString writes a string to the buffer for later writing (with an annoying copy--sorry!)
func (aw *Writer) WriteString(s string) (int, error) {
	return aw.Write([]byte(s))
}

// Flush flushes any remaining data in the buffer to the underlying writer.
// Blocks until the flush is complete.
func (aw *Writer) Flush() error {
	aw.flushNow <- struct{}{}
	<-aw.flushComplete
	return nil
}

// Close closes the Writer, flushing remaining data and waiting for the writeLoop to finish.
// It is possible to cause a panic by calling Write(p) or Flush() after Close()--we don't
// test for that case.
func (aw *Writer) Close() {
	close(aw.flushNow) // Closing the flushNow channel signals the writeLoop to exit
	<-aw.flushComplete // Wait until writing is complete
}

// writeLoop is a goroutine that continuously moves data from the buffer to the writer.
func (aw *Writer) writeLoop() {
	ticker := time.NewTicker(aw.flushInterval) // Ticker to flush periodically
	defer ticker.Stop()                        // Stop the ticker when the writeLoop exits

	for {
		select {
		case data := <-aw.databuffer:
			aw.writer.Write(data) // Write data from the buffer to the writer

		case _, ok := <-aw.flushNow:
			aw.flush()
			// Signal whoever requested this that flushing is done
			aw.flushComplete <- struct{}{}
			if !ok {
				return
			}

		case <-ticker.C:
			aw.flush()
		}
	}
}

func (aw *Writer) flush() {
	// This loop empties the aw.databuffer channel before finally
	// calling the underlying writer's Flush() method
	for {
		select {
		case data := <-aw.databuffer:
			aw.writer.Write(data)
		default:
			aw.writer.Flush()
			return
		}
	}
}
