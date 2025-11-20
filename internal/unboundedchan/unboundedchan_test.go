package unboundedchan

import (
	"testing"
)

func TestUnboundedChannel(t *testing.T) {
	unboundedQueue := NewUnboundedChannel[int]()

	// Goroutine to send data.
	// Send a all integers [0, 19].
	max := 20
	go func() {
		ch := unboundedQueue.In()
		for i := range max {
			ch <- i
		}
		close(ch) // Close the input channel when done
	}()

	// Goroutine to receive and process data (here, sum it all up)
	sum := 0
	expect := (max * (max - 1)) / 2
	for d := range unboundedQueue.Out() {
		sum += d
	}
	if sum != expect {
		t.Errorf("UnboundedQueue sum was %d, want %d", sum, expect)
	}
}
